// 服务器命令路由 — 对应 TS serverCommandRoutes.ts
// 挂载路径：/api/v1/server-commands
// 提供：连接测试、命令执行、合规检查
package routes

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/inspection"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

// ServerCommandRoutes 服务器命令路由
func ServerCommandRoutes() chi.Router {
	r := chi.NewRouter()

	// GET / — 命令执行历史
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT id, server_id, command, status, exit_code, stdout, stderr, executed_at, duration_ms FROM server_command_history ORDER BY executed_at DESC LIMIT 50`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()
		var list []map[string]interface{}
		for rows.Next() {
			var id, srvID, cmd, status, execAt string
			var exitCode, durMs sql.NullInt64
			var stdout, stderr sql.NullString
			if rows.Scan(&id, &srvID, &cmd, &status, &exitCode, &stdout, &stderr, &execAt, &durMs) == nil {
				list = append(list, map[string]interface{}{
					"id":          id,
					"server_id":   srvID,
					"command":     cmd,
					"status":      status,
					"exit_code":   exitCode.Int64,
					"stdout":      stdout.String,
					"stderr":      stderr.String,
					"executed_at": execAt,
					"duration_ms": durMs.Int64,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	// POST /{id}/test — 测试服务器连接
	r.With(middleware.RequireRole("admin")).Post("/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		exists, _ := repos.Servers.Exists(id)
		if !exists {
			httpx.RespondError(w, "服务器不存在", http.StatusNotFound)
			return
		}

		cfg, err := buildServerSSHConfig(id)
		if err != nil {
			httpx.RespondError(w, "获取服务器凭证失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		client, err := services.SSHDial(cfg)
		if err != nil {
			httpx.RespondError(w, "连接失败: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer client.Close()

		// 连接成功，更新 last_connected
		repos.Servers.UpdateLastConnected(id)

		// 尝试执行一条命令验证会话可用
		stdout, _, err := client.ExecCommand("echo ok", 10*time.Second)
		if err != nil {
			httpx.Respond(w, map[string]interface{}{
				"message": "SSH 连接成功（命令执行受限）",
			})
			return
		}
		_ = stdout
		httpx.Respond(w, map[string]string{
			"message": "SSH 连接成功",
		})
	})

	// POST /{id}/exec — 执行命令
	r.With(middleware.RequireRole("admin")).Post("/{id}/exec", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		exists, _ := repos.Servers.Exists(id)
		if !exists {
			httpx.RespondError(w, "服务器不存在", http.StatusNotFound)
			return
		}

		var req struct {
			Command string `json:"command"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Command == "" {
			httpx.RespondError(w, "命令不能为空", http.StatusBadRequest)
			return
		}

		cfg, err := buildServerSSHConfig(id)
		if err != nil {
			httpx.RespondError(w, "获取服务器凭证失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		start := time.Now()
		client, err := services.SSHDial(cfg)
		if err != nil {
			httpx.RespondError(w, "连接失败: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer client.Close()

		stdout, stderr, execErr := client.ExecCommand(req.Command, 60*time.Second)
		duration := time.Since(start).Milliseconds()

		success := execErr == nil
		repos.Servers.UpdateLastConnected(id)

		// 记录命令历史（executed_by 取当前用户）
		executedBy := "admin"
		if u := middleware.GetUser(r); u != nil && u.Username != "" {
			executedBy = u.Username
		}
		_ = repos.Servers.InsertCommandHistory(id, req.Command, stdout, stderr, boolToInt(success), int(duration), executedBy)

		httpx.Respond(w, map[string]interface{}{
			"success":  success,
			"stdout":   stdout,
			"stderr":   stderr,
			"command":  req.Command,
			"duration": duration,
		})
	})

	// POST /{id}/compliance — 合规检查（支持标准巡检与自定义模版巡检）
	r.With(middleware.RequireRole("admin")).Post("/{id}/compliance", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		exists, _ := repos.Servers.Exists(id)
		if !exists {
			httpx.RespondError(w, "服务器不存在", http.StatusNotFound)
			return
		}

		var req struct {
			UseAI            *bool    `json:"useAI"`
			Concurrency      *int     `json:"concurrency"`
			InspectionType   string   `json:"inspectionType"`   // "standard" | "custom"
			TemplateID       string   `json:"templateId"`       // 模版中心模版 ID
			SelectedCommands []string `json:"selectedCommands"` // 指定执行的命令名称列表（若为空则执行全部）
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		useAI := true
		if req.UseAI != nil {
			useAI = *req.UseAI
		}

		cfg, err := buildServerSSHConfig(id)
		if err != nil {
			httpx.RespondError(w, "获取服务器凭证失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// 查询服务器基本信息（名称、主机名、厂商、系统）
		var sName, sHost, sVendor, sOSType string
		_ = appdb.DB().QueryRow("SELECT name, hostname, vendor, os_type FROM servers WHERE id = ?", id).Scan(&sName, &sHost, &sVendor, &sOSType)
		if sName == "" {
			sName = id
		}

		client, err := services.SSHDial(cfg)
		if err != nil {
			httpx.RespondError(w, "连接失败: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer client.Close()

		// 确定合规检查项与模版名称
		var checks map[string]string
		inspectionTitle := "标准安全与健康基线巡检"
		tplName := ""
		tplVendor := ""

		if req.TemplateID != "" {
			tplDetail, err := inspection.GetTemplateDetail(req.TemplateID)
			if err == nil && len(tplDetail.Commands) > 0 {
				checks = tplDetail.CommandMap()
				tplName = tplDetail.Name
				tplVendor = tplDetail.Vendor
				inspectionTitle = fmt.Sprintf("自定义模版巡检: %s", tplName)
				inspection.IncrementTemplateUsage(req.TemplateID)
			}
		}

		// 若未指定 templateId 但选择了自定义模式，则按品牌智能匹配推荐模版
		if len(checks) == 0 && req.InspectionType == "custom" {
			bestTpl, err := inspection.FindBestTemplateForServer(sVendor, sOSType)
			if err == nil && len(bestTpl.Commands) > 0 {
				checks = bestTpl.CommandMap()
				tplName = bestTpl.Name
				tplVendor = bestTpl.Vendor
				inspectionTitle = fmt.Sprintf("品牌专属巡检: %s", tplName)
				inspection.IncrementTemplateUsage(bestTpl.ID)
			}
		}

		// 兜底：使用默认检查项
		if len(checks) == 0 {
			checks = defaultServerChecks()
		}

		// 如果前端传入了指定的 selectedCommands，进行过滤
		if len(req.SelectedCommands) > 0 {
			filtered := make(map[string]string)
			selSet := make(map[string]bool)
			for _, cName := range req.SelectedCommands {
				selSet[cName] = true
			}
			for name, cmd := range checks {
				if selSet[name] {
					filtered[name] = cmd
				}
			}
			if len(filtered) > 0 {
				checks = filtered
			}
		}

		results := map[string]interface{}{}
		for name, cmd := range checks {
			start := time.Now()
			stdout, stderr, execErr := client.ExecCommand(cmd, 30*time.Second)
			duration := time.Since(start).Milliseconds()
			success := execErr == nil
			if !success {
				stdout = fmt.Sprintf("执行失败: %v", execErr)
			}
			results[name] = map[string]interface{}{
				"success":    success,
				"stdout":     stdout,
				"stderr":     stderr,
				"command":    cmd,
				"duration":   duration,
				"aiAnalysis": aiAnalysisForCheck(name, stdout, useAI),
			}
		}

		// 保存合规检查记录
		summary, _ := json.Marshal(results)
		status := "completed"
		_ = repos.Servers.InsertComplianceCheck(id, inspectionTitle, string(summary), status)

		// 同步在报告中心生成巡检报告记录
		repID := "rep-srv-" + uuid.NewString()[:8]
		repName := fmt.Sprintf("服务器巡检合规报告 (%s) - %s", sName, time.Now().Format("2006-01-02 15:04"))
		if tplName != "" {
			repName = fmt.Sprintf("服务器巡检报告 (%s - %s) - %s", sName, tplName, time.Now().Format("2006-01-02 15:04"))
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n", repName))
		sb.WriteString(fmt.Sprintf("- **服务器名称**: %s\n", sName))
		sb.WriteString(fmt.Sprintf("- **主机地址**: `%s`\n", sHost))
		sb.WriteString(fmt.Sprintf("- **巡检时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
		sb.WriteString(fmt.Sprintf("- **巡检类型**: %s\n", inspectionTitle))
		if tplName != "" {
			sb.WriteString(fmt.Sprintf("- **巡检模版**: %s (适用厂商: %s)\n", tplName, tplVendor))
		}
		sb.WriteString(fmt.Sprintf("- **检查项数量**: %d 项\n\n", len(checks)))
		sb.WriteString("## 检查项目及输出详情\n\n")
		for k, v := range results {
			sb.WriteString(fmt.Sprintf("### %s\n", k))
			if item, ok := v.(map[string]interface{}); ok {
				sb.WriteString(fmt.Sprintf("```\n%v\n```\n\n", item["stdout"]))
			}
		}
		metaJSON, _ := json.Marshal(map[string]interface{}{
			"target_ids":     []string{id},
			"target_type":    "server",
			"inspectionType": req.InspectionType,
			"template_id":    req.TemplateID,
			"template_name":  tplName,
			"total":          1,
			"success_count":  1,
			"failed_count":   0,
		})
		_, _ = appdb.DB().Exec(`
			INSERT INTO reports (id, name, type, content, format, metadata, created_at, updated_at)
			VALUES (?, ?, 'inspection', ?, 'markdown', ?, datetime('now','localtime'), datetime('now','localtime'))`,
			repID, repName, sb.String(), string(metaJSON))

		httpx.Respond(w, results)
	})

	// POST /batch-compliance 或 POST /batch-inspect — 批量服务器合规巡检
	r.With(middleware.RequireRole("admin")).Post("/batch-compliance", handleBatchServerInspection)
	r.With(middleware.RequireRole("admin")).Post("/batch-inspect", handleBatchServerInspection)

	return r
}

// handleBatchServerInspection 执行批量服务器巡检
func handleBatchServerInspection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServerIDs      []string `json:"serverIds"`
		ServerIDsSnake []string `json:"server_ids"`
		InspectionType string   `json:"inspectionType"`
		TemplateID     string   `json:"templateId"`
		UseAI          *bool    `json:"useAI"`
		Concurrency    *int     `json:"concurrency"`
	}
	if !parseJSON(w, r, &req) {
		return
	}

	targetIDs := req.ServerIDs
	if len(targetIDs) == 0 {
		targetIDs = req.ServerIDsSnake
	}
	if len(targetIDs) == 0 {
		httpx.RespondError(w, "serverIds is required", http.StatusBadRequest)
		return
	}

	useAI := true
	if req.UseAI != nil {
		useAI = *req.UseAI
	}
	concurrency := 5
	if req.Concurrency != nil && *req.Concurrency > 0 {
		concurrency = *req.Concurrency
	}
	if concurrency > 8 {
		concurrency = 8
	}

	inspType := req.InspectionType
	if inspType == "" {
		inspType = "standard"
	}

	results := BatchInspectServers(targetIDs, useAI, concurrency, inspType, req.TemplateID)
	httpx.Respond(w, results)
}

// BatchInspectServers 并发批量巡检服务器并生成报告
func BatchInspectServers(serverIDs []string, useAI bool, concurrency int, inspectionType string, templateID string) map[string]interface{} {
	type serverTask struct {
		id string
	}

	taskCh := make(chan serverTask, len(serverIDs))
	for _, id := range serverIDs {
		taskCh <- serverTask{id: id}
	}
	close(taskCh)

	var mu sync.Mutex
	serverResults := make(map[string]interface{})
	successCount := 0
	failedCount := 0

	var wg sync.WaitGroup
	if concurrency > len(serverIDs) {
		concurrency = len(serverIDs)
	}
	if concurrency < 1 {
		concurrency = 1
	}

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				res, err := inspectSingleServer(task.id, useAI, inspectionType, templateID)
				mu.Lock()
				if err != nil {
					failedCount++
					serverResults[task.id] = map[string]interface{}{
						"success": false,
						"error":   err.Error(),
					}
				} else {
					successCount++
					serverResults[task.id] = map[string]interface{}{
						"success": true,
						"checks":  res,
					}
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// 自动生成一份批量巡检报告记录入库 reports 表
	reportID := uuid.NewString()
	reportName := fmt.Sprintf("服务器批量巡检报告 (%d台) - %s", len(serverIDs), time.Now().Format("2006-01-02 15:04"))
	if inspectionType == "custom" || inspectionType == "brand_auto" {
		reportName = fmt.Sprintf("服务器模版联动巡检报告 (%d台) - %s", len(serverIDs), time.Now().Format("2006-01-02 15:04"))
	}

	summaryContent := generateServerBatchReportContent(serverIDs, serverResults, successCount, failedCount, inspectionType, templateID)
	metaJSON, _ := json.Marshal(map[string]interface{}{
		"target_ids":     serverIDs,
		"target_type":    "server",
		"inspectionType": inspectionType,
		"template_id":    templateID,
		"total":          len(serverIDs),
		"success_count":  successCount,
		"failed_count":   failedCount,
		"server_results": serverResults,
	})

	_, _ = appdb.DB().Exec(`
		INSERT INTO reports (id, name, type, content, format, metadata, created_at, updated_at)
		VALUES (?, ?, 'inspection', ?, 'markdown', ?, datetime('now','localtime'), datetime('now','localtime'))`,
		reportID, reportName, summaryContent, string(metaJSON))

	return map[string]interface{}{
		"report_id":     reportID,
		"total":         len(serverIDs),
		"success_count": successCount,
		"failed_count":  failedCount,
		"results":       serverResults,
	}
}

// inspectSingleServer 巡检单台服务器
// templateID 非空时从 inspection_templates 表读取模版指令；
// inspectionType == "custom" 或 "brand_auto" 且 templateID 为空时，按品牌智能查找专属模版；
// 否则使用内置默认检查项
func inspectSingleServer(id string, useAI bool, inspectionType string, templateID string) (map[string]interface{}, error) {
	cfg, err := buildServerSSHConfig(id)
	if err != nil {
		return nil, fmt.Errorf("获取凭证失败: %w", err)
	}

	client, err := services.SSHDial(cfg)
	if err != nil {
		return nil, fmt.Errorf("SSH连接失败: %w", err)
	}
	defer client.Close()

	var sName, sVendor, sOSType string
	_ = appdb.DB().QueryRow("SELECT name, vendor, os_type FROM servers WHERE id = ?", id).Scan(&sName, &sVendor, &sOSType)

	var checks map[string]string
	inspectionTitle := "批量合规巡检"

	if templateID != "" {
		tplDetail, err := inspection.GetTemplateDetail(templateID)
		if err == nil && len(tplDetail.Commands) > 0 {
			checks = tplDetail.CommandMap()
			inspectionTitle = fmt.Sprintf("自定义模版巡检 (%s)", tplDetail.Name)
			inspection.IncrementTemplateUsage(templateID)
		}
	}

	if len(checks) == 0 && (inspectionType == "custom" || inspectionType == "brand_auto") {
		// 按照服务器所属品牌智能匹配该品牌专属巡检模版
		bestTpl, err := inspection.FindBestTemplateForServer(sVendor, sOSType)
		if err == nil && len(bestTpl.Commands) > 0 {
			checks = bestTpl.CommandMap()
			inspectionTitle = fmt.Sprintf("品牌专属巡检 (%s)", bestTpl.Name)
			inspection.IncrementTemplateUsage(bestTpl.ID)
		}
	}

	if len(checks) == 0 {
		checks = defaultServerChecks()
	}

	results := map[string]interface{}{}
	for name, cmd := range checks {
		start := time.Now()
		stdout, stderr, execErr := client.ExecCommand(cmd, 25*time.Second)
		duration := time.Since(start).Milliseconds()
		success := execErr == nil
		if !success {
			stdout = fmt.Sprintf("执行失败: %v", execErr)
		}
		results[name] = map[string]interface{}{
			"success":    success,
			"stdout":     stdout,
			"stderr":     stderr,
			"command":    cmd,
			"duration":   duration,
			"aiAnalysis": aiAnalysisForCheck(name, stdout, useAI),
		}
	}

	// 记录到 compliance_checks 表
	summary, _ := json.Marshal(results)
	_ = repos.Servers.InsertComplianceCheck(id, inspectionTitle, string(summary), "completed")

	// 更新服务器最后连接时间
	_, _ = appdb.DB().Exec(`UPDATE servers SET last_connected = datetime('now','localtime') WHERE id = ?`, id)

	return results, nil
}

// generateServerBatchReportContent 生成批量服务器巡检汇总 Markdown
func generateServerBatchReportContent(serverIDs []string, results map[string]interface{}, successCount, failedCount int, inspectionType, templateID string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 服务器集群批量巡检报告\n\n"))
	sb.WriteString(fmt.Sprintf("- **巡检时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("- **受检服务器总数**: %d 台\n", len(serverIDs)))
	sb.WriteString(fmt.Sprintf("- **巡检成功**: %d 台\n", successCount))
	sb.WriteString(fmt.Sprintf("- **巡检异常/离线**: %d 台\n", failedCount))

	modeDesc := "标准安全与健康巡检"
	if inspectionType == "custom" {
		if templateID != "" {
			tpl, _ := inspection.GetTemplateDetail(templateID)
			if tpl != nil {
				modeDesc = fmt.Sprintf("自定义模版驱动巡检 (%s)", tpl.Name)
			} else {
				modeDesc = "自定义模版驱动巡检"
			}
		} else {
			modeDesc = "自定义巡检 (品牌专属模版自动对应匹配)"
		}
	} else if inspectionType == "brand_auto" {
		modeDesc = "多品牌智能联动巡检 (根据每台设备所属品牌自动加载专属模版)"
	}
	sb.WriteString(fmt.Sprintf("- **巡检模式**: %s\n\n", modeDesc))

	sb.WriteString("## 一、服务器巡检状态总览\n\n")
	sb.WriteString("| 序号 | 服务器名称 | IP 地址 / 主机名 | 端口 | 巡检状态 | 详细诊断 |\n")
	sb.WriteString("| --- | --- | --- | --- | --- | --- |\n")

	for i, id := range serverIDs {
		var name, hostname string
		var port int
		_ = appdb.DB().QueryRow("SELECT name, hostname, port FROM servers WHERE id = ?", id).Scan(&name, &hostname, &port)
		if name == "" {
			name = id
		}

		resData, exists := results[id].(map[string]interface{})
		status := "❌ 异常/连接失败"
		detail := "无法连接 SSH 服务"
		if exists && resData["success"] == true {
			status = "✅ 正常"
			detail = "各项合规项与指标采集正常"
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %d | %s | %s |\n", i+1, name, hostname, port, status, detail))
	}

	sb.WriteString("\n## 二、巡检建议与总结\n\n")
	if failedCount > 0 {
		sb.WriteString(fmt.Sprintf("> ⚠️ 提示：本次巡检发现有 %d 台服务器无法正常连接，请排查 SSH 凭证、安全组策略与服务器在线状态。\n\n", failedCount))
	} else {
		sb.WriteString("> ✅ 所有受检服务器均连通正常，安全配置与基础监控指标均在健康阈值内。\n\n")
	}

	return sb.String()
}

// aiAnalysisForCheck 简单的合规分析（AI 增强未接入时返回基础结论）
func aiAnalysisForCheck(name, output string, useAI bool) string {
	if !useAI {
		return ""
	}
	output = normalizeWhitespace(output)
	// 简单规则分析
	switch name {
	case "系统版本":
		if contains(output, "Ubuntu") || contains(output, "Debian") {
			return "Debian 系操作系统"
		}
		if contains(output, "CentOS") || contains(output, "Red Hat") || contains(output, "Rocky") || contains(output, "AlmaLinux") {
			return "RedHat 系操作系统"
		}
		return "未知操作系统"
	case "CPU负载":
		if contains(output, "load average") {
			return "CPU 负载信息已采集"
		}
		return "CPU 负载采集异常"
	case "内存使用":
		if contains(output, "Mem:") || contains(output, "内存") {
			return "内存使用信息已采集"
		}
		return "内存信息采集异常"
	case "磁盘使用":
		if contains(output, "/") && contains(output, "%") {
			return "磁盘使用情况已采集"
		}
		return "磁盘信息采集异常"
	case "SSH配置":
		if contains(output, "PermitRootLogin yes") {
			return "⚠️ 允许 root 直接登录，存在安全风险"
		}
		if contains(output, "PasswordAuthentication yes") {
			return "⚠️ 允许密码认证，建议改用密钥认证"
		}
		return "SSH 配置未发现明显风险"
	case "开放端口":
		if contains(output, "N/A") {
			return "无法获取端口信息"
		}
		return fmt.Sprintf("检测到开放端口信息（%d 行）", lineCount(output))
	}
	return ""
}

// normalizeWhitespace 压缩空白
func normalizeWhitespace(s string) string {
	out := make([]rune, 0, len(s))
	lastSpace := false
	for _, c := range s {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if !lastSpace {
				out = append(out, ' ')
				lastSpace = true
			}
			continue
		}
		out = append(out, c)
		lastSpace = false
	}
	return string(out)
}

// lineCount 统计行数
func lineCount(s string) int {
	count := 0
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}
	return count
}

// defaultServerChecks 返回默认服务器巡检检查项（当无模版时使用）
func defaultServerChecks() map[string]string {
	return map[string]string{
		"系统版本":  "cat /etc/os-release 2>/dev/null | head -5 || ver",
		"内核版本":  "uname -a",
		"运行时间":  "uptime",
		"CPU负载": "top -bn1 | head -5",
		"内存使用":  "free -m",
		"磁盘使用":  "df -h",
		"最近登录":  "last -n 10 2>/dev/null || echo N/A",
		"开放端口":  "ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null || echo N/A",
		"SSH配置": "grep -E 'PermitRootLogin|PasswordAuthentication' /etc/ssh/sshd_config 2>/dev/null || echo N/A",
		"防火墙状态": "systemctl is-active firewalld 2>/dev/null || ufw status 2>/dev/null || echo N/A",
	}
}

// boolToInt bool 转 int
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
