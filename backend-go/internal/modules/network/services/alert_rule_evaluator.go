package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
	"itops-backend/internal/modules/ops"
	"itops-backend/internal/repos"
)

var (
	evaluatorMu     sync.Mutex
	evaluatorCancel context.CancelFunc
	evaluatorDone   chan struct{}
)

// StartAlertRuleEvaluator 启动夜莺风格 Prometheus 规则后台定时评估引擎（间隔 10s）
func StartAlertRuleEvaluator() {
	evaluatorMu.Lock()
	if evaluatorCancel != nil {
		evaluatorMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	evaluatorCancel = cancel
	evaluatorDone = done
	evaluatorMu.Unlock()

	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Infof("Alert rule evaluator stopped")
				return
			case <-ticker.C:
				evaluateActiveAlertRules(ctx)
			}
		}
	}()
	logger.Infof("Nightignale-style Prometheus alert rule evaluator started (10s interval)")
}

// StopAlertRuleEvaluator 停止告警规则评估引擎并等待后台循环退出（幂等）
func StopAlertRuleEvaluator() {
	evaluatorMu.Lock()
	cancel := evaluatorCancel
	done := evaluatorDone
	evaluatorCancel = nil
	evaluatorDone = nil
	evaluatorMu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		<-done
	}
}

type SampleEvaluationItem struct {
	Value       float64           `json:"value"`
	DeviceID    string            `json:"deviceId"`
	DeviceIP    string            `json:"deviceIp"`
	DeviceName  string            `json:"deviceName"`
	Vendor      string            `json:"vendor"`
	IfName      string            `json:"ifName,omitempty"`
	MetricName  string            `json:"metricName"`
	MetricLabel string            `json:"metricLabel,omitempty"`
	Unit        string            `json:"unit,omitempty"`
	Tags        map[string]string `json:"tags"`
	IsTriggered bool              `json:"isTriggered"`
}

func evaluateActiveAlertRules(ctx context.Context) {
	rules, err := repos.AlertRules.List()
	if err != nil || len(rules) == 0 {
		return
	}

	db := appdb.DB()

	for _, rule := range rules {
		if ctx.Err() != nil {
			return
		}
		if !rule.Enabled {
			continue
		}

		// 提取与此规则 Metric 相关的当前采样值列表；无真实样本则不评估、不产生告警
		samples := CollectMetricCurrentSamples(rule.MetricName)
		if len(samples) == 0 {
			continue
		}

		nowTs := time.Now().UnixMilli()

		for _, sample := range samples {
			// 1. Tag 过滤匹配检查
			if !MatchTagFilters(sample.Tags, rule.TagsFilter) {
				continue
			}

			// 2. 比较运算符与阈值判定
			isTriggered := CheckThresholdCondition(sample.Value, rule.Operator, rule.Threshold)

			if isTriggered {
				// 3. 静默时间 (Cooldown) 校验
				if nowTs-rule.LastTriggered < rule.CooldownMs {
					continue
				}

				// 触发告警记录插入
				triggerAlertRecord(db, rule, sample, nowTs)
				_ = repos.AlertRules.UpdateLastTriggered(rule.ID, nowTs)
				// 同一评估周期内立即进入静默：否则 N 个超阈值样本会各插一条告警（风暴）
				rule.LastTriggered = nowTs
			}
		}
	}
}

// deviceInfo 设备基础信息（app 库）
type deviceInfo struct {
	IP     string
	Name   string
	Vendor string
	Status string
}

// loadDeviceInfoMap 读取设备基础信息，避免指标库与 app 库之间跨库 JOIN
func loadDeviceInfoMap() map[string]deviceInfo {
	result := make(map[string]deviceInfo)
	rows, err := appdb.DB().Query(`SELECT id, COALESCE(ip_address, ''), COALESCE(name, ''), COALESCE(vendor, ''), COALESCE(status, '') FROM network_devices`)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var info deviceInfo
		if rows.Scan(&id, &info.IP, &info.Name, &info.Vendor, &info.Status) == nil {
			result[id] = info
		}
	}
	return result
}

// parseMetricValue 解析存储的指标值；形如 "OID=123" 时只取数值部分
func parseMetricValue(raw string) (float64, bool) {
	cleanValStr := strings.TrimSpace(raw)
	if idx := strings.Index(cleanValStr, "="); idx != -1 {
		cleanValStr = cleanValStr[idx+1:]
	}
	v, err := stringsToFloat(cleanValStr)
	if err != nil {
		return 0, false
	}
	return v, true
}

// sampleFreshnessWindow 样本新鲜度窗口：超过该时长未更新的样本不参与阈值判定，
// 防止采集停止后历史值在每个静默期结束后重复触发告警
const sampleFreshnessWindow = 5 * time.Minute

// isSampleFresh 判断样本时间戳是否在新鲜度窗口内（无法解析的时间戳视为不新鲜）
func isSampleFresh(ts string, nowTs int64) bool {
	t, err := appdb.ParseDBTime(ts)
	if err != nil {
		return false
	}
	return nowTs-t.UnixMilli() <= sampleFreshnessWindow.Milliseconds()
}

// CollectMetricCurrentSamples 收集当前指标的所有设备采样值（仅返回真实采集到的样本）
func CollectMetricCurrentSamples(metricName string) []SampleEvaluationItem {
	var items []SampleEvaluationItem
	seenMap := make(map[string]bool)
	devices := loadDeviceInfoMap()
	nowTs := time.Now().UnixMilli()

	// 1. 从 custom_dashboard_metrics 查询（用户自定义指标与标签，位于指标库）
	custRows, err := appdb.DBForTable("custom_dashboard_metrics").Query(`
		SELECT m.id, m.oid, m.custom_name, m.device_id, m.last_value, m.unit,
		       COALESCE(m.metric_name, ''), COALESCE(m.tags, ''), COALESCE(m.last_sampled_at, '')
		FROM custom_dashboard_metrics m
	`)
	if err == nil {
		defer custRows.Close()
		for custRows.Next() {
			var mId, mOid, cName, devId, lastVal, unit, mName, cTags, lastSampled string
			if err := custRows.Scan(&mId, &mOid, &cName, &devId, &lastVal, &unit, &mName, &cTags, &lastSampled); err != nil {
				continue
			}
			info := devices[devId]
			keyName := strings.TrimSpace(mName)
			if keyName == "" {
				if strings.Contains(strings.ToLower(cName), "cpu") {
					keyName = "cpu_usage"
				} else if strings.Contains(cName, "内存") {
					keyName = "memory_usage"
				} else {
					keyName = "custom_metric"
				}
			}

			// 如果指定了 metricName 并且不匹配，跳过（如果 metricName 为空则匹配全部）
			if metricName != "" && keyName != metricName {
				continue
			}

			// 无法解析为数值说明没有真实采样，直接跳过（禁止用占位值参与阈值判定）
			val, ok := parseMetricValue(lastVal)
			if !ok {
				continue
			}
			// 采集已停止的指标不再参与判定，避免历史值反复触发
			if !isSampleFresh(lastSampled, nowTs) {
				continue
			}

			tags := map[string]string{
				"device_id":    devId,
				"device_ip":    info.IP,
				"device_name":  info.Name,
				"vendor":       info.Vendor,
				"metric_label": cName,
				"status":       info.Status,
			}
			if cTags != "" {
				var extra map[string]string
				if json.Unmarshal([]byte(cTags), &extra) == nil {
					for k, v := range extra {
						tags[k] = v
					}
				}
			}

			uniqueKey := fmt.Sprintf("%s-%s", devId, keyName)
			seenMap[uniqueKey] = true

			items = append(items, SampleEvaluationItem{
				Value:       val,
				DeviceID:    devId,
				DeviceIP:    info.IP,
				DeviceName:  info.Name,
				Vendor:      info.Vendor,
				MetricName:  keyName,
				MetricLabel: cName,
				Unit:        unit,
				Tags:        tags,
			})
		}
	}

	// 2. 从 network_devices 补充设备在线状态指标（真实 status 字段）
	if metricName == "device_status" || metricName == "" {
		rows, err := appdb.DB().Query(`SELECT id, ip_address, name, vendor, status FROM network_devices WHERE status != ''`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id, ip, name, vendor, status string
				if err := rows.Scan(&id, &ip, &name, &vendor, &status); err != nil {
					continue
				}
				tags := map[string]string{
					"device_id":   id,
					"device_ip":   ip,
					"device_name": name,
					"vendor":      vendor,
					"status":      status,
				}
				uniqueKey := fmt.Sprintf("%s-device_status", id)
				if seenMap[uniqueKey] {
					continue
				}
				seenMap[uniqueKey] = true
				stVal := 0.0
				if status == "online" {
					stVal = 1.0
				}
				items = append(items, SampleEvaluationItem{
					Value:       stVal,
					DeviceID:    id,
					DeviceIP:    ip,
					DeviceName:  name,
					Vendor:      vendor,
					MetricName:  "device_status",
					MetricLabel: "设备在线状态",
					Unit:        "0/1",
					Tags:        tags,
				})
			}
		}
	}

	// 3. SNMP metric samples（指标库，不与 app 库网络设备表做跨库 JOIN）
	if metricName != "" && metricName != "device_status" {
		rows, err := appdb.DBForTable("snmp_metric_samples").Query(`
			SELECT s.device_id, s.metric_name, s.value, s.tags_json, s.collected_at
			FROM snmp_metric_samples s
			WHERE s.metric_name = ?
			ORDER BY s.collected_at DESC LIMIT 200`, metricName)
		if err == nil {
			defer rows.Close()
			seenSeries := make(map[string]bool)
			for rows.Next() {
				var devID, mName, tagsJSON, cAt string
				var val float64
				if err := rows.Scan(&devID, &mName, &val, &tagsJSON, &cAt); err != nil {
					continue
				}
				if !isSampleFresh(cAt, nowTs) {
					continue
				}
				var sampleTags map[string]string
				if json.Unmarshal([]byte(tagsJSON), &sampleTags) != nil {
					sampleTags = nil
				}
				ifName := sampleTags["ifDescr"]
				if ifName == "" {
					ifName = sampleTags["ifName"]
				}
				// 同一设备同一序列只取最新一条（查询已按时间倒序）
				seriesKey := devID + "|" + mName + "|" + ifName
				if seenSeries[seriesKey] {
					continue
				}
				seenSeries[seriesKey] = true

				info := devices[devID]
				tags := map[string]string{
					"device_id":   devID,
					"device_ip":   info.IP,
					"device_name": info.Name,
					"vendor":      info.Vendor,
					"if_name":     ifName,
				}
				for k, v := range sampleTags {
					tags[k] = v
				}
				items = append(items, SampleEvaluationItem{
					Value:       val,
					DeviceID:    devID,
					DeviceIP:    info.IP,
					DeviceName:  info.Name,
					Vendor:      info.Vendor,
					MetricName:  mName,
					IfName:      ifName,
					MetricLabel: mName,
					Unit:        sampleTags["unit"],
					Tags:        tags,
				})
			}
		}
	}

	return items
}

func stringsToFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

// MatchTagFilters 校验样本的 Tags 是否符合规则要求
func MatchTagFilters(sampleTags map[string]string, filters []repos.TagFilterItem) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if f.Key == "" {
			continue
		}
		tagVal, exists := sampleTags[f.Key]
		if !exists {
			tagVal = ""
		}
		switch f.Op {
		case "=":
			if tagVal != f.Value {
				return false
			}
		case "!=":
			if tagVal == f.Value {
				return false
			}
		case "=~":
			matched, _ := regexp.MatchString(f.Value, tagVal)
			if !matched {
				return false
			}
		case "!~":
			matched, _ := regexp.MatchString(f.Value, tagVal)
			if matched {
				return false
			}
		}
	}
	return true
}

// CheckThresholdCondition 校验值比较运算符
func CheckThresholdCondition(val float64, op string, threshold float64) bool {
	switch op {
	case ">":
		return val > threshold
	case ">=":
		return val >= threshold
	case "<":
		return val < threshold
	case "<=":
		return val <= threshold
	case "==":
		return val == threshold
	case "!=":
		return val != threshold
	default:
		return val > threshold
	}
}

func triggerAlertRecord(db *sql.DB, rule repos.AlertRuleRecord, sample SampleEvaluationItem, nowTs int64) {
	// 主键使用 uuid，避免同一毫秒内多条告警 ID 冲突
	alertID := uuid.NewString()
	title := fmt.Sprintf("%s 触发告警", rule.Name)
	message := rule.Template
	if message == "" {
		message = fmt.Sprintf("指标 [%s] 当前采样值为 %.2f，达到触发条件 (%s %.2f)", rule.MetricName, sample.Value, rule.Operator, rule.Threshold)
	}

	message = strings.ReplaceAll(message, "${device_name}", sample.DeviceName)
	message = strings.ReplaceAll(message, "${device_ip}", sample.DeviceIP)
	message = strings.ReplaceAll(message, "${if_name}", sample.IfName)
	message = strings.ReplaceAll(message, "${value}", fmt.Sprintf("%.2f", sample.Value))

	nowStr := time.Now().Format("2006-01-02 15:04:05")
	metaBytes, _ := json.Marshal(map[string]interface{}{
		"device_id":   sample.DeviceID,
		"device_ip":   sample.DeviceIP,
		"device_name": sample.DeviceName,
		"metric_name": sample.MetricName,
		"value":       sample.Value,
		"tags":        sample.Tags,
		"rule_id":     rule.ID,
		"promql":      rule.PromQL,
	})

	_, err := db.Exec(`
		INSERT INTO alerts (id, source, severity, title, content, metadata, status, device_id, created_at, updated_at)
		VALUES (?, 'promql_alert', ?, ?, ?, ?, 'firing', ?, ?, ?)`,
		alertID, rule.Severity, title, message, string(metaBytes), sample.DeviceID, nowStr, nowStr,
	)
	if err != nil {
		logger.Warnf("Failed to insert firing alert record: %v", err)
	} else {
		logger.Infof("⚡ Alert Triggered: [%s] %s - %s", rule.Severity, title, message)
		dispatchInfo := ops.AlertDispatchInfo{
			AlertID:    alertID,
			Source:     "promql_alert",
			Title:      title,
			Message:    message,
			Severity:   rule.Severity,
			MetricName: sample.MetricName,
			Value:      sample.Value,
			DeviceID:   sample.DeviceID,
			DeviceName: sample.DeviceName,
			DeviceIP:   sample.DeviceIP,
			Tags:       sample.Tags,
			Status:     "firing",
		}
		ops.HandleAlertForRemediation(dispatchInfo)
		if rule.NotifyRemote {
			ops.DispatchAlertNotification(dispatchInfo)
		}
	}
}
