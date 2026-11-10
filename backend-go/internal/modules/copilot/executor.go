// Copilot 多设备只读指令分发与并发执行器
// 支持识别目标设备范围（单机、交换机群、服务器群、全网）、安全审查与真实 SSH 并发采集
package copilot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"itops-backend/internal/appdb"
	"itops-backend/internal/crypto"
	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

// DeviceQueryResult 单设备查询结果
type DeviceQueryResult struct {
	DeviceID     string `json:"device_id"`
	DeviceName   string `json:"device_name"`
	IP           string `json:"ip"`
	DeviceType   string `json:"device_type"` // switch, router, firewall, server
	Vendor       string `json:"vendor"`
	Status       string `json:"status"` // success, stale, timeout, failed, blocked, cancelled
	DurationMs   int64  `json:"duration_ms"`
	Command      string `json:"command"`
	Output       string `json:"output"`
	ErrorMessage string `json:"error_message,omitempty"`
	Source       string `json:"source"` // live_ssh, cached, none
}

// targetDeviceItem 待执行设备（网络设备或服务器）
type targetDeviceItem struct {
	isServer   bool
	netDev     *repos.NetworkDevice
	serverItem *repos.Server
}

// resolveTargetDevices 解析目标设备范围：精确名称/IP 优先，其次设备类别关键字，最后模糊匹配
// 返回 (设备列表, 未找到提示)。未找到明确目标时绝不退化为“全量设备”。
func resolveTargetDevices(targetScope string) ([]targetDeviceItem, string) {
	allNetDevices, _ := repos.NetworkDevices.List()
	allServers, _ := repos.Servers.List()
	scope := strings.TrimSpace(targetScope)
	scopeLower := strings.ToLower(scope)

	// 1. 精确匹配设备名称或管理 IP（优先级最高，避免“核心交换机-1”被当成类别关键字）
	var exact []targetDeviceItem
	for i := range allNetDevices {
		dev := allNetDevices[i]
		if dev == nil {
			continue
		}
		if strings.EqualFold(dev.Name, scope) || (dev.IPAddress != "" && strings.EqualFold(dev.IPAddress, scope)) {
			exact = append(exact, targetDeviceItem{netDev: dev})
		}
	}
	for i := range allServers {
		srv := &allServers[i]
		nameHit := strings.EqualFold(srv.Name, scope)
		ipHit := srv.IPAddress != nil && *srv.IPAddress != "" && strings.EqualFold(*srv.IPAddress, scope)
		if nameHit || ipHit {
			exact = append(exact, targetDeviceItem{isServer: true, serverItem: srv})
		}
	}
	if len(exact) > 0 {
		return exact, ""
	}

	// 2. 设备类别关键字
	switch scopeLower {
	case "", "all", "全网", "全部":
		var targets []targetDeviceItem
		for i := range allNetDevices {
			if allNetDevices[i] != nil {
				targets = append(targets, targetDeviceItem{netDev: allNetDevices[i]})
			}
		}
		for i := range allServers {
			targets = append(targets, targetDeviceItem{isServer: true, serverItem: &allServers[i]})
		}
		if len(targets) == 0 {
			return nil, "⚠️ 当前系统中暂无已纳管的网络设备或服务器，无法执行指令查询。"
		}
		return targets, ""
	case "network", "networks", "switches", "switch", "交换机", "网络", "交换机群", "网络设备":
		var targets []targetDeviceItem
		for i := range allNetDevices {
			if allNetDevices[i] != nil {
				targets = append(targets, targetDeviceItem{netDev: allNetDevices[i]})
			}
		}
		if len(targets) == 0 {
			return nil, "⚠️ 当前系统中暂无已纳管的网络设备，无法执行指令查询。"
		}
		return targets, ""
	case "servers", "server", "hosts", "host", "服务器", "主机", "服务器群":
		var targets []targetDeviceItem
		for i := range allServers {
			targets = append(targets, targetDeviceItem{isServer: true, serverItem: &allServers[i]})
		}
		if len(targets) == 0 {
			return nil, "⚠️ 当前系统中暂无已纳管的服务器主机，无法执行指令查询。"
		}
		return targets, ""
	}

	// 3. 名称/IP 关键字模糊匹配
	var fuzzy []targetDeviceItem
	for i := range allNetDevices {
		dev := allNetDevices[i]
		if dev == nil {
			continue
		}
		if strings.Contains(strings.ToLower(dev.Name), scopeLower) || strings.Contains(strings.ToLower(dev.IPAddress), scopeLower) {
			fuzzy = append(fuzzy, targetDeviceItem{netDev: dev})
		}
	}
	for i := range allServers {
		srv := &allServers[i]
		ip := ""
		if srv.IPAddress != nil {
			ip = *srv.IPAddress
		}
		if strings.Contains(strings.ToLower(srv.Name), scopeLower) || (ip != "" && strings.Contains(strings.ToLower(ip), scopeLower)) {
			fuzzy = append(fuzzy, targetDeviceItem{isServer: true, serverItem: srv})
		}
	}
	if len(fuzzy) > 0 {
		return fuzzy, ""
	}

	// 4. 未找到明确目标：不执行任何设备
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚠️ **未找到匹配的目标设备**：`%s`\n\n", scope))
	sb.WriteString("为避免指令误发到非目标设备，本次 **未向任何设备执行查询**。当前纳管设备如下：\n\n")
	for _, d := range allNetDevices {
		if d != nil {
			sb.WriteString(fmt.Sprintf("- 网络设备：**%s** (`%s`)\n", d.Name, d.IPAddress))
		}
	}
	for i := range allServers {
		srv := &allServers[i]
		ip := ""
		if srv.IPAddress != nil {
			ip = *srv.IPAddress
		}
		sb.WriteString(fmt.Sprintf("- 服务器主机：**%s** (`%s`)\n", srv.Name, ip))
	}
	sb.WriteString("\n> 💡 请使用准确的设备名称或管理 IP 重试，例如「**执行指令 df -h**」。")
	return nil, sb.String()
}

func targetDeviceID(it targetDeviceItem) string {
	if it.isServer {
		return it.serverItem.ID
	}
	return it.netDev.ID
}

func targetDeviceName(it targetDeviceItem) string {
	if it.isServer {
		return it.serverItem.Name
	}
	return it.netDev.Name
}

func targetDeviceIP(it targetDeviceItem) string {
	if it.isServer {
		if it.serverItem.IPAddress != nil {
			return *it.serverItem.IPAddress
		}
		return ""
	}
	return it.netDev.IPAddress
}

// ExecuteReadOnlyQueryOnDevices 执行多设备只读指令查询
// targetScope: "switches", "servers", "all", 或具体设备名称/IP
// role: 当前用户角色，用于 middleware.CheckCommandSafety 策略校验
func ExecuteReadOnlyQueryOnDevices(ctx context.Context, targetScope string, cmd string, role string) (string, []DeviceQueryResult) {
	startAll := time.Now()

	// 1. 安全指令审查（第一道绝对硬隔离红线）
	cmd = strings.TrimSpace(cmd)
	allowed, blockReason := ValidateReadOnlyCommand("", cmd)
	if !allowed {
		logger.Warnf("安全拦截：AI 尝试执行非只读指令 [%s]: %s", cmd, blockReason)
		return GenerateSecurityBlockNotice(cmd, blockReason), nil
	}

	// 1.1 执行前的角色策略校验（在建立任何 SSH 连接之前）
	if strings.TrimSpace(role) == "" {
		role = "admin"
	}
	if safety := middleware.CheckCommandSafety(cmd, role); !safety.Allowed {
		reason := safety.Reason
		if reason == "" {
			reason = "命令未通过当前角色的安全策略校验"
		}
		logger.Warnf("安全拦截：角色 [%s] 执行指令 [%s] 被策略阻断: %s", role, cmd, reason)
		return GenerateSecurityBlockNotice(cmd, reason), nil
	}

	// 2. 解析与筛选目标设备
	targets, notFoundMsg := resolveTargetDevices(targetScope)
	if notFoundMsg != "" {
		return notFoundMsg, nil
	}

	// 3. 并发执行指令（Goroutines + WaitGroup，遵循 ctx 取消）
	var mu sync.Mutex
	var results []DeviceQueryResult
	var wg sync.WaitGroup

	for _, item := range targets {
		wg.Add(1)
		go func(it targetDeviceItem) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				mu.Lock()
				results = append(results, DeviceQueryResult{
					DeviceID:     targetDeviceID(it),
					DeviceName:   targetDeviceName(it),
					IP:           targetDeviceIP(it),
					Command:      cmd,
					Status:       "cancelled",
					Source:       "none",
					ErrorMessage: "请求已取消或超时，未执行 SSH",
				})
				mu.Unlock()
				return
			default:
			}

			var res DeviceQueryResult
			if it.isServer {
				res = executeOnServer(it.serverItem, cmd)
			} else {
				res = executeOnNetworkDevice(it.netDev, cmd)
			}

			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}(item)
	}

	wg.Wait()
	totalDuration := time.Since(startAll).Milliseconds()

	// 4. 统计成功、缓存快照与失败
	successCount := 0
	staleCount := 0
	failedCount := 0
	for _, r := range results {
		switch r.Status {
		case "success":
			successCount++
		case "stale":
			staleCount++
		default:
			failedCount++
		}
	}

	// 5. 格式化输出清晰明了的结构化列表
	var sb strings.Builder
	sb.WriteString("### 🛡️ 真实设备指令执行与配置查询报告\n\n")
	sb.WriteString("> **安全防护机制**：`只读控制策略已强制启用 (Read-Only Enforced)` | `配置下发与变更: 已阻断 (Write: Forbidden)`\n\n")

	sb.WriteString("| 执行指标 | 运行数据 |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **查询指令** | `%s` |\n", strings.ReplaceAll(cmd, "`", "'")))
	sb.WriteString(fmt.Sprintf("| **目标设备总数** | `%d 台`（实时成功 `%d`，缓存快照 `%d`，异常/超时 `%d`） |\n", len(results), successCount, staleCount, failedCount))
	sb.WriteString(fmt.Sprintf("| **总调度耗时** | `%d ms` |\n", totalDuration))
	sb.WriteString("| **安全审计** | `全部只读白名单校验通过，无任何配置下发或状态修改` |\n\n")

	sb.WriteString("---\n\n")
	sb.WriteString("### 📋 各设备返回结果列表\n\n")

	for idx, r := range results {
		statusBadge := "🟢 执行成功"
		switch r.Status {
		case "success":
		case "stale":
			statusBadge = "🟡 缓存快照（非实时）"
		case "cancelled":
			statusBadge = "⚪ 已取消"
		default:
			statusBadge = "🔴 " + r.Status
		}
		vendorTag := r.Vendor
		if vendorTag == "" {
			vendorTag = "通用"
		}

		sb.WriteString(fmt.Sprintf("#### %d. 🖥️ %s (%s)\n", idx+1, r.DeviceName, r.IP))
		sb.WriteString(fmt.Sprintf("* **规格属性**：品牌/系统 `%s` · 类别 `%s` · 耗时 `%d ms` · 状态 `%s`", vendorTag, r.DeviceType, r.DurationMs, statusBadge))
		if r.Source == "cached" {
			sb.WriteString(" · *(设备不可达，以下为最近一次同类巡检的缓存快照，非本次实时回显)*")
		}
		sb.WriteString("\n\n")

		outputContent := strings.TrimSpace(r.Output)
		if outputContent == "" {
			if r.ErrorMessage != "" {
				outputContent = fmt.Sprintf("执行未获取到回显数据: %s", r.ErrorMessage)
			} else {
				outputContent = "设备执行完毕，无文本回显输出。"
			}
		}

		// 限制超长文本，保证可读性
		if len(outputContent) > 3500 {
			outputContent = outputContent[:3500] + "\n\n... [为保证排版性能，超出 3500 字符部分已自动截断，配置已完整执行] ..."
		}

		sb.WriteString("```shell\n")
		sb.WriteString(outputContent)
		sb.WriteString("\n```\n\n")
	}

	// 6. AI 智能交叉比对与提炼分析
	sb.WriteString("---\n\n")
	sb.WriteString("### 🤖 NetMon AI 智能诊断与提炼总结\n\n")
	aiSummary := generateAIInsightOnResults(cmd, results)
	sb.WriteString(aiSummary)

	return sb.String(), results
}

// executeOnNetworkDevice 执行网络交换机只读命令
func executeOnNetworkDevice(dev *repos.NetworkDevice, cmd string) DeviceQueryResult {
	start := time.Now()
	res := DeviceQueryResult{
		DeviceID:   dev.ID,
		DeviceName: dev.Name,
		IP:         dev.IPAddress,
		DeviceType: "交换机/网络设备",
		Vendor:     dev.Vendor,
		Command:    cmd,
		Status:     "success",
		Source:     "live_ssh",
	}

	cred, err := services.NetworkDeviceServiceInstance.GetFullCredentials(dev.ID)
	if err != nil {
		res.Status = "error"
		res.ErrorMessage = "获取设备SSH凭证失败: " + err.Error()
		res.DurationMs = time.Since(start).Milliseconds()
		return res
	}

	sshDevice := services.SSHDeviceConfig{
		Vendor:         cred.Vendor,
		Host:           cred.IPAddress,
		Port:           cred.SSHPort,
		Username:       cred.Username,
		Password:       cred.Password,
		EnablePassword: cred.EnablePassword,
		PrivateKey:     cred.PrivateKey,
		AuthType:       cred.AuthType,
	}

	// 针对不同厂商组装禁用分页命令
	pagingCmd := "screen-length 0 temporary"
	vLower := strings.ToLower(cred.Vendor)
	if strings.Contains(vLower, "cisco") || strings.Contains(vLower, "ruijie") {
		pagingCmd = "terminal length 0"
	}

	// 尝试 SSH 执行（超时 8s）
	out, sshErr := services.ConnectAndRunShellWithTimeout(sshDevice, []string{pagingCmd, cmd}, 8*time.Second)
	res.DurationMs = time.Since(start).Milliseconds()

	if sshErr == nil && strings.TrimSpace(out) != "" {
		// 清除分页命令痕迹与提示符
		cleaned := services.ExtractCommandOutput(out, cmd)
		if cleaned != "" {
			res.Output = strings.TrimSpace(cleaned)
		} else {
			res.Output = strings.TrimSpace(out)
		}
		return res
	}

	// 实时 SSH 失败后，仅允许使用同类型巡检快照兜底
	// 只有在缓存命令类型与本次指令完全一致时，才使用巡检快照，并明确标记 stale/cached
	if wantType := cachedCommandType(cmd); wantType != "" {
		if cachedOut, ok := findCachedOutputForDevice(dev.ID, wantType); ok {
			res.Status = "stale"
			res.Source = "cached"
			res.Output = cachedOut
			res.DurationMs = time.Since(start).Milliseconds()
			return res
		}
	}
	// 若无缓存且连接失败，输出清晰的连接状态提示
	res.Status = "timeout"
	if sshErr != nil {
		res.ErrorMessage = fmt.Sprintf("SSH 连接/执行超时 (8s): %v", sshErr)
		res.Output = fmt.Sprintf("⚠️ 目标设备 %s (%s) 无法建立实时 SSH 会话 (原因: %v)。\n请检查设备管理 IP 是否可通、SSH 端口 %d 是否开放，或确认鉴权密码是否正确。", dev.Name, dev.IPAddress, sshErr, dev.SSHPort)
	} else {
		res.ErrorMessage = "未接收到回显数据"
	}

	return res
}

// executeOnServer 执行服务器主机命令
func executeOnServer(srv *repos.Server, cmd string) DeviceQueryResult {
	start := time.Now()
	ip := ""
	if srv.IPAddress != nil {
		ip = *srv.IPAddress
	}
	osName := "Linux"
	if srv.OS != nil {
		osName = *srv.OS
	}

	res := DeviceQueryResult{
		DeviceID:   srv.ID,
		DeviceName: srv.Name,
		IP:         ip,
		DeviceType: "服务器主机",
		Vendor:     osName,
		Command:    cmd,
		Status:     "success",
		Source:     "live_ssh",
	}

	// 获取服务器凭证
	username, password, privateKey, useSSHKey, err := repos.Servers.GetCredentialFields(srv.ID)
	if err != nil {
		res.Status = "error"
		res.ErrorMessage = "获取服务器凭据失败: " + err.Error()
		res.DurationMs = time.Since(start).Milliseconds()
		return res
	}

	// 若未直接配置密码，尝试从 ssh_keys 获取
	if password == "" && privateKey == "" && srv.SSHKeyID != nil && *srv.SSHKeyID != "" {
		if key, kerr := repos.SshKeys.GetByID(*srv.SSHKeyID); kerr == nil && key != nil {
			if key.Username != nil && *key.Username != "" {
				username = *key.Username
			}
			if key.Password != nil && *key.Password != "" {
				if dec, derr := crypto.Decrypt(*key.Password); derr == nil {
					password = dec
				}
			}
			if key.PrivateKey != nil && *key.PrivateKey != "" {
				if dec, derr := crypto.Decrypt(*key.PrivateKey); derr == nil {
					privateKey = dec
				}
			}
		}
	}

	port := 22
	if srv.Port > 0 {
		port = srv.Port
	}

	authType := "password"
	if useSSHKey == 1 || privateKey != "" {
		authType = "key"
	}

	sshCfg := services.SSHDeviceConfig{
		Host:       ip,
		Port:       port,
		Username:   username,
		Password:   password,
		PrivateKey: privateKey,
		AuthType:   authType,
	}

	client, err := services.SSHDialTimeout(sshCfg, 8*time.Second)
	if err != nil {
		res.Status = "error"
		res.ErrorMessage = "SSH 连接失败: " + err.Error()
		res.DurationMs = time.Since(start).Milliseconds()
		res.Output = fmt.Sprintf("⚠️ 无法连接至服务器 %s (%s:%d): %v", srv.Name, ip, port, err)
		return res
	}
	defer client.Close()

	stdout, stderr, execErr := client.ExecCommand(cmd, 10*time.Second)
	res.DurationMs = time.Since(start).Milliseconds()

	if execErr != nil {
		res.Status = "error"
		res.ErrorMessage = execErr.Error()
		res.Output = stdout
		if stderr != "" {
			if res.Output != "" {
				res.Output += "\n[stderr]: " + stderr
			} else {
				res.Output = "[stderr]: " + stderr
			}
		}
	} else {
		res.Output = stdout
	}

	return res
}

// cachedCommandType 将只读指令映射为巡检快照中的命令类型
// 仅用于“同类型缓存”判定；无法精确对应时返回空串，表示不允许使用缓存。
func cachedCommandType(cmd string) string {
	c := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(cmd))), " ")
	switch c {
	case "display version", "show version":
		return "version"
	case "display vlan", "show vlan", "show vlan brief":
		return "vlan"
	case "display stp", "display stp brief", "show spanning-tree brief":
		return "stp"
	case "display arp", "show ip arp":
		return "arp"
	case "display mac-address", "show mac address-table":
		return "mac"
	case "display ip routing-table", "show ip route":
		return "routes"
	case "display interface brief", "show ip interface brief":
		return "interface"
	case "display cpu-usage":
		return "cpu"
	case "display memory-usage":
		return "memory"
	case "display logbuffer":
		return "log"
	case "display environment", "display temperature":
		return "environment"
	case "display power":
		return "power"
	case "display fan":
		return "fan"
	case "display lldp neighbor brief", "show lldp neighbors detail":
		return "neighbor"
	case "display ntp status", "display ntp-service status":
		return "ntp"
	}
	// 巡检快照中的 config_checksum 仅包含配置摘要，不能冒充全量配置回显
	return ""
}

// findCachedOutputForDevice 在历史巡检快照中查找“同类型”指令的输出
// 只有 wantType 与快照条目类型完全一致时才返回，绝不回退到任意历史输出。
func findCachedOutputForDevice(deviceID string, wantType string) (string, bool) {
	if wantType == "" {
		return "", false
	}
	db := appdb.DB()
	if db == nil {
		return "", false
	}

	var resultsStr sql.NullString
	err := db.QueryRow("SELECT results FROM network_inspection_history WHERE device_id = ? ORDER BY created_at DESC LIMIT 1", deviceID).Scan(&resultsStr)
	if err != nil || !resultsStr.Valid || resultsStr.String == "" {
		return "", false
	}

	type cachedItem struct {
		Type      string `json:"type"`
		RawOutput string `json:"rawOutput"`
	}
	var items []cachedItem
	if err := json.Unmarshal([]byte(resultsStr.String), &items); err != nil {
		return "", false
	}

	for _, it := range items {
		if it.Type == wantType && it.RawOutput != "" {
			return it.RawOutput, true
		}
	}
	return "", false

}

// generateAIInsightOnResults 对多设备返回结果进行交叉对比分析
func generateAIInsightOnResults(cmd string, results []DeviceQueryResult) string {
	var summaryText strings.Builder

	liveCount := 0
	staleCount := 0
	failedCount := 0
	for _, r := range results {
		switch r.Status {
		case "success":
			liveCount++
		case "stale":
			staleCount++
		default:
			failedCount++
		}
	}

	summaryText.WriteString(fmt.Sprintf("1. **采集覆盖度**：指令 `%s` 共覆盖 **%d** 台设备，其中实时 SSH 回显 **%d** 台，缓存快照（非实时）**%d** 台，执行失败/超时 **%d** 台。\n", cmd, len(results), liveCount, staleCount, failedCount))
	if staleCount > 0 {
		summaryText.WriteString("   *缓存快照仅用于临时参考，不代表本次实时设备状态，请以实时采集结果为准。*\n")
	}

	if liveCount == 0 {
		summaryText.WriteString("\n2. **结论**：本次没有获取到任何实时设备回显，无法给出设备侧分析结论。请先排查设备可达性与 SSH 凭证后重试。\n")
		summaryText.WriteString("\n> 💡 **操作提示**：可先执行「巡检核心交换机-1」确认设备连通性，或检查设备管理 IP / SSH 端口配置。")
		return summaryText.String()
	}

	cmdLower := strings.ToLower(cmd)
	if strings.Contains(cmdLower, "version") {
		summaryText.WriteString("2. **系统版本**：请以各设备实时回显中的版本号为准进行基线比对，重点关注存在差异的固件版本。\n")
	} else if strings.Contains(cmdLower, "current-configuration") || strings.Contains(cmdLower, "config") {
		summaryText.WriteString("2. **配置查询**：配置回显已原样呈现，只读查询未触发任何写操作；请人工核对关键配置项差异。\n")
	} else if strings.Contains(cmdLower, "interface") {
		summaryText.WriteString("2. **端口状态**：请结合各设备回显中的 UP/DOWN 数量与错包计数进行链路排查。\n")
	} else if strings.Contains(cmdLower, "routing") || strings.Contains(cmdLower, "route") {
		summaryText.WriteString("2. **路由状态**：请以各设备返回的路由表条目为准，核对是否存在缺失或异常路由。\n")
	} else if strings.Contains(cmdLower, "df") || strings.Contains(cmdLower, "disk") {
		summaryText.WriteString("2. **磁盘资源**：请以各主机实时返回的挂载点使用率为准评估容量风险。\n")
	}

	summaryText.WriteString("\n> 💡 **操作提示**：如需进一步查询特定设备的详细端口计数器或邻居拓扑，您可以继续提问，例如 `display interface brief`。")

	return summaryText.String()
}
