package routes

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

func RegisterAlertRuleRoutes(r chi.Router) {
	r.Get("/alert-rules", handleListAlertRules)
	r.Post("/alert-rules", handleCreateAlertRule)
	r.Put("/alert-rules/{id}", handleUpdateAlertRule)
	r.Delete("/alert-rules/{id}", handleDeleteAlertRule)
	r.Get("/alert-rules/available-metrics", handleGetAvailableMetrics)
	r.Get("/alert-rules/available-tags", handleGetAvailableTags)
	r.Post("/alert-rules/preview-match", handlePreviewAlertMatch)
	r.Post("/alert-rules/seed-templates", handleSeedAlertTemplates)
}

func handleListAlertRules(w http.ResponseWriter, r *http.Request) {
	rules, err := repos.AlertRules.List()
	if err != nil {
		httpx.RespondError(w, "查询告警规则失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, rules)
}

func handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	var payload repos.AlertRuleRecord
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.RespondError(w, "无效的参数请求", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.Name) == "" {
		httpx.RespondError(w, "规则名称不能为空", http.StatusBadRequest)
		return
	}
	if payload.ID == "" {
		payload.ID = uuid.NewString()
	}

	// 如果提供了 PromQL，解析补充字段
	if strings.TrimSpace(payload.PromQL) != "" {
		pMetric, pTags, pOp, pTh, ok := parsePromQLExpression(payload.PromQL)
		if ok {
			if payload.MetricName == "" {
				payload.MetricName = pMetric
			}
			if len(payload.TagsFilter) == 0 && len(pTags) > 0 {
				payload.TagsFilter = pTags
			}
			if payload.Operator == "" && pOp != "" {
				payload.Operator = pOp
			}
			if payload.Threshold == 0 && pTh != 0 {
				payload.Threshold = pTh
			}
		}
	} else if payload.MetricName != "" {
		// 根据字段自动生成 PromQL
		var tagParts []string
		for _, tf := range payload.TagsFilter {
			if tf.Key != "" {
				op := tf.Op
				if op == "" {
					op = "="
				}
				tagParts = append(tagParts, fmt.Sprintf(`%s%s"%s"`, tf.Key, op, tf.Value))
			}
		}
		tagStr := ""
		if len(tagParts) > 0 {
			tagStr = "{" + strings.Join(tagParts, ", ") + "}"
		}
		op := payload.Operator
		if op == "" {
			op = ">"
		}
		payload.PromQL = fmt.Sprintf("%s%s %s %.2f", payload.MetricName, tagStr, op, payload.Threshold)
	}

	if payload.MetricName == "" {
		payload.MetricName = "cpu_usage"
	}
	if payload.Severity == "" {
		payload.Severity = "warning"
	}
	if payload.Operator == "" {
		payload.Operator = ">"
	}
	if payload.CooldownMs <= 0 {
		payload.CooldownMs = 600000 // 默认 10 分钟静默
	}
	payload.Enabled = true

	if err := repos.AlertRules.Create(payload); err != nil {
		httpx.RespondError(w, "创建告警规则失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, payload)
}

func handleUpdateAlertRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httpx.RespondError(w, "缺失规则 ID", http.StatusBadRequest)
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.RespondError(w, "无效的参数请求", http.StatusBadRequest)
		return
	}

	existing, err := repos.AlertRules.GetByID(id)
	if err != nil {
		httpx.RespondError(w, "找不到目标规则", http.StatusNotFound)
		return
	}

	// 快捷开关切换支持
	if enabledVal, ok := req["enabled"].(bool); ok && len(req) == 1 {
		if err := repos.AlertRules.Toggle(id, enabledVal); err != nil {
			httpx.RespondError(w, "更新状态失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]bool{"success": true})
		return
	}

	// 完整修改
	if name, ok := req["name"].(string); ok && name != "" {
		existing.Name = name
	}
	if desc, ok := req["description"].(string); ok {
		existing.Description = desc
	}
	if sev, ok := req["severity"].(string); ok {
		existing.Severity = sev
	}
	if mName, ok := req["metricName"].(string); ok {
		existing.MetricName = mName
	}
	if promql, ok := req["promql"].(string); ok {
		existing.PromQL = promql
	}
	if op, ok := req["operator"].(string); ok {
		existing.Operator = op
	}
	if th, ok := req["threshold"].(float64); ok {
		existing.Threshold = th
	}
	if forDur, ok := req["forDurationSec"].(float64); ok {
		existing.ForDurationSec = int(forDur)
	}
	if groupBy, ok := req["groupByTags"].(string); ok {
		existing.GroupByTags = groupBy
	}
	if cdMs, ok := req["cooldownMs"].(float64); ok {
		existing.CooldownMs = int64(cdMs)
	}
	if tmpl, ok := req["template"].(string); ok {
		existing.Template = tmpl
	}
	if enabledVal, ok := req["enabled"].(bool); ok {
		existing.Enabled = enabledVal
	}
	if nr, ok := req["notifyRemote"].(bool); ok {
		existing.NotifyRemote = nr
	} else if nr, ok := req["notify_remote"].(bool); ok {
		existing.NotifyRemote = nr
	}

	if tagsRaw, ok := req["tagsFilter"]; ok {
		bytes, _ := json.Marshal(tagsRaw)
		var tags []repos.TagFilterItem
		_ = json.Unmarshal(bytes, &tags)
		existing.TagsFilter = tags
	}

	if err := repos.AlertRules.Update(id, *existing); err != nil {
		httpx.RespondError(w, "更新告警规则失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, existing)
}

func handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := repos.AlertRules.Delete(id); err != nil {
		httpx.RespondError(w, "删除告警规则失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, map[string]bool{"success": true})
}

// Prometheus 格式动态指标列表结构
type MetricItemInfo struct {
	MetricName  string `json:"metricName"`
	Label       string `json:"label"`
	Category    string `json:"category"`
	Unit        string `json:"unit"`
	Description string `json:"description"`
}

func handleGetAvailableMetrics(w http.ResponseWriter, r *http.Request) {
	// 1. 核心网络大屏标准 Prometheus 指标
	baseMetrics := []MetricItemInfo{
		{MetricName: "device_status", Label: "设备在线综合状态 (Ping+SNMP)", Category: "网络大屏核心", Unit: "0/1", Description: "1 表示在线，0 表示离线/连通失败"},
		{MetricName: "cpu_usage", Label: "CPU 整体使用率 (%)", Category: "网络大屏核心", Unit: "%", Description: "设备 CPU 核心当前负载比例"},
		{MetricName: "memory_usage", Label: "内存整体使用率 (%)", Category: "网络大屏核心", Unit: "%", Description: "设备已用内存与物理内存比例"},
		{MetricName: "if_status", Label: "网络接口 UP/DOWN 状态", Category: "网络大屏核心", Unit: "1(UP)/2(DOWN)", Description: "接口物理与链路层运行状态"},
		{MetricName: "if_in_bps", Label: "接口入站实时流量 (bps)", Category: "网络大屏核心", Unit: "bps", Description: "网络接口每秒接收比特数"},
		{MetricName: "if_out_bps", Label: "接口出站实时流量 (bps)", Category: "网络大屏核心", Unit: "bps", Description: "网络接口每秒发送比特数"},
		{MetricName: "if_in_errors", Label: "接口入站错误包数 (个/s)", Category: "网络大屏核心", Unit: "个", Description: "接口接收的错包计数值"},
		{MetricName: "if_out_discards", Label: "接口出站丢包数 (个/s)", Category: "网络大屏核心", Unit: "个", Description: "接口发包丢弃数量"},
		{MetricName: "ping_rtt_ms", Label: "Ping 响应延迟 (ms)", Category: "网络大屏核心", Unit: "ms", Description: "设备 ICMP 往返延时"},
		{MetricName: "ping_loss_percent", Label: "Ping 丢包率 (%)", Category: "网络大屏核心", Unit: "%", Description: "ICMP 探测丢失包比例"},
	}

	// 2. 从指标库 (snmp_metric_definitions) 提取自定义 OID 指标
	defRows, err := appdb.DBForTable("snmp_metric_definitions").Query(`SELECT metric_name, description, unit, metric_type FROM snmp_metric_definitions LIMIT 200`)
	if err == nil {
		defer defRows.Close()
		for defRows.Next() {
			var mName, desc, unit, mType sql.NullString
			if err := defRows.Scan(&mName, &desc, &unit, &mType); err == nil && mName.String != "" {
				baseMetrics = append(baseMetrics, MetricItemInfo{
					MetricName:  mName.String,
					Label:       mName.String + " (" + desc.String + ")",
					Category:    "SNMP 自定义指标",
					Unit:        unit.String,
					Description: desc.String,
				})
			}
		}
	}

	httpx.Respond(w, baseMetrics)
}

func handleGetAvailableTags(w http.ResponseWriter, r *http.Request) {
	metricName := r.URL.Query().Get("metric")

	// 提取网络设备常用的全量 Tag 框架
	tagMap := map[string][]string{
		"env":            {"production", "staging", "development", "testing"},
		"cluster":        {"core-net", "access-net", "datacenter", "branch"},
		"role":           {"core-switch", "dist-switch", "access-switch", "border-router", "firewall"},
		"device_ip":      {},
		"device_name":    {},
		"vendor":         {"ZTE", "HUAWEI", "H3C", "CISCO", "RUIJIE", "MAIPU"},
		"rack":           {"A-01", "A-02", "B-01", "B-02"},
		"location":       {"核心机房A区", "汇聚机房B区", "数据中心", "边缘接入", "IDC-1", "IDC-2"},
		"if_name":        {},
		"if_index":       {},
		"module_name":    {"IF-MIB", "ZXROSNGMIB", "SYSTEM-MIB"},
		"severity_level": {"critical", "warning", "info"},
	}

	// 从网络设备表提取活的设备名称与 IP
	db := appdb.DB()
	devRows, err := db.Query(`SELECT ip_address, name, vendor FROM network_devices LIMIT 100`)
	if err == nil {
		defer devRows.Close()
		for devRows.Next() {
			var ip, name, vendor sql.NullString
			if err := devRows.Scan(&ip, &name, &vendor); err == nil {
				if ip.String != "" {
					tagMap["device_ip"] = append(tagMap["device_ip"], ip.String)
				}
				if name.String != "" {
					tagMap["device_name"] = append(tagMap["device_name"], name.String)
				}
				if vendor.String != "" {
					tagMap["vendor"] = append(tagMap["vendor"], vendor.String)
				}
			}
		}
	}

	// 从指标库 custom_dashboard_metrics 提取所有用户自定义的 Tag 键值
	custRows, err := appdb.DBForTable("custom_dashboard_metrics").Query(`SELECT COALESCE(tags, ''), COALESCE(custom_name, '') FROM custom_dashboard_metrics WHERE tags != '' OR custom_name != ''`)
	if err == nil {
		defer custRows.Close()
		for custRows.Next() {
			var cTags, cName string
			if err := custRows.Scan(&cTags, &cName); err == nil {
				if cName != "" {
					tagMap["metric_label"] = append(tagMap["metric_label"], cName)
				}
				if cTags != "" {
					var extra map[string]string
					if json.Unmarshal([]byte(cTags), &extra) == nil {
						for k, v := range extra {
							if _, exists := tagMap[k]; !exists {
								tagMap[k] = []string{}
							}
							tagMap[k] = append(tagMap[k], v)
						}
					}
				}
			}
		}
	}

	// 从采样点中提取常用的接口名称
	if metricName != "" {
		sampleRows, err := appdb.DBForTable("snmp_metric_samples").Query(`SELECT tag_name FROM snmp_metric_samples WHERE metric_name = ? AND tag_name != '' LIMIT 50`, metricName)
		if err == nil {
			defer sampleRows.Close()
			for sampleRows.Next() {
				var tName string
				if err := sampleRows.Scan(&tName); err == nil && tName != "" {
					tagMap["if_name"] = append(tagMap["if_name"], tName)
				}
			}
		}
	}

	// 去重处理
	for k, list := range tagMap {
		seen := make(map[string]bool)
		var unique []string
		for _, item := range list {
			item = strings.TrimSpace(item)
			if item != "" && !seen[item] {
				seen[item] = true
				unique = append(unique, item)
			}
		}
		tagMap[k] = unique
	}

	httpx.Respond(w, tagMap)
}

type PreviewMatchRequest struct {
	PromQL     string                `json:"promql"`
	MetricName string                `json:"metricName"`
	TagsFilter []repos.TagFilterItem `json:"tagsFilter"`
	Operator   string                `json:"operator"`
	Threshold  float64               `json:"threshold"`
}

type PreviewMatchResponse struct {
	MatchedCount int                             `json:"matchedCount"`
	TotalSamples int                             `json:"totalSamples"`
	MetricName   string                          `json:"metricName"`
	Operator     string                          `json:"operator"`
	Threshold    float64                         `json:"threshold"`
	Items        []services.SampleEvaluationItem `json:"items"`
}

var (
	promqlMainRegex = regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)(?:\{([^}]*)\})?(?:\s*([><=!]=?|==|!=)\s*([0-9.]+))?\s*$`)
	promqlTagRegex  = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*(=~|!~|!=|=)\s*["']?([^"']*)["']?`)
)

func parsePromQLExpression(q string) (metricName string, tags []repos.TagFilterItem, op string, th float64, ok bool) {
	q = strings.TrimSpace(q)
	matches := promqlMainRegex.FindStringSubmatch(q)
	if len(matches) == 0 {
		return "", nil, "", 0, false
	}
	metricName = matches[1]
	tagStr := matches[2]
	op = matches[3]
	if op == "" {
		op = ">"
	}
	thStr := matches[4]
	if thStr != "" {
		if val, err := strconv.ParseFloat(thStr, 64); err == nil {
			th = val
		}
	}

	if tagStr != "" {
		tagMatches := promqlTagRegex.FindAllStringSubmatch(tagStr, -1)
		for _, tm := range tagMatches {
			if len(tm) >= 4 {
				tags = append(tags, repos.TagFilterItem{
					Key:   tm[1],
					Op:    tm[2],
					Value: tm[3],
				})
			}
		}
	}
	return metricName, tags, op, th, true
}

func handlePreviewAlertMatch(w http.ResponseWriter, r *http.Request) {
	var req PreviewMatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.RespondError(w, "无效的参数格式", http.StatusBadRequest)
		return
	}

	metricName := req.MetricName
	tagsFilter := req.TagsFilter
	operator := req.Operator
	threshold := req.Threshold

	// 如果传入了 promql，进行解析补充
	if strings.TrimSpace(req.PromQL) != "" {
		pMetric, pTags, pOp, pTh, ok := parsePromQLExpression(req.PromQL)
		if ok {
			if metricName == "" || metricName == "cpu_usage" && pMetric != "" {
				metricName = pMetric
			}
			if len(tagsFilter) == 0 && len(pTags) > 0 {
				tagsFilter = pTags
			}
			if operator == "" || operator == ">" && pOp != "" {
				operator = pOp
			}
			if threshold == 0 && pTh != 0 {
				threshold = pTh
			}
		}
	}

	if operator == "" {
		operator = ">"
	}

	allSamples := services.CollectMetricCurrentSamples(metricName)

	var matchedItems []services.SampleEvaluationItem
	for _, sample := range allSamples {
		if services.MatchTagFilters(sample.Tags, tagsFilter) {
			sample.IsTriggered = services.CheckThresholdCondition(sample.Value, operator, threshold)
			matchedItems = append(matchedItems, sample)
		}
	}

	resp := PreviewMatchResponse{
		MatchedCount: len(matchedItems),
		TotalSamples: len(allSamples),
		MetricName:   metricName,
		Operator:     operator,
		Threshold:    threshold,
		Items:        matchedItems,
	}

	httpx.Respond(w, resp)
}

// 预置夜莺 / 大屏常用告警策略模板一键导入
func handleSeedAlertTemplates(w http.ResponseWriter, r *http.Request) {
	templates := []repos.AlertRuleRecord{
		{
			ID:             "tpl-device-offline",
			Name:           "[网络大屏] 设备断连离线告警",
			Description:    "当综合探测(Ping+SNMP)判定设备离线时触发紧急告警",
			Severity:       "critical",
			MetricName:     "device_status",
			Operator:       "==",
			Threshold:      0,
			ForDurationSec: 0,
			Enabled:        true,
			CooldownMs:     600000,
			Template:       "设备 ${device_name}(${device_ip}) 无法响应探测，已脱网离线！",
			CreatedAt:      time.Now().Format("2006-01-02 15:04:05"),
		},
		{
			ID:             "tpl-cpu-high",
			Name:           "[网络大屏] CPU 负载率过高 (>85%)",
			Description:    "连续 1 分钟 CPU 整体利用率超过 85% 阈值",
			Severity:       "warning",
			MetricName:     "cpu_usage",
			Operator:       ">",
			Threshold:      85,
			ForDurationSec: 60,
			Enabled:        true,
			CooldownMs:     1800000,
			Template:       "设备 ${device_name}(${device_ip}) CPU 利用率高达 ${value}%！",
			CreatedAt:      time.Now().Format("2006-01-02 15:04:05"),
		},
		{
			ID:             "tpl-memory-high",
			Name:           "[网络大屏] 内存使用率过高 (>90%)",
			Description:    "连续 1 分钟内存使用率超过 90% 阈值",
			Severity:       "warning",
			MetricName:     "memory_usage",
			Operator:       ">",
			Threshold:      90,
			ForDurationSec: 60,
			Enabled:        true,
			CooldownMs:     1800000,
			Template:       "设备 ${device_name}(${device_ip}) 内存使用率高达 ${value}%！",
			CreatedAt:      time.Now().Format("2006-01-02 15:04:05"),
		},
		{
			ID:             "tpl-if-down",
			Name:           "[网络大屏] 关键网络接口链路 DOWN",
			Description:    "核心交换机或路由端口 link 状态异常 Down 时立刻告警",
			Severity:       "critical",
			MetricName:     "if_status",
			Operator:       "==",
			Threshold:      2,
			ForDurationSec: 0,
			Enabled:        true,
			CooldownMs:     600000,
			Template:       "设备 ${device_name}(${device_ip}) 接口 ${if_name} 物理状态为 DOWN！",
			CreatedAt:      time.Now().Format("2006-01-02 15:04:05"),
		},
		{
			ID:             "tpl-if-in-bps-over",
			Name:           "[网络大屏] 端口入站流量超限 (>100Mbps)",
			Description:    "端口实时入流量超过 100Mbps (100000000 bps)",
			Severity:       "warning",
			MetricName:     "if_in_bps",
			Operator:       ">",
			Threshold:      100000000,
			ForDurationSec: 60,
			Enabled:        true,
			CooldownMs:     1800000,
			Template:       "设备 ${device_name} 接口 ${if_name} 入站流量突增至 ${value} bps",
			CreatedAt:      time.Now().Format("2006-01-02 15:04:05"),
		},
		{
			ID:             "tpl-if-out-bps-over",
			Name:           "[网络大屏] 端口出站流量超限 (>100Mbps)",
			Description:    "端口实时出流量超过 100Mbps (100000000 bps)",
			Severity:       "warning",
			MetricName:     "if_out_bps",
			Operator:       ">",
			Threshold:      100000000,
			ForDurationSec: 60,
			Enabled:        true,
			CooldownMs:     1800000,
			Template:       "设备 ${device_name} 接口 ${if_name} 出站流量突增至 ${value} bps",
			CreatedAt:      time.Now().Format("2006-01-02 15:04:05"),
		},
		{
			ID:             "tpl-ping-rtt-high",
			Name:           "[网络大屏] 网络 ICMP 往返延时过高 (>100ms)",
			Description:    "探测响应延迟持续超过 100ms 影响业务体验",
			Severity:       "info",
			MetricName:     "ping_rtt_ms",
			Operator:       ">",
			Threshold:      100,
			ForDurationSec: 60,
			Enabled:        true,
			CooldownMs:     1800000,
			Template:       "设备 ${device_name}(${device_ip}) 网络延迟增加至 ${value}ms",
			CreatedAt:      time.Now().Format("2006-01-02 15:04:05"),
		},
		{
			ID:             "tpl-ping-loss-high",
			Name:           "[网络大屏] 网络链路丢包率显著 (>10%)",
			Description:    "连续探测丢包率超过 10%",
			Severity:       "warning",
			MetricName:     "ping_loss_percent",
			Operator:       ">",
			Threshold:      10,
			ForDurationSec: 60,
			Enabled:        true,
			CooldownMs:     1800000,
			Template:       "设备 ${device_name}(${device_ip}) 链路线路丢包率达 ${value}%！",
			CreatedAt:      time.Now().Format("2006-01-02 15:04:05"),
		},
	}

	importedCount := 0
	for _, tpl := range templates {
		_ = repos.AlertRules.Create(tpl)
		importedCount++
	}

	httpx.Respond(w, map[string]interface{}{"success": true, "count": importedCount})
}
