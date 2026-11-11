// 分析聚合路由 — 等价于 TS linkageRoutes.ts + dashboardRoutes.ts 的分析端点
package routes

import (
	"database/sql"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
)

// InspectionResult 巡检中心结果项
type InspectionResult struct {
	ID         string  `json:"id"`
	DeviceID   string  `json:"device_id"`
	Source     string  `json:"source"`
	Type       string  `json:"type"`
	Status     string  `json:"status"`
	Summary    *string `json:"summary"`
	DeviceName *string `json:"device_name"`
	DeviceIP   *string `json:"device_ip"`
	DurationMs *int64  `json:"duration_ms"`
	CreatedAt  string  `json:"created_at"`
}

// AnalyticsRoutes 注册所有分析聚合端点 + 告警端点
// 告警端点也挂载在 /api/v1 下，因此合并到同一个 router
func AnalyticsRoutes() chi.Router {
	r := chi.NewRouter()

	// ── 分析聚合 ──
	r.Get("/dashboard/linkage", handleDashboardLinkage)
	r.Get("/dashboard/network-device-stats", handleNetworkDeviceStats)

	// ── SNMP 指标命名体系 + Grafana 式面板编辑器 ──
	r.Mount("/snmp-metrics", SnmpMetricDashboardRoutes())
	r.Get("/inspection-center", handleInspectionCenter)
	r.Get("/device/{id}/overview", handleDeviceOverview)
	r.Get("/trends/inspection-history", handleInspectionHistoryTrend)
	r.Get("/trends/summary", handleTrendSummary)

	// ── 服务器监控大屏 ──
	r.Get("/dashboard/full", handleDashboardFull)
	r.Get("/dashboard/server-metrics", handleServerMetrics)
	r.Get("/dashboard/alert-trends", handleAlertTrends)
	r.Get("/dashboard/task-trends", handleTaskTrends)
	r.Get("/dashboard/agent-stats", handleAgentStats)
	r.Get("/dashboard/task-distribution", handleTaskDistribution)
	r.Get("/dashboard/remediation-stats", handleRemediationStats)
	r.Get("/dashboard/alert-source-stats", handleAlertSourceStats)
	r.Get("/dashboard/sla-stats", handleSlaStats)

	// ── 告警核心 & 夜莺规则引擎 ──
	RegisterAlertRuleRoutes(r)
	r.Get("/alerts", handleListAlerts)
	r.With(middleware.RequireRole("admin")).Put("/alerts/{alertId}/acknowledge", handleAcknowledgeAlert)
	r.With(middleware.RequireRole("admin")).Put("/alerts/{alertId}/resolve", handleResolveAlert)
	r.With(middleware.RequireRole("admin")).Post("/alerts/{alertId}/process", handleProcessAlert)
	r.Get("/alerts/{alertId}/automation-logs", handleAlertAutomationLogs)

	// ── 告警源配置 (Providers) ──
	r.Get("/alerts/providers/list", handleListProviders)
	r.Get("/alerts/providers/configs", handleListProviderConfigs)
	r.With(middleware.RequireRole("admin")).Post("/alerts/providers/configs", handleCreateProviderConfig)
	r.With(middleware.RequireRole("admin")).Put("/alerts/providers/configs/{id}", handleUpdateProviderConfig)
	r.With(middleware.RequireRole("admin")).Delete("/alerts/providers/configs/{id}", handleDeleteProviderConfig)
	r.With(middleware.RequireRole("admin")).Post("/alerts/providers/fetch", handleFetchProvider)

	// ── 自动分析 ──
	r.Get("/alert-auto-analysis", handleListAutoAnalysis)

	// ── 告警关联 ──
	r.Get("/alert-correlation/groups", handleListCorrelationGroups)
	r.Get("/alert-correlation/stats", handleCorrelationStats)
	r.Get("/alert-correlation/groups/{groupId}", handleGetCorrelationGroup)
	r.With(middleware.RequireRole("admin")).Post("/alert-correlation/groups/{groupId}/resolve", handleResolveCorrelationGroup)
	r.With(middleware.RequireRole("admin")).Delete("/alert-correlation/groups/{groupId}", handleDeleteCorrelationGroup)
	r.With(middleware.RequireRole("admin")).Post("/alert-correlation/auto", handleAutoCorrelate)

	// ── 告警映射 ──
	r.Get("/alert-mappings", handleListMappings)
	r.With(middleware.RequireRole("admin")).Post("/alert-mappings", handleCreateMapping)
	r.With(middleware.RequireRole("admin")).Put("/alert-mappings/{id}", handleUpdateMapping)
	r.With(middleware.RequireRole("admin")).Delete("/alert-mappings/{id}", handleDeleteMapping)

	// ── 告警降噪 ──
	r.Get("/alert-noise/stats", handleNoiseStats)
	r.Get("/alert-noise", handleNoiseStats)
	r.Get("/alert-noise/suppressed", handleListSuppressed)
	r.With(middleware.RequireRole("admin")).Post("/alert-noise/suppress", handleSuppressAlert)
	r.With(middleware.RequireRole("admin")).Post("/alert-noise/unsuppress", handleUnsuppressAlert)
	r.With(middleware.RequireRole("admin")).Post("/alert-noise/cleanup", handleCleanupNoise)

	// ── 告警路由别名 ──
	r.Get("/alert-groups", handleListCorrelationGroups)
	r.Get("/alert-providers", handleListProviders)
	r.Get("/alert-escalation-history", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT id, alert_id, rule_id, from_level, to_level, trigger_reason, notified_users, created_at FROM escalation_history ORDER BY created_at DESC LIMIT 50`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()
		var list []map[string]interface{}
		for rows.Next() {
			var id, alertID, ruleID, reason, createdAt string
			var fromLvl, toLvl int
			var users sql.NullString
			if rows.Scan(&id, &alertID, &ruleID, &fromLvl, &toLvl, &reason, &users, &createdAt) == nil {
				list = append(list, map[string]interface{}{
					"id":             id,
					"alert_id":       alertID,
					"rule_id":        ruleID,
					"from_level":     fromLvl,
					"to_level":       toLvl,
					"trigger_reason": reason,
					"notified_users": users.String,
					"created_at":     createdAt,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	return r
}

// handleDashboardLinkage 仪表盘联动统计
func handleDashboardLinkage(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()
	type stat struct {
		C int `json:"c"`
	}

	alertTotal := 0
	db.QueryRow("SELECT COUNT(*) FROM alerts").Scan(&alertTotal)

	openAlerts := 0
	db.QueryRow("SELECT COUNT(*) FROM alerts WHERE status != 'resolved'").Scan(&openAlerts)

	analysisTotal := 0
	db.QueryRow("SELECT COUNT(*) FROM alert_auto_analysis").Scan(&analysisTotal)

	inspectionTotal := 0
	db.QueryRow("SELECT COUNT(*) FROM network_inspection_history").Scan(&inspectionTotal)

	executionTotal := 0
	db.QueryRow("SELECT COUNT(*) FROM remediation_executions").Scan(&executionTotal)

	deviceTotal := 0
	db.QueryRow("SELECT COUNT(*) FROM network_devices").Scan(&deviceTotal)

	serverTotal := 0
	db.QueryRow("SELECT COUNT(*) FROM servers").Scan(&serverTotal)

	httpx.Respond(w, map[string]interface{}{
		"alerts":       map[string]interface{}{"total": alertTotal, "open": openAlerts},
		"analyses":     map[string]interface{}{"total": analysisTotal},
		"inspections":  map[string]interface{}{"total": inspectionTotal},
		"remediations": map[string]interface{}{"total": executionTotal},
		"devices":      map[string]interface{}{"network_devices": deviceTotal, "servers": serverTotal},
	})
}

// handleNetworkDeviceStats 网络设备仪表盘 (严格使用真实设备及真实采集指标)
func handleNetworkDeviceStats(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()
	metricsDB := appdb.DBForTable("snmp_interface_metrics")

	// 1. 设备统计 (真实设备)
	var total, online, offline, snmpEnabled int
	_ = db.QueryRow(`
		SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN status = 'online' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status != 'online' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN snmp_enabled = 1 THEN 1 ELSE 0 END), 0)
		FROM network_devices`).Scan(&total, &online, &offline, &snmpEnabled)

	// 2. 接口统计 (使用窗口函数提取各个设备最新接口状态)
	var totalUp, totalDown, totalErrors int
	var totalUpNull, totalDownNull, totalErrorsNull sql.NullInt64
	_ = metricsDB.QueryRow(`
		SELECT
			SUM(CASE WHEN if_oper_status = 1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN if_oper_status = 2 THEN 1 ELSE 0 END),
			SUM(CASE WHEN COALESCE(if_in_errors, 0) > 0 OR COALESCE(if_out_errors, 0) > 0 THEN 1 ELSE 0 END)
		FROM snmp_interface_metrics
		WHERE sampled_at IN (SELECT MAX(sampled_at) FROM snmp_interface_metrics GROUP BY device_id)
	`).Scan(&totalUpNull, &totalDownNull, &totalErrorsNull)

	totalUp = int(totalUpNull.Int64)
	totalDown = int(totalDownNull.Int64)
	totalErrors = int(totalErrorsNull.Int64)

	// 3. 真实设备的流量 Top 统计 (基于前后 2 次真实采样点差值计算 Mbps)
	type TopDevice struct {
		DeviceID       string  `json:"device_id"`
		DeviceName     string  `json:"device_name"`
		Vendor         string  `json:"vendor"`
		TotalTraffic   float64 `json:"total_traffic_mbps"`
		InterfaceCount int     `json:"interface_count"`
	}
	topDevices := make([]TopDevice, 0)
	var totalInMbps, totalOutMbps float64

	rows, err := metricsDB.Query(`
		WITH Ranked AS (
			SELECT 
				device_id, if_index, if_in_octets, if_out_octets, sampled_at,
				ROW_NUMBER() OVER (PARTITION BY device_id, if_index ORDER BY sampled_at DESC) as rn
			FROM snmp_interface_metrics
		)
		SELECT 
			cur.device_id,
			COALESCE(SUM(cur.if_in_octets - prev.if_in_octets), 0) as delta_in,
			COALESCE(SUM(cur.if_out_octets - prev.if_out_octets), 0) as delta_out,
			COALESCE(MAX((julianday(cur.sampled_at) - julianday(prev.sampled_at)) * 86400), 0) as sec,
			COUNT(DISTINCT cur.if_index) as if_count
		FROM Ranked cur
		JOIN Ranked prev ON cur.device_id = prev.device_id AND cur.if_index = prev.if_index AND prev.rn = 2
		WHERE cur.rn = 1
		  AND cur.if_in_octets >= prev.if_in_octets AND cur.if_out_octets >= prev.if_out_octets
		GROUP BY cur.device_id
		ORDER BY (delta_in + delta_out) DESC
		LIMIT 8`)

	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			var deltaIn, deltaOut, sec float64
			var ifCount int
			if err := rows.Scan(&id, &deltaIn, &deltaOut, &sec, &ifCount); err == nil {
				// 设备名称/厂商在 app 库中查询（指标库与 app 库不跨库 JOIN）
				var name, vendor string
				_ = db.QueryRow(`SELECT COALESCE(name, ''), COALESCE(vendor, 'other') FROM network_devices WHERE id = ?`, id).Scan(&name, &vendor)
				inMbps := 0.0
				outMbps := 0.0
				if sec >= 1 {
					inMbps = roundTo2(deltaIn * 8 / (sec * 1_000_000))
					outMbps = roundTo2(deltaOut * 8 / (sec * 1_000_000))
					if inMbps > 100_000 {
						inMbps = 0
					}
					if outMbps > 100_000 {
						outMbps = 0
					}
				}
				mbps := roundTo2(inMbps + outMbps)
				totalInMbps = roundTo2(totalInMbps + inMbps)
				totalOutMbps = roundTo2(totalOutMbps + outMbps)

				topDevices = append(topDevices, TopDevice{
					DeviceID:       id,
					DeviceName:     name,
					Vendor:         vendor,
					TotalTraffic:   mbps,
					InterfaceCount: ifCount,
				})
			}
		}
	}

	// 4. 最近真实巡检记录
	type RecentInspection struct {
		ID             string `json:"id"`
		DeviceName     string `json:"device_name"`
		InspectionType string `json:"inspection_type"`
		Status         string `json:"status"`
		DurationMs     *int64 `json:"duration_ms"`
		CreatedAt      string `json:"created_at"`
	}
	recentInspections := make([]RecentInspection, 0)
	iRows, err := db.Query(`
		SELECT h.id, d.name, h.inspection_type, h.status, h.duration_ms, h.created_at
		FROM network_inspection_history h
		JOIN network_devices d ON h.device_id = d.id
		ORDER BY h.created_at DESC LIMIT 5`)
	if err == nil {
		defer iRows.Close()
		for iRows.Next() {
			var ins RecentInspection
			var durMs sql.NullInt64
			if err := iRows.Scan(&ins.ID, &ins.DeviceName, &ins.InspectionType, &ins.Status, &durMs, &ins.CreatedAt); err == nil {
				if durMs.Valid {
					ins.DurationMs = &durMs.Int64
				}
				recentInspections = append(recentInspections, ins)
			}
		}
	}

	// 5. 真实厂商分布
	type VendorCount struct {
		Vendor string `json:"vendor"`
		Count  int    `json:"count"`
	}
	vendorDistribution := make([]VendorCount, 0)
	vRows, err := db.Query(`SELECT COALESCE(LOWER(vendor), 'other') as vendor, COUNT(*) as count FROM network_devices GROUP BY LOWER(vendor) ORDER BY count DESC`)
	if err == nil {
		defer vRows.Close()
		for vRows.Next() {
			var v VendorCount
			if vRows.Scan(&v.Vendor, &v.Count) == nil {
				vendorDistribution = append(vendorDistribution, v)
			}
		}
	}

	httpx.Respond(w, map[string]interface{}{
		"devices": map[string]interface{}{
			"total":       total,
			"online":      online,
			"offline":     offline,
			"snmpEnabled": snmpEnabled,
		},
		"interfaces": map[string]interface{}{
			"totalUp":     totalUp,
			"totalDown":   totalDown,
			"totalErrors": totalErrors,
		},
		"throughput": map[string]interface{}{
			"totalInMbps":  totalInMbps,
			"totalOutMbps": totalOutMbps,
			"totalMbps":    roundTo2(totalInMbps + totalOutMbps),
		},
		"topDevicesByTraffic": topDevices,
		"recentInspections":   recentInspections,
		"vendorDistribution":  vendorDistribution,
	})
}

// handleInspectionCenter 巡检中心
func handleInspectionCenter(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()
	deviceID := r.URL.Query().Get("deviceId")
	limit := 100
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	if limit > 200 {
		limit = 200
	}

	results := make([]InspectionResult, 0)

	// SNMP/SSH巡检记录
	q := `SELECT id, device_id, inspection_type, status, summary, duration_ms, created_at
		FROM network_inspection_history`
	args := make([]interface{}, 0)
	if deviceID != "" {
		q += " WHERE device_id = ?"
		args = append(args, deviceID)
	}
	q += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(q, args...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var summary sql.NullString
			var durMs sql.NullInt64
			var devID, inspType, st, createdAt string
			var id string
			if err := rows.Scan(&id, &devID, &inspType, &st, &summary, &durMs, &createdAt); err != nil {
				continue
			}
			item := InspectionResult{
				ID:        id,
				DeviceID:  devID,
				Source:    "inspection",
				Type:      inspType,
				Status:    st,
				CreatedAt: createdAt,
			}
			if summary.Valid {
				item.Summary = &summary.String
			}
			if durMs.Valid {
				item.DurationMs = &durMs.Int64
			}
			results = append(results, item)
		}
	}

	// 补充设备名称/IP
	for i := range results {
		if results[i].DeviceName == nil || results[i].DeviceIP == nil {
			var name, ip sql.NullString
			err := db.QueryRow("SELECT name, ip_address FROM network_devices WHERE id = ?", results[i].DeviceID).Scan(&name, &ip)
			if err == nil {
				if name.Valid {
					results[i].DeviceName = &name.String
				}
				if ip.Valid {
					results[i].DeviceIP = &ip.String
				}
			}
		}
	}

	httpx.Respond(w, map[string]interface{}{
		"results": results,
		"counts": map[string]interface{}{
			"total":       len(results),
			"inspections": len(results),
			"analyses":    0,
			"success":     countByStatus(results, "success"),
			"failed":      countByStatus(results, "failed"),
		},
	})
}

// handleDeviceOverview 设备概览
func handleDeviceOverview(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")
	db := appdb.DB()

	var name, ipAddr sql.NullString
	err := db.QueryRow("SELECT name, ip_address FROM network_devices WHERE id = ?", deviceID).Scan(&name, &ipAddr)
	if err != nil {
		httpx.RespondError(w, "设备不存在", http.StatusNotFound)
		return
	}

	n := name.String
	p := ipAddr.String
	httpx.Respond(w, map[string]interface{}{
		"device": map[string]interface{}{
			"id":   deviceID,
			"name": n,
			"ip":   p,
			"type": "network_device",
		},
		"alert_count":      0,
		"open_alert_count": 0,
		"alerts":           []interface{}{},
		"inspection_count": 0,
		"inspections":      []interface{}{},
		"analysis_count":   0,
		"analyses":         []interface{}{},
		"execution_count":  0,
		"executions":       []interface{}{},
	})
}

// handleInspectionHistoryTrend 巡检趋势
func handleInspectionHistoryTrend(w http.ResponseWriter, r *http.Request) {
	days := 30
	if v, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && v > 0 {
		days = v
	}
	db := appdb.DB()

	type TrendPoint struct {
		Date    string `json:"date"`
		Total   int    `json:"total"`
		Success int    `json:"success"`
		Failed  int    `json:"failed"`
	}

	rows, err := db.Query(`
		SELECT
			strftime('%Y-%m-%d', created_at) as date,
			COUNT(*) as total,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed
		FROM network_inspection_history
		WHERE created_at >= datetime('now', ? || ' days')
		GROUP BY date
		ORDER BY date ASC`, -days)
	if err != nil {
		httpx.RespondError(w, "查询趋势失败", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	data := make([]TrendPoint, 0)
	for rows.Next() {
		var p TrendPoint
		if rows.Scan(&p.Date, &p.Total, &p.Success, &p.Failed) == nil {
			data = append(data, p)
		}
	}

	httpx.Respond(w, data)
}

// handleTrendSummary 趋势总结
func handleTrendSummary(w http.ResponseWriter, r *http.Request) {
	days := 30
	if v, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && v > 0 {
		days = v
	}
	db := appdb.DB()

	var totalInspections, successInspections, failedInspections int
	db.QueryRow(`
		SELECT COUNT(*),
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END)
		FROM network_inspection_history
		WHERE created_at >= datetime('now', ? || ' days')`, -days).Scan(&totalInspections, &successInspections, &failedInspections)

	successRate := 0.0
	if totalInspections > 0 {
		successRate = math.Round(float64(successInspections)/float64(totalInspections)*1000) / 10
	}

	httpx.Respond(w, map[string]interface{}{
		"period":             days,
		"totalInspections":   totalInspections,
		"successInspections": successInspections,
		"failedInspections":  failedInspections,
		"successRate":        successRate,
	})
}

// --- 辅助函数 ---

func iv(v sql.NullInt64) int {
	if v.Valid {
		return int(v.Int64)
	}
	return 0
}

func countByStatus(results []InspectionResult, status string) int {
	c := 0
	for _, r := range results {
		if r.Status == status {
			c++
		}
	}
	return c
}

// ── 告警处理函数 ──

func handleListAlerts(w http.ResponseWriter, r *http.Request) {
	f := repos.AlertFilter{
		Status:   r.URL.Query().Get("status"),
		Severity: r.URL.Query().Get("severity"),
		Source:   r.URL.Query().Get("source"),
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		f.Limit, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		f.Offset, _ = strconv.Atoi(v)
	}
	alerts, err := repos.Alerts.List(f)
	if err != nil {
		httpx.RespondError(w, "获取告警列表失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, alerts)
}

func handleAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "alertId")
	if err := repos.Alerts.UpdateStatus(id, "acknowledged"); err != nil {
		httpx.RespondError(w, "更新告警状态失败", http.StatusInternalServerError)
		return
	}
	httpx.RespondMessage(w, "acknowledged")
}

func handleResolveAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "alertId")
	if err := repos.Alerts.UpdateStatus(id, "resolved"); err != nil {
		httpx.RespondError(w, "更新告警状态失败", http.StatusInternalServerError)
		return
	}
	httpx.RespondMessage(w, "resolved")
}

func handleProcessAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "alertId")
	httpx.Respond(w, map[string]interface{}{
		"alertId":         id,
		"matchedPolicies": []interface{}{},
		"mappingTasks":    []interface{}{},
		"executionIds":    []string{},
		"error":           nil,
	})
}

func handleAlertAutomationLogs(w http.ResponseWriter, r *http.Request) {
	alertID := chi.URLParam(r, "alertId")
	logs, err := repos.AarsResponseLogs.ListByAlert(alertID)
	if err != nil {
		httpx.RespondError(w, "获取自动响应日志失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, logs)
}

func handleListProviders(w http.ResponseWriter, r *http.Request) {
	providers := []map[string]interface{}{
		{"id": "zabbix", "name": "Zabbix", "type": "zabbix", "icon": "zabbix"},
		{"id": "prometheus", "name": "Prometheus AlertManager", "type": "prometheus", "icon": "prometheus"},
		{"id": "webhook", "name": "Webhook", "type": "webhook", "icon": "webhook"},
		{"id": "snmp_trap", "name": "SNMP Trap", "type": "snmp_trap", "icon": "snmp"},
		{"id": "syslog", "name": "Syslog", "type": "syslog", "icon": "syslog"},
	}
	httpx.Respond(w, providers)
}

func handleListProviderConfigs(w http.ResponseWriter, r *http.Request) {
	configs, err := repos.AlertProviderConfigs.List()
	if err != nil {
		httpx.RespondError(w, "获取告警源配置失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, configs)
}

func handleCreateProviderConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProviderID string `json:"provider_id"`
		Name       string `json:"name"`
		Config     string `json:"config"`
		Enabled    *bool  `json:"enabled"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	if req.ProviderID == "" || req.Name == "" {
		httpx.RespondError(w, "provider_id 和 name 为必填项", http.StatusBadRequest)
		return
	}
	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}
	c := repos.AlertProviderConfig{
		ID:         uuid.New().String(),
		ProviderID: req.ProviderID,
		Name:       req.Name,
		Config:     nilSafeStrPtr(req.Config),
		Enabled:    enabled,
	}
	if err := repos.AlertProviderConfigs.Create(c); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, map[string]string{"id": c.ID})
}

func handleUpdateProviderConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		ProviderID string `json:"provider_id"`
		Name       string `json:"name"`
		Config     string `json:"config"`
		Enabled    *bool  `json:"enabled"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}
	c := repos.AlertProviderConfig{
		ProviderID: req.ProviderID,
		Name:       req.Name,
		Config:     nilSafeStrPtr(req.Config),
		Enabled:    enabled,
	}
	if err := repos.AlertProviderConfigs.Update(id, c); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.RespondMessage(w, "updated")
}

func handleDeleteProviderConfig(w http.ResponseWriter, r *http.Request) {
	if err := repos.AlertProviderConfigs.Delete(chi.URLParam(r, "id")); err != nil {
		httpx.RespondError(w, "删除告警源配置失败", http.StatusInternalServerError)
		return
	}
	httpx.RespondMessage(w, "deleted")
}

func handleFetchProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string                 `json:"provider"`
		Config   map[string]interface{} `json:"config"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	httpx.Respond(w, map[string]interface{}{
		"provider": req.Provider,
		"fetched":  0,
		"message":  "Provider fetch completed (placeholder)",
	})
}

func handleListAutoAnalysis(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	alertID := r.URL.Query().Get("alertId")
	items, err := repos.AlertAutoAnalyses.List(limit, alertID)
	if err != nil {
		httpx.RespondError(w, "获取自动分析列表失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, items)
}

func handleListCorrelationGroups(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	groups, err := repos.AlertCorrelations.ListGroups(status, limit)
	if err != nil {
		httpx.RespondError(w, "获取关联组失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, groups)
}

func handleCorrelationStats(w http.ResponseWriter, r *http.Request) {
	stats, err := repos.AlertCorrelations.GetStats()
	if err != nil {
		httpx.RespondError(w, "获取关联统计失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, stats)
}

func handleGetCorrelationGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupId")
	group, err := repos.AlertCorrelations.GetGroupByID(groupID)
	if err != nil || group == nil {
		httpx.RespondError(w, "关联组不存在", http.StatusNotFound)
		return
	}
	members, _ := repos.AlertCorrelations.GetGroupMembers(groupID)
	httpx.Respond(w, map[string]interface{}{
		"group":   group,
		"members": members,
	})
}

func handleResolveCorrelationGroup(w http.ResponseWriter, r *http.Request) {
	if err := repos.AlertCorrelations.ResolveGroup(chi.URLParam(r, "groupId")); err != nil {
		httpx.RespondError(w, "解决关联组失败", http.StatusInternalServerError)
		return
	}
	httpx.RespondMessage(w, "resolved")
}

func handleDeleteCorrelationGroup(w http.ResponseWriter, r *http.Request) {
	if err := repos.AlertCorrelations.DeleteGroup(chi.URLParam(r, "groupId")); err != nil {
		httpx.RespondError(w, "删除关联组失败", http.StatusInternalServerError)
		return
	}
	httpx.RespondMessage(w, "deleted")
}

func handleAutoCorrelate(w http.ResponseWriter, r *http.Request) {
	httpx.Respond(w, map[string]interface{}{
		"groupsCreated":   0,
		"alertsProcessed": 0,
		"message":         "Auto correlation completed (placeholder)",
	})
}

func handleListMappings(w http.ResponseWriter, r *http.Request) {
	items, err := repos.AlertMappings.List()
	if err != nil {
		httpx.RespondError(w, "获取告警映射失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, items)
}

func handleCreateMapping(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AlertSource       string `json:"alert_source"`
		AlertSeverity     string `json:"alert_severity"`
		AlertTitlePattern string `json:"alert_title_pattern"`
		WorkflowID        string `json:"workflow_id"`
		Enabled           *bool  `json:"enabled"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	if req.WorkflowID == "" {
		httpx.RespondError(w, "workflow_id 为必填项", http.StatusBadRequest)
		return
	}
	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}
	m := repos.AlertMapping{
		ID:                uuid.New().String(),
		AlertSource:       nilSafeStrPtr(req.AlertSource),
		AlertSeverity:     nilSafeStrPtr(req.AlertSeverity),
		AlertTitlePattern: nilSafeStrPtr(req.AlertTitlePattern),
		WorkflowID:        req.WorkflowID,
		Enabled:           enabled,
	}
	if err := repos.AlertMappings.Create(m); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, map[string]string{"id": m.ID})
}

func handleUpdateMapping(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		AlertSource       string `json:"alert_source"`
		AlertSeverity     string `json:"alert_severity"`
		AlertTitlePattern string `json:"alert_title_pattern"`
		WorkflowID        string `json:"workflow_id"`
		Enabled           *bool  `json:"enabled"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}
	m := repos.AlertMapping{
		AlertSource:       nilSafeStrPtr(req.AlertSource),
		AlertSeverity:     nilSafeStrPtr(req.AlertSeverity),
		AlertTitlePattern: nilSafeStrPtr(req.AlertTitlePattern),
		WorkflowID:        req.WorkflowID,
		Enabled:           enabled,
	}
	if err := repos.AlertMappings.Update(id, m); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.RespondMessage(w, "updated")
}

func handleDeleteMapping(w http.ResponseWriter, r *http.Request) {
	if err := repos.AlertMappings.Delete(chi.URLParam(r, "id")); err != nil {
		httpx.RespondError(w, "删除告警映射失败", http.StatusInternalServerError)
		return
	}
	httpx.RespondMessage(w, "deleted")
}

func handleNoiseStats(w http.ResponseWriter, r *http.Request) {
	stats, err := repos.AlertNoise.GetStats()
	if err != nil {
		httpx.RespondError(w, "获取降噪统计失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, stats)
}

func handleListSuppressed(w http.ResponseWriter, r *http.Request) {
	items, err := repos.AlertNoise.ListSuppressed()
	if err != nil {
		httpx.RespondError(w, "获取已压制告警失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, items)
}

func handleSuppressAlert(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Fingerprint     string `json:"fingerprint"`
		Reason          string `json:"reason"`
		DurationMinutes int    `json:"durationMinutes"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	if req.Fingerprint == "" {
		httpx.RespondError(w, "fingerprint 为必填项", http.StatusBadRequest)
		return
	}
	if err := repos.AlertNoise.Suppress(req.Fingerprint, req.Reason, req.DurationMinutes); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.RespondMessage(w, "suppressed")
}

func handleUnsuppressAlert(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Fingerprint string `json:"fingerprint"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	if err := repos.AlertNoise.Unsuppress(req.Fingerprint); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.RespondMessage(w, "unsuppressed")
}

func handleCleanupNoise(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DaysToKeep int `json:"daysToKeep"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	if err := repos.AlertNoise.Cleanup(req.DaysToKeep); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.RespondMessage(w, "cleaned up")
}

// nilSafeStrPtr 辅助：空字符串返回 nil 指针
func nilSafeStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
