// SNMP 指标命名体系 + Grafana 式面板编辑器 后端 API
//
// 所有端点挂在 /api/v1 下（由 AnalyticsRoutes 统一 Mount），避免 Chi 重复 Mount 同一路径。
// 提供：
//   - 指标定义列表 / 明细（snmp_exporter 风格命名）
//   - 面板 CRUD（dashboard_panels）
//   - 按面板配置聚合查询时间序列（供大屏渲染）
//   - generator.yml 风格配置解析（module + walk + lookups → 指标定义）
package routes

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

// SnmpMetricDashboardRoutes 注册 SNMP 指标命名 + 面板编辑器相关端点
// 调用方需在已 Mount 到 /api/v1 的 router 上注册（本函数返回的子路由挂在 /snmp-metrics 前缀下）
func SnmpMetricDashboardRoutes() chi.Router {
	r := chi.NewRouter()

	// GET /snmp-metrics/definitions — 指标定义列表（?module=&q=&limit=）
	r.Get("/definitions", func(w http.ResponseWriter, r *http.Request) {
		module := r.URL.Query().Get("module")
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		defs, err := repos.SnmpMetricDefs.Search(module, q, limit)
		if err != nil {
			httpx.RespondError(w, "获取指标定义失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, defs)
	})

	// GET /snmp-metrics/definitions/:name — 单个指标定义
	r.Get("/definitions/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		def, err := repos.SnmpMetricDefs.GetByMetricName(name)
		if err != nil {
			httpx.RespondError(w, "获取指标定义失败", http.StatusInternalServerError)
			return
		}
		if def == nil {
			httpx.RespondError(w, "指标定义不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, def)
	})

	// POST /snmp-metrics/definitions/seed — 手动触发种子（内置 IF-MIB + mib_objects 派生）
	r.With(middleware.RequireRole("admin")).Post("/definitions/seed", func(w http.ResponseWriter, r *http.Request) {
		if err := services.SeedSnmpMetricDefinitions(); err != nil {
			httpx.RespondError(w, "种子失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "指标定义种子完成")
	})

	// ── 面板（dashboard_panels）CRUD ──

	// GET /snmp-metrics/panels — 面板列表
	r.Get("/panels", func(w http.ResponseWriter, r *http.Request) {
		panels, err := repos.DashboardPanels.List()
		if err != nil {
			httpx.RespondError(w, "获取面板失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, panels)
	})

	// POST /snmp-metrics/panels — 创建面板
	r.With(middleware.RequireRole("admin")).Post("/panels", func(w http.ResponseWriter, r *http.Request) {
		var p repos.DashboardPanel
		if !parseJSON(w, r, &p) {
			return
		}
		if p.Title == "" || p.MetricName == "" {
			httpx.RespondError(w, "标题与指标名不能为空", http.StatusBadRequest)
			return
		}
		if p.ChartType == "" {
			p.ChartType = "line"
		}
		if p.Aggregation == "" {
			p.Aggregation = "avg"
		}
		if p.TimeRange == "" {
			p.TimeRange = "24h"
		}
		if err := repos.DashboardPanels.Create(&p); err != nil {
			httpx.RespondError(w, "创建面板失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, p)
	})

	// PUT /snmp-metrics/panels/:id — 更新面板
	r.With(middleware.RequireRole("admin")).Put("/panels/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var p repos.DashboardPanel
		if !parseJSON(w, r, &p) {
			return
		}
		if err := repos.DashboardPanels.Update(id, &p); err != nil {
			httpx.RespondError(w, "更新面板失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "面板已更新")
	})

	// DELETE /snmp-metrics/panels/:id — 删除面板
	r.With(middleware.RequireRole("admin")).Delete("/panels/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := repos.DashboardPanels.Delete(id); err != nil {
			httpx.RespondError(w, "删除面板失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "面板已删除")
	})

	// POST /snmp-metrics/panels/query — 按面板配置聚合查询时间序列
	r.Post("/panels/query", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MetricName  string            `json:"metric_name"`
			DeviceID    string            `json:"device_id"`
			Aggregation string            `json:"aggregation"`
			TagFilters  map[string]string `json:"tag_filters"`
			GroupBy     []string          `json:"group_by"`
			TimeRange   string            `json:"time_range"` // 1h|6h|24h|7d
			Hours       int               `json:"hours"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.MetricName == "" {
			httpx.RespondError(w, "指标名不能为空", http.StatusBadRequest)
			return
		}
		hours := req.Hours
		if hours <= 0 {
			hours = timeRangeToHours(req.TimeRange)
		}
		opt := repos.MetricQueryOptions{
			MetricName:  req.MetricName,
			DeviceID:    req.DeviceID,
			TagFilters:  req.TagFilters,
			GroupBy:     req.GroupBy,
			Aggregation: req.Aggregation,
			Hours:       hours,
		}
		series, err := repos.SnmpMetricSamples.QueryAggregated(opt)
		if err != nil {
			httpx.RespondError(w, "查询失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, series)
	})

	// POST /snmp-metrics/generator — 解析 generator.yml 风格配置
	r.With(middleware.RequireRole("admin")).Post("/generator", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Module  string                `json:"module"`
			Walk    []string              `json:"walk"`
			Lookups []services.SnmpLookup `json:"lookups"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Module == "" {
			httpx.RespondError(w, "module 不能为空", http.StatusBadRequest)
			return
		}
		created, err := services.GenerateDefinitionsFromGenerator(req.Module, req.Walk, req.Lookups)
		if err != nil {
			httpx.RespondError(w, "解析失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{
			"module":  req.Module,
			"created": created,
		})
	})

	// GET /snmp-metrics/export/prometheus — 导出真实采样数据的 Prometheus Exposition 格式
	// 无真实样本的指标不导出点位（不再填充占位 CPU/内存值）
	r.Get("/export/prometheus", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		var sb strings.Builder

		// 1. 设备信息（app 库）
		type promDevice struct {
			ID     string
			IP     string
			Name   string
			Vendor string
			Status string
		}
		devices := make([]promDevice, 0)
		devByID := make(map[string]promDevice)
		devRows, err := appdb.DB().Query(`SELECT id, ip_address, name, vendor, status FROM network_devices WHERE status != ''`)
		if err == nil {
			for devRows.Next() {
				var d promDevice
				if err := devRows.Scan(&d.ID, &d.IP, &d.Name, &d.Vendor, &d.Status); err == nil {
					devices = append(devices, d)
					devByID[d.ID] = d
				}
			}
			devRows.Close()
		}

		// 2. 用户自定义面板指标（指标库），仅使用真实 last_value
		type promCustom struct {
			DeviceID   string
			CustomName string
			Value      float64
			HasValue   bool
			OID        string
			MetricKey  string
			TagsJSON   string
		}
		customByDevice := make(map[string][]promCustom)
		cpuByDevice := make(map[string]float64)
		memByDevice := make(map[string]float64)

		custRows, cErr := appdb.DBForTable("custom_dashboard_metrics").Query(
			`SELECT device_id, custom_name, last_value, oid, COALESCE(metric_name, ''), COALESCE(tags, '') FROM custom_dashboard_metrics WHERE custom_name != ''`)
		if cErr == nil {
			for custRows.Next() {
				var devID, cName, lVal, cOid, mName, cTags string
				if err := custRows.Scan(&devID, &cName, &lVal, &cOid, &mName, &cTags); err != nil || cName == "" {
					continue
				}
				metricKey := strings.TrimSpace(mName)
				if metricKey == "" {
					if strings.Contains(strings.ToLower(cName), "cpu") {
						metricKey = "cpu_usage"
					} else if strings.Contains(cName, "内存") {
						metricKey = "memory_usage"
					} else {
						metricKey = "custom_metric"
					}
				}
				raw := strings.TrimSpace(lVal)
				if idx := strings.Index(raw, "="); idx != -1 {
					raw = raw[idx+1:]
				}
				val, parseErr := strconv.ParseFloat(strings.TrimSpace(raw), 64)
				hasVal := parseErr == nil

				if hasVal && metricKey == "cpu_usage" {
					cpuByDevice[devID] = val
				}
				if hasVal && metricKey == "memory_usage" {
					memByDevice[devID] = val
				}
				customByDevice[devID] = append(customByDevice[devID], promCustom{
					DeviceID:   devID,
					CustomName: cName,
					Value:      val,
					HasValue:   hasVal,
					OID:        cOid,
					MetricKey:  metricKey,
					TagsJSON:   cTags,
				})
			}
			custRows.Close()
		}

		// 3. 在线状态 + 真实 CPU/内存指标
		sb.WriteString("# HELP device_status 设备综合在线状态 (1=online, 0=offline)\n")
		sb.WriteString("# TYPE device_status gauge\n")
		for _, d := range devices {
			stVal := 0
			if d.Status == "online" {
				stVal = 1
			}
			sb.WriteString(fmt.Sprintf("device_status{device_ip=%q, device_name=%q, vendor=%q} %d\n", d.IP, d.Name, d.Vendor, stVal))
		}

		sb.WriteString("\n# HELP cpu_usage 设备 CPU 整体使用率 (%)\n")
		sb.WriteString("# TYPE cpu_usage gauge\n")
		for _, d := range devices {
			if val, ok := cpuByDevice[d.ID]; ok {
				sb.WriteString(fmt.Sprintf("cpu_usage{device_ip=%q, device_name=%q, vendor=%q} %.2f\n", d.IP, d.Name, d.Vendor, val))
			}
		}

		sb.WriteString("\n# HELP memory_usage 设备内存整体使用率 (%)\n")
		sb.WriteString("# TYPE memory_usage gauge\n")
		for _, d := range devices {
			if val, ok := memByDevice[d.ID]; ok {
				sb.WriteString(fmt.Sprintf("memory_usage{device_ip=%q, device_name=%q, vendor=%q} %.2f\n", d.IP, d.Name, d.Vendor, val))
			}
		}

		// 4. 用户自定义指标（仅导出真实数值）
		for _, d := range devices {
			for _, m := range customByDevice[d.ID] {
				if !m.HasValue {
					continue
				}
				extraTags := ""
				if m.TagsJSON != "" {
					var tagMap map[string]string
					if json.Unmarshal([]byte(m.TagsJSON), &tagMap) == nil {
						for k, v := range tagMap {
							if k != "device_ip" && k != "device_name" && k != "vendor" && k != "metric_label" {
								extraTags += fmt.Sprintf(", %s=%q", k, v)
							}
						}
					}
				}
				sb.WriteString(fmt.Sprintf("\n# HELP %s [用户自定义标签] %s (OID: %s)\n", m.MetricKey, m.CustomName, m.OID))
				sb.WriteString(fmt.Sprintf("# TYPE %s gauge\n", m.MetricKey))
				sb.WriteString(fmt.Sprintf("%s{device_ip=%q, device_name=%q, vendor=%q, metric_label=%q, custom_tag=\"user_defined\"%s} %.2f\n",
					m.MetricKey, d.IP, d.Name, d.Vendor, m.CustomName, extraTags, m.Value))
			}
		}

		// 5. 导出接口采样数据（指标库，设备信息在内存中关联，避免跨库 JOIN）
		sampleRows, sErr := appdb.DBForTable("snmp_metric_samples").Query(`
			SELECT s.device_id, s.metric_name, s.value, s.unit, s.tag_name
			FROM snmp_metric_samples s
			ORDER BY s.metric_name ASC, s.created_at DESC LIMIT 500`)
		if sErr == nil {
			currentMetric := ""
			for sampleRows.Next() {
				var devID string
				var mName, unit, tagName sql.NullString
				var val float64
				if err := sampleRows.Scan(&devID, &mName, &val, &unit, &tagName); err != nil {
					continue
				}
				d := devByID[devID]
				if mName.String != currentMetric {
					currentMetric = mName.String
					sb.WriteString(fmt.Sprintf("\n# HELP %s SNMP 指标采样点数据\n", currentMetric))
					sb.WriteString(fmt.Sprintf("# TYPE %s gauge\n", currentMetric))
				}
				sb.WriteString(fmt.Sprintf("%s{device_ip=%q, device_name=%q, vendor=%q, ifName=%q} %.2f\n",
					currentMetric, d.IP, d.Name, d.Vendor, tagName.String, val))
			}
			sampleRows.Close()
		}

		w.Write([]byte(sb.String()))
	})

	return r
}

// timeRangeToHours 将大屏时间范围转为小时数
func timeRangeToHours(tr string) int {
	switch strings.ToLower(tr) {
	case "1h":
		return 1
	case "6h":
		return 6
	case "7d":
		return 24 * 7
	case "24h", "":
		return 24
	default:
		// 支持 "48h" 这类写法
		if h, err := strconv.Atoi(strings.TrimSuffix(tr, "h")); err == nil {
			return h
		}
		return 24
	}
}

// 确保 encoding/json 被使用（部分端点未来扩展）
var _ = json.Marshal
