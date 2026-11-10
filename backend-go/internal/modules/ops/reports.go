package ops

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
)

// Report 报告实体
type Report struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Content    string  `json:"content"`
	Format     string  `json:"format"`
	TemplateID *string `json:"template_id"`
	TaskID     *string `json:"task_id"`
	Variables  *string `json:"variables"`
	Metadata   *string `json:"metadata"`
	IsPreset   int     `json:"is_preset"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// ReportTemplate 报告模板实体
type ReportTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"` // incident | inspection | change
	Content     string   `json:"content"`
	Variables   []string `json:"variables"`
	IsPreset    bool     `json:"is_preset"`
	CreatedAt   string   `json:"created_at"`
}

// init 预置默认报告模板与初始报告
func initPresetTemplates() {
	db := appdb.DB()
	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM reports WHERE type = 'template'").Scan(&count)
	if count > 0 {
		return
	}

	presetTemplates := []struct {
		ID          string
		Name        string
		Description string
		Type        string
		Content     string
		Variables   []string
	}{
		{
			ID:          "tpl-net-inspection",
			Name:        "网络设备批量巡检报告模板",
			Description: "适用于交换机、路由器、防火墙等网络设备的多设备批量健康巡检报告",
			Type:        "inspection",
			Content:     "# 网络设备批量巡检报告\n\n- 巡检批次: {{batch_name}}\n- 巡检负责人: {{inspector}}\n- 巡检范围: {{scope}}\n\n## 巡检结论\n{{summary}}",
			Variables:   []string{"batch_name", "inspector", "scope", "summary"},
		},
		{
			ID:          "tpl-server-inspection",
			Name:        "服务器集群批量巡检报告模板",
			Description: "适用于 Linux / Windows 服务器主机的系统负载、磁盘存储与合规检查报告",
			Type:        "inspection",
			Content:     "# 服务器集群批量巡检报告\n\n- 巡检集群: {{cluster_name}}\n- 巡检时间: {{inspect_time}}\n\n## 资源健康汇总\n{{resource_summary}}",
			Variables:   []string{"cluster_name", "inspect_time", "resource_summary"},
		},
		{
			ID:          "tpl-infra-inspection",
			Name:        "IT基础设施综合巡检报告模板",
			Description: "覆盖网络设备、服务器、存储与虚拟化的综合基础设施巡检分析",
			Type:        "inspection",
			Content:     "# IT基础设施综合巡检报告\n\n- 数据中心: {{dc_name}}\n- 巡检周期: {{period}}\n\n## 整体健康评分\n{{health_score}}",
			Variables:   []string{"dc_name", "period", "health_score"},
		},
		{
			ID:          "tpl-incident-report",
			Name:        "网络故障应急与根因分析报告",
			Description: "用于重大告警、网络闪断或设备宕机故障的应急复盘与根因分析",
			Type:        "incident",
			Content:     "# 故障复盘与根因分析报告\n\n- 故障名称: {{incident_name}}\n- 影响范围: {{impact}}\n- 恢复时间: {{duration}}\n\n## 根因分析\n{{root_cause}}",
			Variables:   []string{"incident_name", "impact", "duration", "root_cause"},
		},
	}

	for _, tpl := range presetTemplates {
		varsJSON, _ := json.Marshal(tpl.Variables)
		_, _ = db.Exec(`
			INSERT OR IGNORE INTO reports (id, name, type, content, format, variables, is_preset, created_at, updated_at)
			VALUES (?, ?, 'template', ?, 'markdown', ?, 1, datetime('now','localtime'), datetime('now','localtime'))`,
			tpl.ID, tpl.Name, tpl.Content, string(varsJSON))
	}
}

// SyncAllDevicesInspectionReports 同步全网网络设备与服务器生成全量巡检报告入库
func SyncAllDevicesInspectionReports() []string {
	db := appdb.DB()
	var createdReportIDs []string

	// 1. 获取所有网络设备
	rows, err := db.Query(`SELECT id, name, ip_address, vendor, model, role, status FROM network_devices ORDER BY name ASC`)
	var netDeviceIDs []string
	var netDeviceSummary strings.Builder
	var netTotal, netOnline int
	if err == nil {
		netDeviceSummary.WriteString("# 网络设备全网健康巡检报告\n\n")
		netDeviceSummary.WriteString(fmt.Sprintf("- **巡检时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
		netDeviceSummary.WriteString("- **巡检范围**: 全网基础设施交换机、路由器与安全网关\n")
		netDeviceSummary.WriteString("- **报告格式**: ZIP 压缩包（每台设备含独立 Markdown 巡检报告）\n\n")
		netDeviceSummary.WriteString("## 一、网络设备巡检清单\n\n")
		netDeviceSummary.WriteString("| 序号 | 设备名称 | IP 地址 | 厂商 | 型号 | 角色 | 在线状态 |\n")
		netDeviceSummary.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")

		idx := 1
		for rows.Next() {
			var id, name, ip, vendor, model, role, status string
			if rows.Scan(&id, &name, &ip, &vendor, &model, &role, &status) == nil {
				netDeviceIDs = append(netDeviceIDs, id)
				netTotal++
				if status == "online" {
					netOnline++
				}
				vStr := strings.ToUpper(vendor)
				if vStr == "" {
					vStr = "GENERIC"
				}
				netDeviceSummary.WriteString(fmt.Sprintf("| %d | %s | `%s` | %s | %s | %s | %s |\n",
					idx, name, ip, vStr, model, role, status))
				idx++
			}
		}
		rows.Close()
	}

	if len(netDeviceIDs) > 0 {
		reportID := "rep-net-" + uuid.NewString()[:8]
		reportName := fmt.Sprintf("网络设备全网巡检报告 (%d台) - %s", len(netDeviceIDs), time.Now().Format("2006-01-02 15:04"))
		metaJSON, _ := json.Marshal(map[string]interface{}{
			"target_ids":     netDeviceIDs,
			"target_type":    "network_device",
			"inspectionType": "comprehensive",
			"total":          len(netDeviceIDs),
			"online_count":   netOnline,
			"offline_count":  netTotal - netOnline,
			"success_count":  netOnline,
			"failed_count":   netTotal - netOnline,
		})
		_, _ = db.Exec(`
			INSERT OR REPLACE INTO reports (id, name, type, content, format, metadata, is_preset, created_at, updated_at)
			VALUES (?, ?, 'inspection', ?, 'markdown', ?, 1, datetime('now','localtime'), datetime('now','localtime'))`,
			reportID, reportName, netDeviceSummary.String(), string(metaJSON))
		createdReportIDs = append(createdReportIDs, reportID)
	}

	// 2. 获取所有服务器
	sRows, sErr := db.Query(`SELECT id, name, hostname, port, os, status FROM servers ORDER BY name ASC`)
	var serverIDs []string
	var serverSummary strings.Builder
	var serverTotal, serverOnline int
	if sErr == nil {
		serverSummary.WriteString("# 服务器集群合规与系统健康巡检报告\n\n")
		serverSummary.WriteString(fmt.Sprintf("- **巡检时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
		serverSummary.WriteString("- **巡检范围**: 生产服务器主机负载、磁盘使用与合规基线\n")
		serverSummary.WriteString("- **报告格式**: ZIP 压缩包（每台服务器含独立 Markdown 巡检报告）\n\n")
		serverSummary.WriteString("## 一、服务器主机巡检清单\n\n")
		serverSummary.WriteString("| 序号 | 主机名称 | 主机地址 | 端口 | 操作系统 | 运行状态 |\n")
		serverSummary.WriteString("| --- | --- | --- | --- | --- | --- |\n")

		sIdx := 1
		for sRows.Next() {
			var id, name, host, os, status string
			var port int
			if sRows.Scan(&id, &name, &host, &port, &os, &status) == nil {
				serverIDs = append(serverIDs, id)
				serverTotal++
				if status == "online" || status == "active" {
					serverOnline++
				}
				serverSummary.WriteString(fmt.Sprintf("| %d | %s | `%s` | %d | %s | %s |\n",
					sIdx, name, host, port, os, status))
				sIdx++
			}
		}
		sRows.Close()
	}

	if len(serverIDs) > 0 {
		reportID := "rep-srv-" + uuid.NewString()[:8]
		reportName := fmt.Sprintf("服务器集群合规巡检报告 (%d台) - %s", len(serverIDs), time.Now().Format("2006-01-02 15:04"))
		metaJSON, _ := json.Marshal(map[string]interface{}{
			"target_ids":     serverIDs,
			"target_type":    "server",
			"inspectionType": "compliance",
			"total":          len(serverIDs),
			"online_count":   serverOnline,
			"offline_count":  serverTotal - serverOnline,
			"success_count":  serverOnline,
			"failed_count":   serverTotal - serverOnline,
		})
		_, _ = db.Exec(`
			INSERT OR REPLACE INTO reports (id, name, type, content, format, metadata, is_preset, created_at, updated_at)
			VALUES (?, ?, 'inspection', ?, 'markdown', ?, 1, datetime('now','localtime'), datetime('now','localtime'))`,
			reportID, reportName, serverSummary.String(), string(metaJSON))
		createdReportIDs = append(createdReportIDs, reportID)
	}

	return createdReportIDs
}

// initPresetInspectionReports 检查并自动预置最新设备巡检报告
func initPresetInspectionReports() {
	db := appdb.DB()
	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM reports WHERE type != 'template'").Scan(&count)
	if count == 0 {
		SyncAllDevicesInspectionReports()
	}
}

// ReportRoutes 报告中心完整路由 (/api/v1/reports)
func ReportRoutes() chi.Router {
	r := chi.NewRouter()

	// 1. GET / — 获取已生成的报告列表
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		initPresetTemplates()
		initPresetInspectionReports()
		rows, err := appdb.DB().Query(`
			SELECT id, name, type, content, format, template_id, task_id, variables, metadata, is_preset, created_at, updated_at
			FROM reports
			WHERE type != 'template'
			ORDER BY created_at DESC LIMIT 100`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()

		var list []Report
		for rows.Next() {
			var rep Report
			var tplID, taskID, vars, meta sql.NullString
			if err := rows.Scan(&rep.ID, &rep.Name, &rep.Type, &rep.Content, &rep.Format,
				&tplID, &taskID, &vars, &meta, &rep.IsPreset, &rep.CreatedAt, &rep.UpdatedAt); err == nil {
				if tplID.Valid {
					rep.TemplateID = &tplID.String
				}
				if taskID.Valid {
					rep.TaskID = &taskID.String
				}
				if vars.Valid {
					rep.Variables = &vars.String
				}
				if meta.Valid {
					rep.Metadata = &meta.String
				}
				list = append(list, rep)
			}
		}
		if list == nil {
			list = []Report{}
		}
		httpx.Respond(w, list)
	})

	// POST /sync-inspection-reports — 同步生成全网设备巡检报告
	r.Post("/sync-inspection-reports", func(w http.ResponseWriter, r *http.Request) {
		ids := SyncAllDevicesInspectionReports()
		httpx.Respond(w, map[string]interface{}{
			"success":    true,
			"report_ids": ids,
			"message":    fmt.Sprintf("已成功同步生成 %d 份全网设备巡检报告", len(ids)),
		})
	})

	// DELETE /{id} — 删除报告
	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, err := appdb.DB().Exec(`DELETE FROM reports WHERE id = ?`, id)
		if err != nil {
			httpx.RespondError(w, "删除报告失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "报告已删除")
	})

	// 2. GET /templates — 获取报告模板列表
	r.Get("/templates", func(w http.ResponseWriter, r *http.Request) {
		initPresetTemplates()
		rows, err := appdb.DB().Query(`
			SELECT id, name, content, variables, is_preset, created_at
			FROM reports
			WHERE type = 'template'
			ORDER BY is_preset DESC, created_at DESC`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()

		var list []ReportTemplate
		for rows.Next() {
			var id, name, content, createdAt string
			var varsStr sql.NullString
			var isPreset int
			if err := rows.Scan(&id, &name, &content, &varsStr, &isPreset, &createdAt); err == nil {
				var vars []string
				if varsStr.Valid && varsStr.String != "" {
					_ = json.Unmarshal([]byte(varsStr.String), &vars)
				}
				desc := "自定义报告模板"
				tType := "inspection"
				if strings.Contains(name, "故障") {
					tType = "incident"
					desc = "故障应急与根因分析模板"
				} else if strings.Contains(name, "变更") {
					tType = "change"
					desc = "运维变更记录模板"
				} else if strings.Contains(name, "网络") {
					desc = "网络设备健康巡检模板"
				} else if strings.Contains(name, "服务器") {
					desc = "服务器主机批量巡检模板"
				}
				list = append(list, ReportTemplate{
					ID:          id,
					Name:        name,
					Description: desc,
					Type:        tType,
					Content:     content,
					Variables:   vars,
					IsPreset:    isPreset == 1,
					CreatedAt:   createdAt,
				})
			}
		}
		if list == nil {
			list = []ReportTemplate{}
		}
		httpx.Respond(w, list)
	})

	// 3. POST /templates — 创建自定义报告模板
	r.With(middleware.RequireRole("admin")).Post("/templates", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Type        string   `json:"type"`
			Content     string   `json:"content"`
			Variables   []string `json:"variables"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			httpx.RespondError(w, "模板名称不能为空", http.StatusBadRequest)
			return
		}
		id := uuid.NewString()
		varsJSON, _ := json.Marshal(req.Variables)
		_, err := appdb.DB().Exec(`
			INSERT INTO reports (id, name, type, content, format, variables, is_preset, created_at, updated_at)
			VALUES (?, ?, 'template', ?, 'markdown', ?, 0, datetime('now','localtime'), datetime('now','localtime'))`,
			id, req.Name, req.Content, string(varsJSON))
		if err != nil {
			httpx.RespondError(w, "创建模板失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]string{"id": id, "name": req.Name})
	})

	// 4. DELETE /templates/{id} — 删除报告模板
	r.With(middleware.RequireRole("admin")).Delete("/templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, err := appdb.DB().Exec(`DELETE FROM reports WHERE id = ? AND type = 'template' AND is_preset = 0`, id)
		if err != nil {
			httpx.RespondError(w, "删除失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "模板已删除")
	})

	// 5. POST /generate — 手动生成报告
	r.Post("/generate", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TemplateID string                 `json:"templateId"`
			Variables  map[string]interface{} `json:"variables"`
			Format     string                 `json:"format"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		var tplName, tplContent string
		err := appdb.DB().QueryRow("SELECT name, content FROM reports WHERE id = ? AND type = 'template'", req.TemplateID).Scan(&tplName, &tplContent)
		if err != nil {
			httpx.RespondFailStatus(w, http.StatusNotFound, "报告模板不存在")
			return
		}

		// 替换模板变量
		content := tplContent
		for k, v := range req.Variables {
			valStr := fmt.Sprintf("%v", v)
			content = strings.ReplaceAll(content, "{{"+k+"}}", valStr)
		}

		reportID := uuid.NewString()
		reportName := fmt.Sprintf("%s - %s", tplName, time.Now().Format("2006-01-02 15:04"))
		varsJSON, _ := json.Marshal(req.Variables)

		_, err = appdb.DB().Exec(`
			INSERT INTO reports (id, name, type, content, format, template_id, variables, created_at, updated_at)
			VALUES (?, ?, 'inspection', ?, 'markdown', ?, ?, datetime('now','localtime'), datetime('now','localtime'))`,
			reportID, reportName, content, req.TemplateID, string(varsJSON))
		if err != nil {
			httpx.RespondError(w, "生成报告失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		httpx.Respond(w, map[string]string{
			"id":   reportID,
			"name": reportName,
		})
	})

	// 6. GET /analytics — 报告数据分析
	r.Get("/analytics", func(w http.ResponseWriter, r *http.Request) {
		db := appdb.DB()
		var reportCount, inspectionTotal, inspectionCompleted, inspectionFailed int
		_ = db.QueryRow("SELECT COUNT(*) FROM reports WHERE type != 'template'").Scan(&reportCount)
		_ = db.QueryRow(`SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END),0)
			FROM network_inspection_history`).Scan(&inspectionTotal, &inspectionCompleted, &inspectionFailed)

		httpx.Respond(w, map[string]interface{}{
			"analysisStats": map[string]interface{}{
				"total":     reportCount + inspectionTotal,
				"completed": reportCount + inspectionCompleted,
				"failed":    inspectionFailed,
			},
			"remediationStats": map[string]interface{}{
				"total":         inspectionTotal,
				"success_count": inspectionCompleted,
				"failed_count":  inspectionFailed,
				"rolled_back":   0,
			},
		})
	})

	// 7. GET /scheduled/all — 定时报告（读取真实配置表）
	r.Get("/scheduled/all", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT id, name, template_id, cron_expression, enabled, recipients, format, last_generated
			FROM report_schedules ORDER BY created_at DESC`)
		if err != nil {
			httpx.Respond(w, []map[string]interface{}{})
			return
		}
		defer rows.Close()
		list := []map[string]interface{}{}
		for rows.Next() {
			var id, name, templateID, cron, format string
			var recipients, lastGenerated sql.NullString
			var enabled int
			if rows.Scan(&id, &name, &templateID, &cron, &enabled, &recipients, &format, &lastGenerated) != nil {
				continue
			}
			var recipientList []string
			if recipients.Valid && recipients.String != "" {
				_ = json.Unmarshal([]byte(recipients.String), &recipientList)
			}
			if recipientList == nil {
				recipientList = []string{}
			}
			list = append(list, map[string]interface{}{
				"id":              id,
				"name":            name,
				"template_id":     templateID,
				"cron_expression": cron,
				"enabled":         enabled == 1,
				"recipients":      recipientList,
				"format":          format,
				"last_generated":  lastGenerated.String,
			})
		}
		httpx.Respond(w, list)
	})

	// 8. GET /{id}/export & GET /{id}/download-zip — 导出报告（支持 ZIP 压缩包，每台设备独立文件）
	r.Get("/{id}/export", handleExportReport)
	r.Get("/{id}/download-zip", handleExportReportZip)

	return r
}

// handleExportReportZip 专门导出 ZIP 压缩包端点
func handleExportReportZip(w http.ResponseWriter, r *http.Request) {
	exportReportAsZip(w, r, chi.URLParam(r, "id"))
}

// handleExportReport 处理报告导出（Markdown / ZIP；PDF、Word 尚未实现）
func handleExportReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))

	switch format {
	case "zip":
		exportReportAsZip(w, r, id)
		return
	case "pdf", "word", "docx":
		httpx.RespondFailStatus(w, http.StatusBadRequest, "暂不支持导出为 "+format+" 格式，请使用 markdown 或 zip")
		return
	case "", "md", "markdown":
		// 默认按 Markdown 输出
	default:
		httpx.RespondFailStatus(w, http.StatusBadRequest, "不支持的导出格式: "+format)
		return
	}

	var name, content string
	err := appdb.DB().QueryRow("SELECT name, content FROM reports WHERE id = ?", id).Scan(&name, &content)
	if err != nil {
		httpx.RespondError(w, "报告不存在", http.StatusNotFound)
		return
	}

	filename := fmt.Sprintf("report-%s.md", sanitizeFilename(id))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(content))
}

// exportReportAsZip 将巡检报告打包为 ZIP 压缩包：每台设备独立一个巡检报告文件
func exportReportAsZip(w http.ResponseWriter, r *http.Request, reportID string) {
	db := appdb.DB()
	var reportName, reportContent string
	var metaStr sql.NullString
	err := db.QueryRow("SELECT name, content, metadata FROM reports WHERE id = ?", reportID).Scan(&reportName, &reportContent, &metaStr)
	if err != nil {
		httpx.RespondError(w, "报告不存在", http.StatusNotFound)
		return
	}

	// 准备 ZIP 缓冲区
	zipBuf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zipBuf)

	// 解析元数据中的目标设备/服务器列表
	var meta map[string]interface{}
	if metaStr.Valid && metaStr.String != "" {
		_ = json.Unmarshal([]byte(metaStr.String), &meta)
	}

	var targetIDs []string
	targetType := "network_device"

	if meta != nil {
		if tt, ok := meta["target_type"].(string); ok {
			targetType = tt
		}
		if idsRaw, ok := meta["target_ids"].([]interface{}); ok {
			for _, item := range idsRaw {
				if idStr, ok := item.(string); ok {
					targetIDs = append(targetIDs, idStr)
				}
			}
		}
	}

	// 如果没有在 metadata 中找到具体 ID，则根据最近的巡检设备自动补全
	if len(targetIDs) == 0 {
		if targetType == "server" || strings.Contains(reportName, "服务器") {
			rows, _ := db.Query("SELECT id FROM servers ORDER BY updated_at DESC LIMIT 10")
			if rows != nil {
				for rows.Next() {
					var sid string
					if rows.Scan(&sid) == nil {
						targetIDs = append(targetIDs, sid)
					}
				}
				rows.Close()
			}
		} else {
			rows, _ := db.Query("SELECT id FROM network_devices ORDER BY updated_at DESC LIMIT 10")
			if rows != nil {
				for rows.Next() {
					var did string
					if rows.Scan(&did) == nil {
						targetIDs = append(targetIDs, did)
					}
				}
				rows.Close()
			}
		}
	}

	// 1. 写入总览汇总文件：00_巡检总览及汇总报告.md
	overviewHeader := "# " + reportName + "\n\n"
	overviewHeader += "- **导出时间**: " + time.Now().Format("2006-01-02 15:04:05") + "\n"
	overviewHeader += fmt.Sprintf("- **巡检设备总数**: %d 台\n", len(targetIDs))
	overviewHeader += fmt.Sprintf("- **报告说明**: 本压缩包内包含每台受检设备的独立巡检报告文件，共计 %d 份。\n\n", len(targetIDs))

	overviewFile, _ := zipWriter.Create("00_巡检总览及汇总报告.md")
	_, _ = overviewFile.Write([]byte(overviewHeader + reportContent))

	// 2. 为每台设备/服务器生成独立的巡检报告文件
	deviceIndex := 1
	for _, devID := range targetIDs {
		if targetType == "server" || strings.Contains(reportName, "服务器") {
			// 查询服务器详情与最新合规检查记录
			var sName, sHost string
			var sPort int
			var sOs, sCpu, sMem, sDisk sql.NullString
			_ = db.QueryRow(`
				SELECT name, hostname, port, os, cpu_cores, memory_gb, disk_gb
				FROM servers WHERE id = ?`, devID).Scan(&sName, &sHost, &sPort, &sOs, &sCpu, &sMem, &sDisk)
			if sName == "" {
				// 记录已不存在：仅保留 ID，不编造主机信息
				sName = devID
			}

			// 查询该服务器最新合规检查结果
			var checkSummary sql.NullString
			_ = db.QueryRow(`SELECT check_results FROM compliance_checks WHERE server_id = ? ORDER BY created_at DESC LIMIT 1`, devID).Scan(&checkSummary)

			devReportMd := generateSingleServerReportMarkdown(deviceIndex, sName, sHost, sPort, sOs.String, sCpu.String, sMem.String, sDisk.String, checkSummary.String)
			cleanName := sanitizeFilename(sName)
			cleanHost := sanitizeFilename(sHost)
			filename := fmt.Sprintf("%02d_服务器_%s_%s_巡检报告.md", deviceIndex, cleanName, cleanHost)

			f, err := zipWriter.Create(filename)
			if err == nil {
				_, _ = f.Write([]byte(devReportMd))
			}
		} else {
			// 查询网络设备详情与最新巡检历史
			var dName, dIP, dVendor, dModel, dType string
			_ = db.QueryRow(`
				SELECT name, ip_address, vendor, model, device_type
				FROM network_devices WHERE id = ?`, devID).Scan(&dName, &dIP, &dVendor, &dModel, &dType)
			if dName == "" {
				// 记录已不存在：仅保留 ID，不编造 IP / 厂商
				dName = devID
			}

			// 查询最新巡检明细
			var inspSummary, resultsJSON sql.NullString
			var durMs sql.NullInt64
			_ = db.QueryRow(`
				SELECT summary, results, duration_ms
				FROM network_inspection_history WHERE device_id = ? ORDER BY created_at DESC LIMIT 1`, devID).Scan(&inspSummary, &resultsJSON, &durMs)

			devReportMd := generateSingleNetworkDeviceReportMarkdown(deviceIndex, dName, dIP, dVendor, dModel, dType, durMs.Int64, inspSummary.String, resultsJSON.String)
			cleanName := sanitizeFilename(dName)
			cleanIP := sanitizeFilename(dIP)
			filename := fmt.Sprintf("%02d_网络设备_%s_%s_巡检报告.md", deviceIndex, cleanName, cleanIP)

			f, err := zipWriter.Create(filename)
			if err == nil {
				_, _ = f.Write([]byte(devReportMd))
			}
		}
		deviceIndex++
	}

	_ = zipWriter.Close()

	// 发送 ZIP 下载响应
	zipFilename := fmt.Sprintf("inspection_reports_%s.zip", time.Now().Format("20060102_150405"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", zipFilename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", zipBuf.Len()))
	_, _ = w.Write(zipBuf.Bytes())
}

// generateSingleNetworkDeviceReportMarkdown 生成单台网络设备的独立巡检报告
func generateSingleNetworkDeviceReportMarkdown(index int, name, ip, vendor, model, devType string, durationMs int64, summary, resultsJSON string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 网络设备单机巡检报告\n\n"))
	sb.WriteString(fmt.Sprintf("## 一、设备档案基本信息\n\n"))
	sb.WriteString(fmt.Sprintf("- **设备序号**: #%d\n", index))
	sb.WriteString(fmt.Sprintf("- **设备名称**: %s\n", name))
	sb.WriteString(fmt.Sprintf("- **管理 IP**: `%s`\n", ip))
	vStr := strings.ToUpper(vendor)
	if vStr == "" {
		vStr = "GENERIC"
	}
	sb.WriteString(fmt.Sprintf("- **设备厂商**: %s\n", vStr))
	if model != "" {
		sb.WriteString(fmt.Sprintf("- **设备型号**: %s\n", model))
	}
	if devType != "" {
		sb.WriteString(fmt.Sprintf("- **设备类型**: %s\n", devType))
	}
	sb.WriteString(fmt.Sprintf("- **巡检时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))

	// 解析巡检项数据
	type ParsedItem struct {
		Type      string `json:"type"`
		Success   bool   `json:"success"`
		Status    string `json:"status"` // normal | warning | critical | error
		Details   string `json:"details"`
		RawOutput string `json:"rawOutput"`
	}

	var parsedItems []ParsedItem
	if resultsJSON != "" {
		_ = json.Unmarshal([]byte(resultsJSON), &parsedItems)
	}

	// 统计异常与警告数量（仅基于真实巡检项，不臆测）
	criticalCount := 0
	warningCount := 0
	normalCount := 0

	for _, item := range parsedItems {
		switch item.Status {
		case "critical", "error":
			criticalCount++
		case "warning":
			warningCount++
		default:
			normalCount++
		}
	}

	// 确定整体健康状态；无数据时不做健康结论
	if len(parsedItems) == 0 {
		sb.WriteString("- **健康状态**: 未知（无巡检项数据）\n\n")
	} else if criticalCount > 0 {
		sb.WriteString(fmt.Sprintf("- **健康状态**: ❌ 异常 (存在 %d 项严重问题，%d 项警告)\n\n", criticalCount, warningCount))
	} else if warningCount > 0 {
		sb.WriteString(fmt.Sprintf("- **健康状态**: ⚠️ 警告 (存在 %d 项警告关注项)\n\n", warningCount))
	} else {
		sb.WriteString(fmt.Sprintf("- **健康状态**: ✅ 正常 (共 %d 项巡检项通过)\n\n", normalCount))
	}

	// 连通性表格：仅使用采集到的真实耗时
	sb.WriteString("## 二、连通性与服务可用性探测\n\n")
	if durationMs > 0 {
		sb.WriteString("| 探测协议 | 端口 | 连通状态 | 延迟 (ms) | 诊断结论 |\n")
		sb.WriteString("| --- | --- | --- | --- | --- |\n")
		sb.WriteString(fmt.Sprintf("| 巡检会话 | - | ✅ 已完成 | %d ms | 采集命令执行完成 |\n\n", durationMs))
	} else {
		sb.WriteString("无连通性探测数据。\n\n")
	}

	// 巡检项表格
	sb.WriteString("## 三、关键巡检项执行明细\n\n")
	if len(parsedItems) == 0 {
		sb.WriteString("无巡检项明细数据，请先对该设备执行巡检任务。\n\n")
	}
	sb.WriteString("| 检查项目 | 执行命令 | 巡检结果与分析 | 状态 |\n")
	sb.WriteString("| --- | --- | --- | --- |\n")

	type CheckItemDef struct {
		Name    string
		Command string
		Result  string
		Status  string
	}

	var tableItems []CheckItemDef

	if len(parsedItems) > 0 {
		for _, pi := range parsedItems {
			name := pi.Type
			cmd := "display " + pi.Type
			switch pi.Type {
			case "interface":
				name = "接口运行状态 (Up/Down)"
				cmd = "display interface brief / show ip int brief"
			case "cpu":
				name = "CPU 与系统负载"
				cmd = "display cpu-usage / show processes cpu"
			case "memory":
				name = "内存空间利用率"
				cmd = "display memory / show memory statistics"
			case "environment":
				name = "电源与风扇环境"
				cmd = "display environment / show environment"
			case "routing":
				name = "路由表与拓扑收敛"
				cmd = "display ip routing-table / show ip route"
			case "acl":
				name = "安全策略与访问控制"
				cmd = "display acl all / show access-lists"
			case "stp":
				name = "生成树拓扑状态 (STP)"
				cmd = "display stp brief"
			case "lldp":
				name = "邻居发现状态 (LLDP)"
				cmd = "display lldp neighbor brief"
			case "config":
				name = "设备运行配置状态"
				cmd = "display current-configuration"
			}

			statusStr := "✅ 正常"
			resText := pi.Details
			if resText == "" {
				resText = pi.RawOutput
			}
			if resText == "" {
				resText = "检查完成"
			}

			switch pi.Status {
			case "critical", "error":
				statusStr = "❌ 异常"
				if !strings.HasPrefix(resText, "❌") {
					resText = "❌ " + resText
				}
			case "warning":
				statusStr = "⚠️ 警告"
				if !strings.HasPrefix(resText, "⚠️") {
					resText = "⚠️ " + resText
				}
			default:
				if !strings.HasPrefix(resText, "✅") {
					resText = "✅ " + resText
				}
			}

			tableItems = append(tableItems, CheckItemDef{
				Name:    name,
				Command: cmd,
				Result:  resText,
				Status:  statusStr,
			})
		}
	}

	// 无可解析巡检项时不生成任何推测性明细

	for _, item := range tableItems {
		cleanRes := strings.ReplaceAll(item.Result, "|", "\\|")
		cleanRes = strings.ReplaceAll(cleanRes, "\n", " ")
		sb.WriteString(fmt.Sprintf("| %s | `%s` | %s | %s |\n", item.Name, item.Command, cleanRes, item.Status))
	}
	sb.WriteString("\n")

	// 四、巡检诊断总结
	sb.WriteString("## 四、巡检诊断总结\n\n")
	if summary != "" {
		sb.WriteString(summary + "\n\n")
	} else if len(parsedItems) == 0 {
		sb.WriteString("本次未获得巡检结果数据，无法给出健康结论。\n\n")
	} else if criticalCount > 0 || warningCount > 0 {
		sb.WriteString(fmt.Sprintf("发现 %d 个严重问题，%d 个警告，需要立即处理\n\n", criticalCount, warningCount))
	} else {
		sb.WriteString(fmt.Sprintf("共 %d 项巡检项全部正常。\n\n", normalCount))
	}

	// 五、异常处理清单（仅在存在真实异常时输出）
	if criticalCount > 0 || warningCount > 0 {
		sb.WriteString("## 五、异常问题清单\n\n")
		for _, item := range parsedItems {
			if item.Status == "critical" || item.Status == "error" || item.Status == "warning" {
				detail := item.Details
				if detail == "" {
					detail = item.RawOutput
				}
				sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", item.Status, item.Type, strings.ReplaceAll(detail, "\n", " ")))
			}
		}
		sb.WriteString("\n## 六、运维保障建议\n\n")
	} else {
		sb.WriteString("## 五、运维保障建议\n\n")
	}
	sb.WriteString("1. **配置备份**: 请确认该设备已加入自动定时配置备份任务计划。\n")
	sb.WriteString("2. **链路监控**: 持续监控核心聚合链路流量峰值，预防带宽瓶颈。\n")
	sb.WriteString("3. **日志审计**: 定期审计 Syslog 远端日志中心安全事件。\n")

	return sb.String()
}

// generateSingleServerReportMarkdown 生成单台服务器的独立巡检报告
func generateSingleServerReportMarkdown(index int, name, host string, port int, os, cpu, mem, disk, checkResultsJSON string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 服务器单机合规巡检报告\n\n"))
	sb.WriteString(fmt.Sprintf("## 一、服务器基础配置档案\n\n"))
	sb.WriteString(fmt.Sprintf("- **主机序号**: #%d\n", index))
	sb.WriteString(fmt.Sprintf("- **服务器名称**: %s\n", name))
	sb.WriteString(fmt.Sprintf("- **主机名/IP**: `%s`\n", host))
	sb.WriteString(fmt.Sprintf("- **SSH 端口**: %d\n", port))
	if os != "" {
		sb.WriteString(fmt.Sprintf("- **操作系统**: %s\n", os))
	}
	if cpu != "" {
		sb.WriteString(fmt.Sprintf("- **CPU 配置**: %s 核\n", cpu))
	}
	if mem != "" {
		sb.WriteString(fmt.Sprintf("- **内存配置**: %s GB\n", mem))
	}
	if disk != "" {
		sb.WriteString(fmt.Sprintf("- **磁盘容量**: %s GB\n", disk))
	}
	sb.WriteString(fmt.Sprintf("- **巡检时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))

	// 解析合规巡检结果
	var results map[string]struct {
		Success    bool   `json:"success"`
		Stdout     string `json:"stdout"`
		Command    string `json:"command"`
		AIAnalysis string `json:"aiAnalysis"`
	}
	if checkResultsJSON != "" {
		_ = json.Unmarshal([]byte(checkResultsJSON), &results)
	}

	abnormalCount := 0
	for _, res := range results {
		if !res.Success {
			abnormalCount++
		}
	}

	if len(results) == 0 {
		sb.WriteString("- **巡检结论**: 未知（未获取到合规检查数据）\n\n")
	} else if abnormalCount > 0 {
		sb.WriteString(fmt.Sprintf("- **巡检结论**: ❌ 存在 %d 项合规检查异常，需立即整改\n\n", abnormalCount))
	} else {
		sb.WriteString("- **巡检结论**: ✅ 合规检查通过 (各项基线指标正常)\n\n")
	}

	sb.WriteString("## 二、合规与性能巡检项目明细\n\n")
	sb.WriteString("| 检查项目 | 执行命令 | 检查结果与分析 | 结论 |\n")
	sb.WriteString("| --- | --- | --- | --- |\n")

	if len(results) > 0 {
		for checkName, res := range results {
			statusIcon := "✅ 通过"
			if !res.Success {
				statusIcon = "❌ 异常"
			}
			analysis := res.AIAnalysis
			if analysis == "" {
				if res.Success {
					analysis = "检查项符合基线要求"
				} else {
					analysis = "检查项未达标，需整改"
				}
			}
			cleanCmd := strings.ReplaceAll(res.Command, "|", "\\|")
			cleanStdout := strings.ReplaceAll(res.Stdout, "\n", " ")
			if runes := []rune(cleanStdout); len(runes) > 40 {
				cleanStdout = string(runes[:40]) + "..."
			}
			cleanStdout = strings.ReplaceAll(cleanStdout, "|", "\\|")
			sb.WriteString(fmt.Sprintf("| %s | `%s` | %s (%s) | %s |\n", checkName, cleanCmd, analysis, cleanStdout, statusIcon))
		}
	} else {
		sb.WriteString("无合规检查结果数据，请先对该服务器执行巡检任务。\n")
	}

	sb.WriteString("\n## 三、综合安全与优化建议\n\n")
	sb.WriteString("1. **补丁更新**: 建议按季度计划更新系统内核与基础软件安全补丁。\n")
	sb.WriteString("2. **存储预警**: 定期清理日志目录 `/var/log` 及临时文件。\n")
	sb.WriteString("3. **权限基线**: 确保禁用密码暴力破解与限制 root 账户直接公网登录。\n")

	return sb.String()
}

// sanitizeFilename 清理文件名非法字符
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", " ", "_")
	return replacer.Replace(name)
}

// parseJSON 通用请求体解析
func parseJSON(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		httpx.RespondError(w, "无效的 JSON 请求体: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}
