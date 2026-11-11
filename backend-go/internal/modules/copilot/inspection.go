// Copilot 智能运维助手 — 设备巡检调度与健康诊断报告生成器
// 支持自然语言意图识别、单机/批量巡检、基于真实采集数据的健康评分与数据库持久化
// 原则：报告只呈现实测/在库数据，未采集的维度明确标注「未采集」，绝不编造指标
package copilot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"itops-backend/internal/appdb"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

// isInspectionQuery 识别巡检意图与目标设备
func isInspectionQuery(rawQuery string) (bool, string, string) {
	lower := strings.ToLower(rawQuery)

	// 1. 关键词判断：是否包含巡检相关诉求
	isInspect := strings.Contains(lower, "巡检") ||
		strings.Contains(lower, "体检") ||
		strings.Contains(lower, "健康检查") ||
		strings.Contains(lower, "合规检查") ||
		strings.Contains(lower, "检查设备") ||
		strings.Contains(lower, "设备体检") ||
		strings.Contains(lower, "发起巡检") ||
		strings.Contains(lower, "执行巡检")

	if !isInspect {
		return false, "", ""
	}

	// 2. 识别巡检类型
	inspType := "standard"
	if strings.Contains(lower, "深度") || strings.Contains(lower, "全面") || strings.Contains(lower, "full") {
		inspType = "full"
	} else if strings.Contains(lower, "快速") || strings.Contains(lower, "quick") {
		inspType = "quick"
	}

	// 3. 目标设备范围解析：显式全部/批量
	if strings.Contains(lower, "所有") ||
		strings.Contains(lower, "全部") ||
		strings.Contains(lower, "全网") ||
		strings.Contains(lower, "批量") ||
		strings.Contains(lower, "整网") ||
		strings.Contains(lower, "all") {
		return true, "all", inspType
	}

	// 4. 精准匹配在库设备名称或 IP
	netDevices, _ := repos.NetworkDevices.List()
	for _, d := range netDevices {
		if d == nil {
			continue
		}
		if strings.Contains(rawQuery, d.Name) || (d.IPAddress != "" && strings.Contains(rawQuery, d.IPAddress)) {
			return true, d.Name, inspType
		}
	}

	// 5. 未指定具体设备时，默认执行全网设备巡检
	return true, "all", inspType
}

// ExecuteInspectionFromChat 执行来自聊天窗口的巡检指令并生成结构化诊断报告
func ExecuteInspectionFromChat(ctx context.Context, targetScope, inspType string) string {
	start := time.Now()

	if targetScope == "all" {
		return executeAllDevicesInspection(ctx, inspType)
	}

	netDevices, _ := repos.NetworkDevices.List()
	var targetDev *repos.NetworkDevice
	for _, d := range netDevices {
		if d == nil {
			continue
		}
		if strings.EqualFold(d.Name, targetScope) || strings.EqualFold(d.IPAddress, targetScope) ||
			strings.Contains(strings.ToLower(d.Name), strings.ToLower(targetScope)) ||
			strings.Contains(d.IPAddress, targetScope) {
			targetDev = d
			break
		}
	}

	if targetDev == nil {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("⚠️ **未找到匹配的巡检设备**：`%s`\n\n", targetScope))
		sb.WriteString("当前纳管的有效设备列表如下，您可直接点击或发送指令重新发起巡检：\n\n")
		for _, d := range netDevices {
			statusBadge := "🟢 在线"
			if !isDeviceOnline(d) {
				statusBadge = "🔴 离线"
			}
			sb.WriteString(fmt.Sprintf("- **%s** (`%s`) · %s · 厂商: `%s`\n", d.Name, d.IPAddress, statusBadge, d.Vendor))
		}
		sb.WriteString("\n> 💡 **操作提示**：例如发送「**巡检 设备名称**」或「**巡检所有网络设备**」。")
		return sb.String()
	}

	return inspectSingleDeviceReport(targetDev, inspType, start)
}

func isDeviceOnline(d *repos.NetworkDevice) bool {
	s := strings.ToLower(d.Status)
	return s == "online" || s == "up"
}

// ───────────────────────────────────────────────
// 真实数据采集与健康评分
// ───────────────────────────────────────────────

// deviceSignals 一台设备本次巡检采集到的真实信号
type deviceSignals struct {
	sshOK        bool // SSH 巡检是否真实执行
	sshExecuted  int
	sshFailed    int
	sshSummary   string
	health       *services.DeviceHealth // SNMP 健康检查（nil = 未采集到）
	activeAlerts int                    // 在库未 resolved 告警数
	lastBackup   string                 // 最近一次配置备份时间（空 = 无记录）
}

// collectDeviceSignals 采集单台设备的真实巡检信号
func collectDeviceSignals(dev *repos.NetworkDevice, inspType string, withSSH bool) deviceSignals {
	sig := deviceSignals{}

	if withSSH {
		if res, err := services.NetworkInspectionServiceInstance.InspectDevice(dev.ID, inspType, nil); err == nil && res != nil {
			sig.sshOK = true
			sig.sshExecuted = res.CommandsExecuted
			sig.sshFailed = res.CommandsFailed
			sig.sshSummary = res.Summary
		}
	}

	sig.health = services.SnmpServiceInstance.HealthCheck(dev.ID, dev.IPAddress, 0)

	db := appdb.DB()
	_ = db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE device_id = ? AND status NOT IN ('resolved','closed')`, dev.ID).Scan(&sig.activeAlerts)
	_ = db.QueryRow(`SELECT COALESCE(MAX(created_at),'') FROM network_config_backups WHERE device_id = ? AND status = 'success'`, dev.ID).Scan(&sig.lastBackup)
	return sig
}

// scoreDevice 基于真实信号计算健康分；返回 -1 表示无任何采集数据、未评估
func scoreDevice(sig deviceSignals) int {
	hasSignal := false
	score := 100

	if sig.sshOK {
		hasSignal = true
		if sig.sshFailed > 0 {
			score -= min(sig.sshFailed*5, 20)
		}
	}
	if h := sig.health; h != nil {
		hasSignal = true
		if h.CpuUsage != nil {
			switch {
			case *h.CpuUsage > 90:
				score -= 25
			case *h.CpuUsage > 80:
				score -= 15
			}
		}
		if h.MemoryUsage != nil {
			switch {
			case *h.MemoryUsage > 90:
				score -= 25
			case *h.MemoryUsage > 80:
				score -= 15
			}
		}
		if h.Temperature != nil && *h.Temperature > 60 {
			score -= 10
		}
		if h.InterfaceHighUtil > 0 {
			score -= min(h.InterfaceHighUtil*5, 20)
		}
		if h.InterfaceCount > 0 && h.InterfacesDown > h.InterfacesUp {
			score -= 5
		}
	}
	if sig.activeAlerts > 0 {
		hasSignal = true
		score -= min(sig.activeAlerts*8, 30)
	}

	if !hasSignal {
		return -1
	}
	return max(score, 0)
}

func scoreLabel(score int) string {
	switch {
	case score < 0:
		return "未评估"
	case score >= 90:
		return "优秀 🟢"
	case score >= 75:
		return "良好 🟡"
	case score >= 60:
		return "一般 🟠"
	default:
		return "异常 🔴"
	}
}

// ───────────────────────────────────────────────
// 单设备报告
// ───────────────────────────────────────────────

// inspectSingleDeviceReport 对单台设备执行巡检并输出卡片（全部基于实测数据）
func inspectSingleDeviceReport(dev *repos.NetworkDevice, inspType string, startTime time.Time) string {
	db := appdb.DB()
	nowStr := time.Now().Format("2006-01-02 15:04:05")

	// 1. 离线设备：基于平台记录状态出具排查报告（不做虚假的协议实测表述）
	if !isDeviceOnline(dev) {
		durationMs := time.Since(startTime).Milliseconds()
		summary := fmt.Sprintf("平台记录设备状态为离线（%s），本次未执行实测采集，建议排查物理链路与管理通道 (耗时 %dms)", dev.Status, durationMs)

		_, _ = db.Exec(`
			INSERT INTO network_inspection_history (id, device_id, inspection_type, status, commands_executed, commands_failed, summary, duration_ms, created_at)
			VALUES (?, ?, ?, 'failed', 0, 0, ?, ?, ?)`,
			dev.ID+"-"+fmt.Sprintf("%d", time.Now().Unix()), dev.ID, inspType, summary, durationMs, nowStr)
		_ = repos.NetworkDevices.UpdateInspectionResult(dev.ID, summary)

		var sb strings.Builder
		sb.WriteString("### 🩺 NetMon 智能巡检健康诊断报告\n\n")
		sb.WriteString("> ⚠️ **巡检警报**：平台记录的设备状态为 **离线 (OFFLINE)**，本次巡检未采集到实测指标。\n\n")

		sb.WriteString("#### 1. 📌 设备资产与巡检元数据\n\n")
		sb.WriteString("| 属性 | 信息 |\n")
		sb.WriteString("| :--- | :--- |\n")
		sb.WriteString(fmt.Sprintf("| **设备名称** | **%s** |\n", dev.Name))
		sb.WriteString(fmt.Sprintf("| **管理 IP** | `%s` |\n", dev.IPAddress))
		sb.WriteString(fmt.Sprintf("| **设备品牌/型号** | `%s` / `%s` |\n", dev.Vendor, safeStr(dev.Model, "未登记")))
		sb.WriteString(fmt.Sprintf("| **记录状态** | 🔴 **离线 (%s)** |\n", dev.Status))
		sb.WriteString(fmt.Sprintf("| **巡检时间** | `%s` (耗时 `%d ms`) |\n\n", nowStr, durationMs))

		sb.WriteString("#### 2. 💡 排障建议\n\n")
		sb.WriteString("1. **物理层核查**：确认设备是否上架通电，排查上联光纤/网线链路。\n")
		sb.WriteString(fmt.Sprintf("2. **网关与路由核查**：检查平台宿主机至 `%s` 的路由可达性与 UDP 161 / TCP %d 端口放通。\n", dev.IPAddress, dev.SSHPort))
		sb.WriteString("3. **状态刷新**：设备恢复后等待下一轮探测自动更新状态，或在设备详情页手动触发 SNMP/SSH 测试。\n")
		sb.WriteString("4. **虚假设备清理**：若该设备为临时测试创建的不存在设备，请前往「**网络设备 ➔ 设备管理**」删除，避免误告警。\n\n")

		sb.WriteString("> 📝 **数据存档**：本次巡检结论已归档至 `network_inspection_history`。")
		return sb.String()
	}

	// 2. 在线设备：执行真实 SSH 巡检 + SNMP 健康检查 + 在库告警/备份查询
	sig := collectDeviceSignals(dev, inspType, true)
	score := scoreDevice(sig)
	durationMs := time.Since(startTime).Milliseconds()

	upPorts, downPorts := -1, -1
	if sig.health != nil && sig.health.InterfaceCount > 0 {
		upPorts = sig.health.InterfacesUp
		downPorts = sig.health.InterfacesDown
	} else if latest, _ := repos.SnmpInterfaceMetrics.GetLatestByDeviceID(dev.ID); len(latest) > 0 {
		upPorts, downPorts = 0, 0
		for _, iface := range latest {
			if iface.IfOperStatus == 1 {
				upPorts++
			} else {
				downPorts++
			}
		}
	}

	scoreStr := "未评估"
	if score >= 0 {
		scoreStr = fmt.Sprintf("%d", score)
	}
	summary := fmt.Sprintf("健康分 %s，SSH 巡检执行 %d 条命令（失败 %d），活动告警 %d 个 (耗时 %dms)",
		scoreStr, sig.sshExecuted, sig.sshFailed, sig.activeAlerts, durationMs)
	_ = repos.NetworkDevices.UpdateInspectionResult(dev.ID, summary)

	var sb strings.Builder
	sb.WriteString("### 🩺 NetMon 智能巡检健康诊断报告\n\n")
	if score < 0 {
		sb.WriteString(fmt.Sprintf("> ⚪ **巡检结论**：设备 `[ %s ]` 平台记录在线，但本次未采集到 SNMP/SSH 实测数据，健康分**未评估**。请检查设备凭证配置。\n\n", dev.Name))
	} else {
		sb.WriteString(fmt.Sprintf("> 🟢 **巡检结论**：设备 `[ %s ]` 实测巡检完毕，健康指数 **%d / 100 分**（评级：**%s**）。\n\n", dev.Name, score, scoreLabel(score)))
	}

	sb.WriteString("#### 1. 📌 设备资产档案\n\n")
	sb.WriteString("| 属性 | 值 | 属性 | 值 |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **设备名称** | **%s** | **管理 IP** | `%s` |\n", dev.Name, dev.IPAddress))
	sb.WriteString(fmt.Sprintf("| **厂商/品牌** | `%s` | **设备型号** | `%s` |\n", dev.Vendor, safeStr(dev.Model, "未登记")))
	sb.WriteString(fmt.Sprintf("| **设备角色** | `%s` | **安装位置** | `%s` |\n", safeStr(dev.Role, "未登记"), safeStr(dev.Location, "未登记")))
	sb.WriteString(fmt.Sprintf("| **巡检耗时** | `%d ms` | **完成时间** | `%s` |\n\n", durationMs, nowStr))

	sb.WriteString("#### 2. 📊 实测指标体检明细\n\n")
	sb.WriteString("| 体检维度 | 实测值 | 数据来源 |\n")
	sb.WriteString("| :--- | :--- | :--- |\n")

	// SSH 巡检
	if sig.sshOK {
		sb.WriteString(fmt.Sprintf("| **SSH 配置巡检** | 执行 %d 条 / 失败 %d 条 | 本次实测 |\n", sig.sshExecuted, sig.sshFailed))
	} else {
		sb.WriteString("| **SSH 配置巡检** | 未执行（无 SSH 凭证或连接失败） | — |\n")
	}

	// SNMP 系统信息
	if h := sig.health; h != nil {
		if h.SysUptime > 0 {
			sb.WriteString(fmt.Sprintf("| **系统运行时长** | %s | SNMP 实测 |\n", formatUptime(h.SysUptime)))
		}
		if h.CpuUsage != nil {
			sb.WriteString(fmt.Sprintf("| **CPU 利用率** | %.1f%% | SNMP 实测 |\n", *h.CpuUsage))
		} else {
			sb.WriteString("| **CPU 利用率** | 未采集（设备不支持该 OID） | — |\n")
		}
		if h.MemoryUsage != nil {
			sb.WriteString(fmt.Sprintf("| **内存使用率** | %.1f%% | SNMP 实测 |\n", *h.MemoryUsage))
		} else {
			sb.WriteString("| **内存使用率** | 未采集（设备不支持该 OID） | — |\n")
		}
		if h.Temperature != nil {
			sb.WriteString(fmt.Sprintf("| **设备温度** | %.1f ℃ | SNMP 实测 |\n", *h.Temperature))
		}
		if h.InterfaceHighUtil > 0 {
			sb.WriteString(fmt.Sprintf("| **高负载接口** | %d 个接口利用率 >80%% | SNMP 实测 |\n", h.InterfaceHighUtil))
		}
	} else {
		sb.WriteString("| **SNMP 健康检查** | 未采集（无 SNMP 凭证或设备无响应） | — |\n")
	}

	// 接口统计
	if upPorts >= 0 {
		src := "SNMP 实测"
		if sig.health == nil || sig.health.InterfaceCount == 0 {
			src = "最近一次采集存档"
		}
		sb.WriteString(fmt.Sprintf("| **物理接口状态** | UP: %d 口 / DOWN: %d 口 | %s |\n", upPorts, downPorts, src))
	} else {
		sb.WriteString("| **物理接口状态** | 未采集 | — |\n")
	}

	// 告警与备份（在库真实数据）
	sb.WriteString(fmt.Sprintf("| **活动告警** | %d 个未解决告警 | 告警中心在库数据 |\n", sig.activeAlerts))
	if sig.lastBackup != "" {
		sb.WriteString(fmt.Sprintf("| **配置备份** | 最近成功备份 `%s` | 备份记录在库数据 |\n", sig.lastBackup))
	} else {
		sb.WriteString("| **配置备份** | 无成功备份记录 | 备份记录在库数据 |\n")
	}
	sb.WriteString("\n")

	sb.WriteString("#### 3. 💡 运维建议\n\n")
	if sig.sshFailed > 0 {
		sb.WriteString(fmt.Sprintf("- SSH 巡检有 %d 条命令执行失败，建议在「巡检中心」查看明细并核对设备命令兼容性；\n", sig.sshFailed))
	}
	if sig.health != nil && sig.health.CpuUsage != nil && *sig.health.CpuUsage > 80 {
		sb.WriteString(fmt.Sprintf("- CPU 利用率 %.1f%% 超过 80%% 警戒线，建议排查异常进程或流量突增；\n", *sig.health.CpuUsage))
	}
	if sig.health != nil && sig.health.MemoryUsage != nil && *sig.health.MemoryUsage > 80 {
		sb.WriteString(fmt.Sprintf("- 内存使用率 %.1f%% 超过 80%% 警戒线，建议关注内存泄漏风险；\n", *sig.health.MemoryUsage))
	}
	if sig.activeAlerts > 0 {
		sb.WriteString(fmt.Sprintf("- 存在 %d 个未解决告警，请前往「告警中心」处理；\n", sig.activeAlerts))
	}
	if sig.lastBackup == "" {
		sb.WriteString("- 尚无成功配置备份，建议在「配置备份」中为该设备建立定期备份任务；\n")
	}
	if downPorts > 0 {
		sb.WriteString(fmt.Sprintf("- %d 个端口处于 DOWN 状态，长期闲置端口建议 `shutdown` 以提升安全合规评级；\n", downPorts))
	}
	if score >= 90 {
		sb.WriteString("- 各项实测指标正常，保持当前采集与备份策略即可。\n")
	}
	sb.WriteString("\n> 📝 **报告归档**：巡检结论已更新至设备档案，历史记录见 `network_inspection_history`。")
	return sb.String()
}

func formatUptime(centiseconds int64) string {
	d := time.Duration(centiseconds) * 10 * time.Millisecond
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%d 天 %d 小时 %d 分", days, hours, minutes)
	}
	return fmt.Sprintf("%d 小时 %d 分", hours, minutes)
}

// ───────────────────────────────────────────────
// 全网批量巡检
// ───────────────────────────────────────────────

// executeAllDevicesInspection 全网批量巡检：逐台执行真实 SNMP 健康检查（并发 8）
func executeAllDevicesInspection(ctx context.Context, inspType string) string {
	start := time.Now()
	devices, err := repos.NetworkDevices.List()
	if err != nil || len(devices) == 0 {
		return "⚠️ 当前系统中暂无已纳管的网络设备，无法发起全网巡检。请前往「网络设备」页面添加设备。"
	}

	type inspectItem struct {
		name      string
		ip        string
		vendor    string
		isOnline  bool
		score     int
		statusStr string
		summary   string
	}

	var results []inspectItem
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)

	for _, d := range devices {
		if d == nil {
			continue
		}
		wg.Add(1)
		go func(dev *repos.NetworkDevice) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			item := inspectItem{name: dev.Name, ip: dev.IPAddress, vendor: dev.Vendor, isOnline: isDeviceOnline(dev)}

			if !item.isOnline {
				item.score = 0
				item.statusStr = "🔴 离线"
				item.summary = "平台记录状态离线，未执行实测采集，建议排查物理线路或删除虚假设备"
			} else {
				// 批量巡检走 SNMP 健康检查（轻量）；SSH 深度巡检由单机报告执行
				sig := collectDeviceSignals(dev, inspType, false)
				item.score = scoreDevice(sig)
				switch {
				case item.score < 0:
					item.statusStr = "⚪ 未评估"
					item.summary = "在线但无 SNMP 采集数据，请检查 SNMP 凭证配置"
				case item.score >= 90:
					item.statusStr = "🟢 正常"
					item.summary = fmt.Sprintf("实测健康分 %d，活动告警 %d 个", item.score, sig.activeAlerts)
				default:
					item.statusStr = "🟡 关注"
					item.summary = fmt.Sprintf("实测健康分 %d，活动告警 %d 个，建议查看单机报告", item.score, sig.activeAlerts)
				}
			}

			_ = repos.NetworkDevices.UpdateInspectionResult(dev.ID, item.summary)

			mu.Lock()
			results = append(results, item)
			mu.Unlock()
		}(d)
	}

	wg.Wait()
	durationMs := time.Since(start).Milliseconds()

	onlineCount, offlineCount, totalScore, evaluated := 0, 0, 0, 0
	for _, r := range results {
		if r.isOnline {
			onlineCount++
		} else {
			offlineCount++
		}
		if r.score >= 0 {
			totalScore += r.score
			evaluated++
		}
	}
	avgScore := -1
	if evaluated > 0 {
		avgScore = totalScore / evaluated
	}

	var sb strings.Builder
	sb.WriteString("### 🌐 NetMon 全网基础设施批量巡检总览\n\n")
	sb.WriteString(fmt.Sprintf("> **巡检总耗时**：`%d ms` | **并发采集**：8 | **巡检类型**：`%s`（SNMP 实测）\n\n", durationMs, inspType))

	sb.WriteString("#### 1. 📊 全网健康统计\n\n")
	sb.WriteString("| 指标项 | 统计数值 | 说明 |\n")
	sb.WriteString("| :--- | :---: | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **巡检设备总数** | **%d 台** | 整体纳管规模 |\n", len(results)))
	sb.WriteString(fmt.Sprintf("| **在线设备** | 🟢 **%d 台** | 平台记录状态 |\n", onlineCount))
	sb.WriteString(fmt.Sprintf("| **离线设备** | 🔴 **%d 台** | 平台记录状态 |\n", offlineCount))
	if avgScore >= 0 {
		sb.WriteString(fmt.Sprintf("| **平均健康分** | **%d / 100 分**（%d 台有效评估） | 评级：%s |\n\n", avgScore, evaluated, scoreLabel(avgScore)))
	} else {
		sb.WriteString("| **平均健康分** | 无有效评估 | 所有设备均未采集到实测数据 |\n\n")
	}

	sb.WriteString("#### 2. 📋 各设备健康评分清单\n\n")
	sb.WriteString("| 序号 | 设备名称 | 管理 IP | 品牌 | 状态 | 健康评分 | 巡检结论摘要 |\n")
	sb.WriteString("| :---: | :--- | :--- | :--- | :---: | :---: | :--- |\n")
	for idx, r := range results {
		scoreStr := "未评估"
		if r.score >= 0 {
			scoreStr = fmt.Sprintf("%d 分", r.score)
		}
		sb.WriteString(fmt.Sprintf("| %d | **%s** | `%s` | %s | %s | **%s** | %s |\n",
			idx+1, r.name, r.ip, r.vendor, r.statusStr, scoreStr, r.summary))
	}
	sb.WriteString("\n")

	if offlineCount > 0 {
		sb.WriteString("#### 3. 🚨 离线设备处理建议\n\n")
		for _, r := range results {
			if !r.isOnline {
				sb.WriteString(fmt.Sprintf("- ⚠️ **`%s` (`%s`)**：平台记录持续离线。若为测试设备请前往「**网络设备 ➔ 设备管理**」删除；若为生产设备请排查上联链路与供电。\n", r.name, r.ip))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("> 💡 **深入操作**：输入「**巡检 设备名称**」可执行单机深度巡检（含 SSH 配置核查）。")
	return sb.String()
}

func safeStr(s *string, fallback string) string {
	if s != nil && *s != "" {
		return *s
	}
	return fallback
}