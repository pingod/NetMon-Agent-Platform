// 网络设备路由 — 18 个端点
// 等价于 TS routes/networkDeviceRoutes.ts
package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/crypto"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

// NetworkDeviceRoutes 网络设备路由
func NetworkDeviceRoutes() chi.Router {
	r := chi.NewRouter()

	// GET / — 获取所有网络设备
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		devices, err := services.NetworkDeviceServiceInstance.GetAllDevices()
		if err != nil {
			httpx.RespondError(w, "获取设备列表失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, devices)
	})

	// GET /stats — 设备统计数据
	r.Get("/stats", func(w http.ResponseWriter, r *http.Request) {
		var total, online, offline int
		appdb.DB().QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status='online' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN status='offline' THEN 1 ELSE 0 END),0) FROM network_devices`).
			Scan(&total, &online, &offline)
		httpx.Respond(w, map[string]interface{}{
			"total":   total,
			"online":  online,
			"offline": offline,
		})
	})

	// GET /categories — 设备类别/角色列表
	r.Get("/categories", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT DISTINCT role FROM network_devices WHERE role IS NOT NULL AND role != ''`)
		if err != nil {
			httpx.Respond(w, []string{})
			return
		}
		defer rows.Close()
		var categories []string
		for rows.Next() {
			var cat string
			if rows.Scan(&cat) == nil {
				categories = append(categories, cat)
			}
		}
		if categories == nil {
			categories = []string{"switch", "router", "firewall"}
		}
		httpx.Respond(w, categories)
	})

	// GET /vendors — 设备厂商列表
	r.Get("/vendors", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT DISTINCT vendor FROM network_devices WHERE vendor IS NOT NULL AND vendor != ''`)
		if err != nil {
			httpx.Respond(w, []string{})
			return
		}
		defer rows.Close()
		var vendors []string
		for rows.Next() {
			var v string
			if rows.Scan(&v) == nil {
				vendors = append(vendors, v)
			}
		}
		if vendors == nil {
			vendors = []string{"Huawei", "Cisco", "H3C", "Ruijie"}
		}
		httpx.Respond(w, vendors)
	})

	// GET /import/template — 下载批量导入模板
	r.Get("/import/template", handleDownloadTemplate)

	// POST /import — 批量导入网络设备
	r.With(middleware.RequireRole("admin")).Post("/import", handleBatchImport)

	// PUT /{id}/poll-interval — 单设备采集间隔更新
	r.With(middleware.RequireRole("admin")).Put("/{id}/poll-interval", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			SystemPollInterval    int `json:"system_poll_interval"`
			InterfacePollInterval int `json:"interface_poll_interval"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if err := repos.NetworkDevices.UpdatePollInterval(id, req.SystemPollInterval, req.InterfacePollInterval); err != nil {
			httpx.RespondError(w, "更新设备采集间隔失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{
			"success": true,
			"message": "采集间隔设置已保存",
		})
	})

	// POST /batch-poll-interval — 按品牌/设备列表批量更新采集间隔
	r.With(middleware.RequireRole("admin")).Post("/batch-poll-interval", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Vendor                string   `json:"vendor"`
			DeviceIDs             []string `json:"device_ids"`
			SystemPollInterval    int      `json:"system_poll_interval"`
			InterfacePollInterval int      `json:"interface_poll_interval"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		affected, err := repos.NetworkDevices.BatchUpdatePollIntervals(req.Vendor, req.DeviceIDs, req.SystemPollInterval, req.InterfacePollInterval)
		if err != nil {
			httpx.RespondError(w, "批量更新采集间隔失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{
			"success": true,
			"count":   affected,
			"message": fmt.Sprintf("已成功为 %d 台设备应用新的采集间隔", affected),
		})
	})

	// GET /inspections — 获取所有巡检记录
	r.Get("/inspections", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 20
		}
		inspType := r.URL.Query().Get("type")
		status := r.URL.Query().Get("status")

		inspections, err := services.NetworkDeviceServiceInstance.GetAllInspections(limit, inspType, status)
		if err != nil {
			httpx.RespondError(w, "获取巡检记录失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, inspections)
	})

	// POST /test-connection — 临时连接测试
	r.With(middleware.RequireRole("admin")).Post("/test-connection", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IPAddress    string `json:"ip_address"`
			SSHPort      int    `json:"ssh_port"`
			Username     string `json:"username"`
			Password     string `json:"password"`
			CredentialID string `json:"credential_id"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.IPAddress == "" {
			httpx.RespondError(w, "ip_address is required", http.StatusBadRequest)
			return
		}

		testUsername := req.Username
		testPassword := req.Password
		testPrivateKey := ""

		// 如果提供了 credential_id，从 ssh_keys 表获取用户名和密码/密钥
		if req.CredentialID != "" {
			cred, err := repos.SshKeys.GetByID(req.CredentialID)
			if err != nil || cred == nil {
				httpx.RespondError(w, "credential not found", http.StatusBadRequest)
				return
			}
			if cred.Username != nil && *cred.Username != "" {
				testUsername = *cred.Username
			}
			if cred.Password != nil && *cred.Password != "" {
				decrypted, err := crypto.Decrypt(*cred.Password)
				if err == nil {
					testPassword = decrypted
				}
			}
			if cred.PrivateKey != nil && *cred.PrivateKey != "" {
				decrypted, err := crypto.Decrypt(*cred.PrivateKey)
				if err == nil {
					testPrivateKey = decrypted
				}
			}
		}

		// 密码认证：需要用户名和密码；密钥认证：需要用户名和密钥
		if testUsername == "" || (testPassword == "" && testPrivateKey == "") {
			httpx.RespondError(w, "username and password/key are required (可提供 credential_id 自动获取)", http.StatusBadRequest)
			return
		}

		result := services.NetworkDeviceServiceInstance.TestTemporaryConnectionWithKey(req.IPAddress, req.SSHPort, testUsername, testPassword, testPrivateKey)
		httpx.Respond(w, result)
	})

	// POST /batch-inspect — 批量巡检
	r.With(middleware.RequireRole("admin")).Post("/batch-inspect", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DeviceIDs        []string `json:"deviceIds"`
			DeviceIDsSnake   []string `json:"device_ids"`
			InspectionType   string   `json:"inspectionType"`
			CustomTypes      []string `json:"customTypes"`
			TemplateID       string   `json:"templateId"`
			SelectedCommands []string `json:"selectedCommands"`
			Concurrency      int      `json:"concurrency"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		targetIDs := req.DeviceIDs
		if len(targetIDs) == 0 {
			targetIDs = req.DeviceIDsSnake
		}
		if len(targetIDs) == 0 {
			httpx.RespondError(w, "deviceIds is required", http.StatusBadRequest)
			return
		}
		if req.InspectionType == "" {
			req.InspectionType = "standard"
		}
		results := services.NetworkInspectionServiceInstance.BatchInspectWithTemplate(targetIDs, req.InspectionType, req.CustomTypes, req.TemplateID, req.SelectedCommands, req.Concurrency)

		// 自动生成批量巡检报告记录入库 reports 表
		reportID := uuid.New().String()
		reportName := fmt.Sprintf("网络设备批量巡检报告 (%d台) - %s", len(targetIDs), time.Now().Format("2006-01-02 15:04"))
		
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# 网络设备批量巡检报告\n\n"))
		sb.WriteString(fmt.Sprintf("- **巡检时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
		sb.WriteString(fmt.Sprintf("- **受检设备总数**: %d 台\n", len(targetIDs)))
		sb.WriteString(fmt.Sprintf("- **巡检成功**: %d 台\n", len(results)))
		sb.WriteString(fmt.Sprintf("- **巡检失败**: %d 台\n\n", len(targetIDs)-len(results)))
		sb.WriteString("## 一、设备巡检清单\n\n")
		sb.WriteString("| 序号 | 设备名称 | IP 地址 | 厂商 | 巡检类型 | 状态 | 耗时 |\n")
		sb.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
		for i, res := range results {
			if res != nil {
				var dName, dIP, dVendor string
				_ = appdb.DB().QueryRow("SELECT name, ip_address, vendor FROM network_devices WHERE id = ?", res.DeviceID).Scan(&dName, &dIP, &dVendor)
				if dName == "" {
					dName = res.DeviceID
				}
				sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s | %dms |\n",
					i+1, dName, dIP, strings.ToUpper(dVendor), res.InspectionType, res.Status, res.DurationMs))
			}
		}

		metaJSON, _ := json.Marshal(map[string]interface{}{
			"target_ids":     targetIDs,
			"target_type":    "network_device",
			"inspectionType": req.InspectionType,
			"total":          len(targetIDs),
			"success_count":  len(results),
			"failed_count":   len(targetIDs) - len(results),
		})

		nowStr := time.Now().Format("2006-01-02 15:04:05")
		_, _ = appdb.DB().Exec(`
			INSERT INTO reports (id, name, type, content, format, metadata, created_at, updated_at)
			VALUES (?, ?, 'inspection', ?, 'markdown', ?, ?, ?)`,
			reportID, reportName, sb.String(), string(metaJSON), nowStr, nowStr)

		httpx.Respond(w, map[string]interface{}{
			"report_id": reportID,
			"total":     len(targetIDs),
			"results":   results,
		})
	})

	// POST /analyze-output — 分析命令输出
	r.Post("/analyze-output", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Vendor  string `json:"vendor"`
			Command string `json:"command"`
			Output  string `json:"output"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Vendor == "" || req.Command == "" || req.Output == "" {
			httpx.RespondError(w, "vendor, command, and output are required", http.StatusBadRequest)
			return
		}
		analysis := services.AnalyzeCommandOutput(req.Vendor, req.Command, req.Output)
		httpx.Respond(w, map[string]string{"analysis": analysis})
	})

	// POST / — 创建设备
	r.With(middleware.RequireRole("admin")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req services.CreateDeviceRequest
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Name == "" || req.IPAddress == "" || req.Vendor == "" {
			httpx.RespondError(w, "Missing required fields: name, ip_address, vendor", http.StatusBadRequest)
			return
		}
		device, err := services.NetworkDeviceServiceInstance.CreateDevice(req)
		if err != nil {
			errMsg := err.Error()
			if contains(errMsg, "UNIQUE constraint") {
				httpx.RespondError(w, "IP address already exists", http.StatusConflict)
				return
			}
			httpx.RespondError(w, errMsg, http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, device)
	})

	// GET /history/:inspectionId — 获取巡检详情
	r.Get("/history/{inspectionId}", func(w http.ResponseWriter, r *http.Request) {
		inspectionID := chi.URLParam(r, "inspectionId")
		inspection, err := services.NetworkDeviceServiceInstance.GetInspectionDetail(inspectionID)
		if err != nil || inspection == nil {
			httpx.RespondError(w, "Inspection not found", http.StatusNotFound)
			return
		}
		httpx.Respond(w, inspection)
	})

	// GET /:id — 获取单个设备
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		device, err := services.NetworkDeviceServiceInstance.GetDeviceByID(id)
		if err != nil || device == nil {
			httpx.RespondError(w, "Device not found", http.StatusNotFound)
			return
		}
		httpx.Respond(w, device)
	})

	// PUT /:id — 更新设备
	r.With(middleware.RequireRole("admin")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req services.UpdateDeviceRequest
		if !parseJSON(w, r, &req) {
			return
		}
		device, err := services.NetworkDeviceServiceInstance.UpdateDevice(id, req)
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if device == nil {
			httpx.RespondError(w, "Device not found", http.StatusNotFound)
			return
		}
		httpx.Respond(w, device)
	})

	// GET /interface-stats-summary — 顶部6卡片网络设备接口吞吐量与流量统计概览
	r.Get("/interface-stats-summary", handleInterfaceStatsSummary)

	// POST /ping-all — 手动立即 Ping 测所有设备并实时更新在线/离线状态
	r.Post("/ping-all", func(w http.ResponseWriter, r *http.Request) {
		total, online, offline := services.DeviceStatusProberInstance.ProbeNow()
		httpx.Respond(w, map[string]interface{}{
			"total":   total,
			"online":  online,
			"offline": offline,
			"message": fmt.Sprintf("已对 %d 台网络设备完成 Ping 测：%d 在线，%d 离线", total, online, offline),
		})
	})

	r.Post("/probe-status", func(w http.ResponseWriter, r *http.Request) {
		total, online, offline := services.DeviceStatusProberInstance.ProbeNow()
		httpx.Respond(w, map[string]interface{}{
			"total":   total,
			"online":  online,
			"offline": offline,
			"message": fmt.Sprintf("已对 %d 台网络设备完成 Ping 测：%d 在线，%d 离线", total, online, offline),
		})
	})

	// DELETE /:id — 删除设备
	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		deleted := services.NetworkDeviceServiceInstance.DeleteDevice(id)
		if !deleted {
			httpx.RespondError(w, "Device not found", http.StatusNotFound)
			return
		}
		httpx.RespondMessage(w, "Device deleted")
	})

	// POST /batch-delete — 批量删除网络设备
	r.With(middleware.RequireRole("admin")).Post("/batch-delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DeviceIDs      []string `json:"device_ids"`
			DeviceIDsCamel []string `json:"deviceIds"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		targetIDs := req.DeviceIDs
		if len(targetIDs) == 0 {
			targetIDs = req.DeviceIDsCamel
		}
		if len(targetIDs) == 0 {
			httpx.RespondError(w, "device_ids is required", http.StatusBadRequest)
			return
		}
		deletedCount := services.NetworkDeviceServiceInstance.BatchDeleteDevices(targetIDs)
		httpx.Respond(w, map[string]interface{}{
			"success":       true,
			"deleted_count": deletedCount,
			"message":       fmt.Sprintf("已成功批量删除 %d 台网络设备", deletedCount),
		})
	})

	// POST /:id/test-connection — 测试设备连接
	r.With(middleware.RequireRole("admin")).Post("/{id}/test-connection", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		result := services.NetworkDeviceServiceInstance.TestConnection(id)
		httpx.Respond(w, result)
	})

	// POST /:id/inspect — 执行巡检
	r.With(middleware.RequireRole("admin")).Post("/{id}/inspect", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			InspectionType   string   `json:"inspectionType"`
			CustomTypes      []string `json:"customTypes"`
			TemplateID       string   `json:"templateId"`
			SelectedCommands []string `json:"selectedCommands"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.InspectionType == "" {
			req.InspectionType = "standard"
		}
		result, err := services.NetworkInspectionServiceInstance.InspectDeviceWithTemplate(id, req.InspectionType, req.CustomTypes, req.TemplateID, req.SelectedCommands)
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// 同步在报告中心生成巡检报告记录
		if result != nil {
			var dName, dIP, dVendor, dModel string
			_ = appdb.DB().QueryRow("SELECT name, ip_address, vendor, model FROM network_devices WHERE id = ?", id).Scan(&dName, &dIP, &dVendor, &dModel)
			if dName == "" {
				dName = id
			}
			repID := "rep-" + uuid.NewString()[:8]
			repTitle := "网络设备巡检报告"
			if result.TemplateName != "" {
				repTitle = fmt.Sprintf("网络设备巡检报告【模版: %s】", result.TemplateName)
			}
			repName := fmt.Sprintf("%s (%s) - %s", repTitle, dName, time.Now().Format("2006-01-02 15:04"))
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("# %s\n\n", repName))
			sb.WriteString(fmt.Sprintf("- **设备名称**: %s\n", dName))
			sb.WriteString(fmt.Sprintf("- **管理 IP**: `%s`\n", dIP))
			sb.WriteString(fmt.Sprintf("- **厂商型号**: %s %s\n", strings.ToUpper(dVendor), dModel))
			if result.TemplateName != "" {
				sb.WriteString(fmt.Sprintf("- **使用模版**: %s (`%s`)\n", result.TemplateName, result.TemplateID))
			}
			sb.WriteString(fmt.Sprintf("- **巡检时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
			sb.WriteString(fmt.Sprintf("- **巡检类型**: %s\n", req.InspectionType))
			sb.WriteString(fmt.Sprintf("- **综合状态**: %s (耗时 %d ms)\n\n", result.Status, result.DurationMs))
			sb.WriteString("## 详细巡检项目执行结果\n\n")
			for _, item := range result.Results {
				sb.WriteString(fmt.Sprintf("### 巡检模块: %s (状态: %s)\n", item.Type, item.Status))
				if item.Details != "" {
					sb.WriteString(fmt.Sprintf("**解析详情**: %s\n\n", item.Details))
				}
				if item.RawOutput != "" {
					sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", item.RawOutput))
				}
			}
			metaJSON, _ := json.Marshal(map[string]interface{}{
				"target_ids":     []string{id},
				"target_type":    "network_device",
				"inspectionType": req.InspectionType,
				"templateId":     req.TemplateID,
				"templateName":   result.TemplateName,
				"total":          1,
				"success_count":  1,
				"failed_count":   0,
			})
			nowStr := time.Now().Format("2006-01-02 15:04:05")
			_, _ = appdb.DB().Exec(`
				INSERT INTO reports (id, name, type, content, format, metadata, created_at, updated_at)
				VALUES (?, ?, 'inspection', ?, 'markdown', ?, ?, ?)`,
				repID, repName, sb.String(), string(metaJSON), nowStr, nowStr)
		}

		httpx.Respond(w, result)
	})

	// POST /:id/inspect-snmp — SNMP 巡检
	r.With(middleware.RequireRole("admin")).Post("/{id}/inspect-snmp", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		result := services.SnmpServiceInstance.InspectDevice(id)
		if result == nil {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "设备未启用 SNMP 或不存在")
			return
		}

		// 同步在报告中心生成 SNMP 巡检报告
		if result != nil {
			var dName, dIP, dVendor, dModel string
			_ = appdb.DB().QueryRow("SELECT name, ip_address, vendor, model FROM network_devices WHERE id = ?", id).Scan(&dName, &dIP, &dVendor, &dModel)
			if dName == "" {
				dName = id
			}
			repID := "rep-snmp-" + uuid.NewString()[:8]
			repName := fmt.Sprintf("网络设备 SNMP 巡检报告 (%s) - %s", dName, time.Now().Format("2006-01-02 15:04"))
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("# %s\n\n", repName))
			sb.WriteString(fmt.Sprintf("- **设备名称**: %s\n", dName))
			sb.WriteString(fmt.Sprintf("- **管理 IP**: `%s`\n", dIP))
			sb.WriteString(fmt.Sprintf("- **系统描述**: %s\n", result.SysDescr))
			sb.WriteString(fmt.Sprintf("- **系统名称**: %s\n", result.SysName))
			sb.WriteString(fmt.Sprintf("- **巡检时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
			sb.WriteString(fmt.Sprintf("- **接口总数**: %d (Up: %d, Down: %d) (耗时 %d ms)\n\n", result.InterfaceCount, result.UpCount, result.DownCount, result.PollDurationMs))
			sb.WriteString("## 接口指标明细\n\n")
			for _, iface := range result.Interfaces {
				sb.WriteString(fmt.Sprintf("- **%s**: 入向利用率 `%.2f%%`, 出向利用率 `%.2f%%`\n", iface.Name, iface.InUtilization, iface.OutUtilization))
			}
			metaJSON, _ := json.Marshal(map[string]interface{}{
				"target_ids":     []string{id},
				"target_type":    "network_device",
				"inspectionType": "snmp",
				"total":          1,
				"success_count":  1,
				"failed_count":   0,
			})
			nowStr := time.Now().Format("2006-01-02 15:04:05")
			_, _ = appdb.DB().Exec(`
				INSERT INTO reports (id, name, type, content, format, metadata, created_at, updated_at)
				VALUES (?, ?, 'inspection', ?, 'markdown', ?, ?, ?)`,
				repID, repName, sb.String(), string(metaJSON), nowStr, nowStr)
		}

		httpx.Respond(w, result)
	})

	// GET /:id/history — 获取设备巡检历史
	r.Get("/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 20
		}
		history, err := services.NetworkDeviceServiceInstance.GetInspectionHistory(id, limit)
		if err != nil {
			httpx.RespondError(w, "Failed to get inspection history", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, history)
	})

	// POST /:id/generate-commands — 生成巡检命令
	r.With(middleware.RequireRole("admin")).Post("/{id}/generate-commands", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			Description string `json:"description"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Description == "" {
			httpx.RespondError(w, "description is required", http.StatusBadRequest)
			return
		}
		device, err := services.NetworkDeviceServiceInstance.GetDeviceByID(id)
		if err != nil || device == nil {
			httpx.RespondError(w, "Device not found", http.StatusNotFound)
			return
		}
		commands := services.GenerateCommands(device.Vendor, req.Description)
		httpx.Respond(w, commands)
	})

	// GET /{id}/interface-summary — 设备接口摘要
	r.Get("/{id}/interface-summary", handleInterfaceSummary)

	return r
}
