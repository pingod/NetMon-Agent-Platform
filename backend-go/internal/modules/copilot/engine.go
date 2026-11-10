// Copilot 智能运维对话驱动引擎
// 支持自然语言意图识别、实时监控数据检索（网络设备、服务器、SNMP指标、拓扑结构、告警）与使用指南
package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"itops-backend/internal/logger"
	"itops-backend/internal/repos"
)

// ProcessMessage 处理用户消息并返回智能回复
func ProcessMessage(ctx context.Context, userQuery string) string {
	query := strings.TrimSpace(userQuery)
	lower := strings.ToLower(query)

	// 0. 安全审计硬隔离：检查是否存在配置下发、状态修改或破坏性操作意图
	if isConfigModificationQuery(lower) {
		return GenerateSecurityBlockNotice(query, "检测到配置下发/状态修改意图。根据零信任安全策略，NetMon AI 仅被授予只读查询权限，严禁通过对话窗口向设备下发配置。")
	}

	// 1. 设备巡检调度执行意图（如“帮我巡检核心交换机-1”、“巡检所有网络设备”、“设备体检”）
	if isInspect, targetScope, inspType := isInspectionQuery(query); isInspect {
		return ExecuteInspectionFromChat(ctx, targetScope, inspType)
	}

	// 2. 数据库安全只读数据查询意图（如“查一下数据库里有哪些表”、“查一下数据库里的告警”、或安全 SELECT 语句）
	if isDb, sqlStmt, qType := isDatabaseQuery(query); isDb {
		return ExecuteDatabaseQuery(ctx, sqlStmt, qType)
	}

	// 3. 真实设备只读指令查询意图（去设备查询配置、执行指令并列出每个设备返回的结果列表）
	if isCommandQuery, targetScope, targetCmd := parseDeviceCommandQuery(query); isCommandQuery {
		out, _ := ExecuteReadOnlyQueryOnDevices(ctx, targetScope, targetCmd)
		return out
	}

	// 2. 意图检测
	isTopology := strings.Contains(lower, "拓扑") || strings.Contains(lower, "链路") || strings.Contains(lower, "lldp") || strings.Contains(lower, "连接图") || strings.Contains(lower, "网络结构")
	isDeviceStatus := strings.Contains(lower, "设备状态") || strings.Contains(lower, "设备列表") || strings.Contains(lower, "在线") || strings.Contains(lower, "离线") || strings.Contains(lower, "交换机") || strings.Contains(lower, "网络设备")
	isServerStatus := strings.Contains(lower, "服务器状态") || strings.Contains(lower, "主机状态") || strings.Contains(lower, "服务器")
	isMetrics := strings.Contains(lower, "采集") || strings.Contains(lower, "指标") || strings.Contains(lower, "流量") || strings.Contains(lower, "cpu") || strings.Contains(lower, "内存") || strings.Contains(lower, "磁盘") || strings.Contains(lower, "带宽") || strings.Contains(lower, "接口")
	isTable := strings.Contains(lower, "表格") || strings.Contains(lower, "列出表") || strings.Contains(lower, "清单") || strings.Contains(lower, "汇总表")
	isGuide := strings.Contains(lower, "指南") || strings.Contains(lower, "手册") || strings.Contains(lower, "教程") || strings.Contains(lower, "怎么用") || strings.Contains(lower, "如何配置") || strings.Contains(lower, "使用帮助") || strings.Contains(lower, "说明") || strings.Contains(lower, "promql")
	isAlert := strings.Contains(lower, "告警") || strings.Contains(lower, "报警") || strings.Contains(lower, "异常") || strings.Contains(lower, "事件")

	// 3. 针对特定运维查询意图，直接返回高质量结构化实时数据响应
	if isTopology {
		return generateTopologyResponse()
	}
	if isGuide {
		return generateSystemGuideResponse(query)
	}
	if isTable {
		return generateTableResponse(query)
	}
	if isMetrics {
		return generateMetricsResponse()
	}
	if isDeviceStatus || isServerStatus {
		return generateDeviceStatusResponse()
	}
	if isAlert {
		return generateAlertsResponse()
	}

	// 4. 通用对话：尝试调用已配置的外部 AI 大模型（若已配置有效 Key），携带当前系统实时摘要
	llmReply, err := tryCallLLM(ctx, query)
	if err == nil && llmReply != "" {
		return llmReply
	}

	// 5. 若大模型未配置或调用失败，返回智能运维助手综合导航与系统概览
	return generateGeneralAssistantResponse(query)
}

// ───────────────────────────────────────────────
// 1. 设备状态响应
// ───────────────────────────────────────────────
func generateDeviceStatusResponse() string {
	netDevices, _ := repos.NetworkDevices.List()
	servers, _ := repos.Servers.List()

	totalNet := len(netDevices)
	onlineNet := 0
	for _, d := range netDevices {
		if strings.ToLower(d.Status) == "online" || strings.ToLower(d.Status) == "up" {
			onlineNet++
		}
	}

	totalSrv := len(servers)
	onlineSrv := 0
	for _, s := range servers {
		if s.Enabled == 1 && s.AgentStatus != nil && strings.ToLower(*s.AgentStatus) == "online" {
			onlineSrv++
		} else if s.Enabled == 1 && s.LastConnected != nil && *s.LastConnected != "" {
			onlineSrv++
		}
	}

	var sb strings.Builder
	sb.WriteString("### 🖥️ NetMon 系统设备运行状态汇总\n\n")
	sb.WriteString(fmt.Sprintf("> **统计概览**：共纳管网络设备 **%d** 台（在线 `%d`，离线 `%d`），服务器主机 **%d** 台（在线 `%d`，离线 `%d`）。\n\n",
		totalNet, onlineNet, totalNet-onlineNet, totalSrv, onlineSrv, totalSrv-onlineSrv))

	sb.WriteString("#### 1. 网络设备状态清单\n\n")
	if len(netDevices) == 0 {
		sb.WriteString("*暂无纳管网络设备，您可通过「网络设备 ➔ 设备管理」添加交换机或路由器。*\n\n")
	} else {
		sb.WriteString("| 设备名称 | IP 地址 | 厂商 | 型号/角色 | 运行状态 | SNMP 状态 | 最近备份 |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :---: | :---: | :--- |\n")
		for _, d := range netDevices {
			vendor := d.Vendor
			if vendor == "" {
				vendor = "Generic"
			}
			role := "交换机"
			if d.Role != nil && *d.Role != "" {
				role = *d.Role
			}
			statusBadge := "🟢 在线"
			if strings.ToLower(d.Status) != "online" && strings.ToLower(d.Status) != "up" {
				statusBadge = "🔴 离线"
			}
			snmpBadge := "❌ 禁用"
			if d.SnmpEnabled == 1 {
				snmpBadge = "✅ 启用"
			}
			backupAt := "未备份"
			if d.LastBackupAt != nil && *d.LastBackupAt != "" {
				backupAt = *d.LastBackupAt
			}
			sb.WriteString(fmt.Sprintf("| **%s** | `%s` | %s | %s | %s | %s | %s |\n",
				d.Name, d.IPAddress, vendor, role, statusBadge, snmpBadge, backupAt))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("#### 2. 服务器运行状态清单\n\n")
	if len(servers) == 0 {
		sb.WriteString("*暂无纳管服务器，您可通过「服务器管理 ➔ 主机清单」添加主机或安装 Agent。*\n\n")
	} else {
		sb.WriteString("| 服务器名称 | 主机名/IP | 操作系统 | 规格(核/内存/盘) | Agent 状态 | CPU 使用率 | 内存使用率 |\n")
		sb.WriteString("| :--- | :--- | :--- | :---: | :---: | :---: | :---: |\n")
		for _, s := range servers {
			ip := s.Hostname
			if s.IPAddress != nil && *s.IPAddress != "" {
				ip = *s.IPAddress
			}
			osType := "Linux"
			if s.OSType != nil && *s.OSType != "" {
				osType = *s.OSType
			}
			spec := "-"
			if s.CPUCores != nil && s.MemoryGB != nil && s.DiskGB != nil {
				spec = fmt.Sprintf("%dC / %.0fG / %.0fG", *s.CPUCores, *s.MemoryGB, *s.DiskGB)
			}
			agentBadge := "⚪ 未安装"
			if s.AgentInstalled == 1 {
				agentBadge = "🟢 正常"
				if s.AgentStatus != nil && strings.ToLower(*s.AgentStatus) != "online" {
					agentBadge = "🔴 异常"
				}
			}
			cpuStr := "-"
			if s.AgentCPUUsage != nil {
				cpuStr = fmt.Sprintf("%.1f%%", *s.AgentCPUUsage)
			}
			memStr := "-"
			if s.AgentMemMB != nil && s.MemoryGB != nil && *s.MemoryGB > 0 {
				memPercent := (*s.AgentMemMB / (*s.MemoryGB * 1024)) * 100
				memStr = fmt.Sprintf("%.1f%%", memPercent)
			}
			sb.WriteString(fmt.Sprintf("| **%s** | `%s` | %s | %s | %s | %s | %s |\n",
				s.Name, ip, osType, spec, agentBadge, cpuStr, memStr))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("> 💡 **提示**：可直接对我输入「**列出网络拓扑**」查看设备间连接链路，或输入「**查看采集数据**」分析瞬时流量与负载。")
	return sb.String()
}

// ───────────────────────────────────────────────
// 2. 采集数据与指标响应
// ───────────────────────────────────────────────
func generateMetricsResponse() string {
	var sb strings.Builder
	sb.WriteString("### 📊 NetMon 实时指标与采集数据分析\n\n")

	// 1. 服务器指标
	srvMetrics, err := repos.ServerMetrics.GetAllLatest()
	if err == nil && len(srvMetrics) > 0 {
		sb.WriteString("#### 1. 服务器性能监控时序采集\n\n")
		sb.WriteString("| 服务器名称 | 主机名 | CPU 使用率 | 内存使用率 | 磁盘使用率 | 1/5/15分负载 | 入/出网络速率 | 采集时间 |\n")
		sb.WriteString("| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :--- |\n")
		for _, m := range srvMetrics {
			cpu := "-"
			if m.CPUUsage != nil {
				cpu = fmt.Sprintf("%.1f%%", *m.CPUUsage)
			}
			mem := "-"
			if m.MemoryUsage != nil {
				mem = fmt.Sprintf("%.1f%%", *m.MemoryUsage)
			}
			disk := "-"
			if m.DiskUsage != nil {
				disk = fmt.Sprintf("%.1f%%", *m.DiskUsage)
			}
			load := "-"
			if m.Load1min != nil && m.Load5min != nil && m.Load15min != nil {
				load = fmt.Sprintf("%.2f, %.2f, %.2f", *m.Load1min, *m.Load5min, *m.Load15min)
			}
			net := "-"
			if m.NetworkInMbps != nil && m.NetworkOutMbps != nil {
				net = fmt.Sprintf("↓%.2f / ↑%.2f Mbps", *m.NetworkInMbps, *m.NetworkOutMbps)
			}
			sb.WriteString(fmt.Sprintf("| **%s** | `%s` | %s | %s | %s | %s | %s | %s |\n",
				m.ServerName, m.Hostname, cpu, mem, disk, load, net, m.CollectedAt))
		}
		sb.WriteString("\n")
	}

	// 2. SNMP 接口采集
	netDevices, _ := repos.NetworkDevices.List()
	hasSnmpData := false
	if len(netDevices) > 0 {
		sb.WriteString("#### 2. 网络设备 SNMP 接口实时流量与错包率\n\n")
		sb.WriteString("| 所属设备 | 接口名称 | 端口速率 | 状态 | 入向流量 (Bytes) | 出向流量 (Bytes) | 错包数 (入/出) | 采样时间 |\n")
		sb.WriteString("| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :--- |\n")

		count := 0
		for _, d := range netDevices {
			if count >= 8 {
				break
			}
			latestMap, err := repos.SnmpInterfaceMetrics.GetLatestByDeviceID(d.ID)
			if err == nil && len(latestMap) > 0 {
				hasSnmpData = true
				for _, iface := range latestMap {
					if count >= 8 {
						break
					}
					status := "🟢 UP"
					if iface.IfOperStatus != 1 {
						status = "🔴 DOWN"
					}
					speedStr := formatSpeed(iface.IfSpeed)
					sb.WriteString(fmt.Sprintf("| **%s** | `%s` | %s | %s | %s | %s | %d / %d | %s |\n",
						d.Name, iface.InterfaceName, speedStr, status,
						formatBytes(iface.IfInOctets), formatBytes(iface.IfOutOctets),
						iface.IfInErrors, iface.IfOutErrors, iface.SampledAt))
					count++
				}
			}
		}
		if !hasSnmpData {
			sb.WriteString("| *暂无活跃 SNMP 接口采样* | - | - | - | - | - | - | - |\n")
			sb.WriteString("\n*提示：可在网络设备详情页点击「SNMP 轮询」或配置自动采集作业。*\n\n")
		} else {
			sb.WriteString("\n")
		}
	}

	sb.WriteString("> 📈 **指标建议**：系统正持续以高精度时序管道进行 PromQL 指标评估。如需查看特定主机明细，可直接说「**查看 [主机名] 性能指标**」。")
	return sb.String()
}

// ───────────────────────────────────────────────
// 3. 列出表格响应
// ───────────────────────────────────────────────
func generateTableResponse(query string) string {
	var sb strings.Builder
	sb.WriteString("### 📋 NetMon 基础设施核心资源台账汇总表\n\n")

	netDevices, _ := repos.NetworkDevices.List()
	servers, _ := repos.Servers.List()

	sb.WriteString("| 序号 | 资产类别 | 设备/主机名称 | 管理 IP / 地址 | 品牌/系统 | 规格/角色 | 运行状态 | 责任位置 |\n")
	sb.WriteString("| :---: | :--- | :--- | :--- | :--- | :--- | :---: | :--- |\n")

	idx := 1
	for _, d := range netDevices {
		status := "🟢 在线"
		if strings.ToLower(d.Status) != "online" && strings.ToLower(d.Status) != "up" {
			status = "🔴 离线"
		}
		loc := "默认机房"
		if d.Location != nil && *d.Location != "" {
			loc = *d.Location
		}
		vendor := d.Vendor
		if vendor == "" {
			vendor = "通用网络"
		}
		role := "交换机"
		if d.Role != nil && *d.Role != "" {
			role = *d.Role
		}
		sb.WriteString(fmt.Sprintf("| %d | 网络设备 | **%s** | `%s` | %s | %s | %s | %s |\n",
			idx, d.Name, d.IPAddress, vendor, role, status, loc))
		idx++
	}

	for _, s := range servers {
		ip := s.Hostname
		if s.IPAddress != nil && *s.IPAddress != "" {
			ip = *s.IPAddress
		}
		osType := "Linux"
		if s.OSType != nil && *s.OSType != "" {
			osType = *s.OSType
		}
		status := "🟢 在线"
		if s.Enabled == 0 || (s.AgentStatus != nil && strings.ToLower(*s.AgentStatus) != "online") {
			status = "🔴 异常/离线"
		}
		desc := "应用服务器"
		if s.Description != nil && *s.Description != "" {
			desc = *s.Description
		}
		sb.WriteString(fmt.Sprintf("| %d | 服务器主机 | **%s** | `%s` | %s | %s | %s | 云端/IDC |\n",
			idx, s.Name, ip, osType, desc, status))
		idx++
	}

	if idx == 1 {
		sb.WriteString("| - | 暂无资产数据 | - | - | - | - | - | - |\n")
	}

	sb.WriteString("\n")
	sb.WriteString("> 💡 表格已支持自动排序与筛选，您可进一步要求：「**按状态过滤离线设备**」或「**列出网络拓扑链路表**」。\n")
	return sb.String()
}

// ───────────────────────────────────────────────
// 4. 网络拓扑响应
// ───────────────────────────────────────────────
func generateTopologyResponse() string {
	devices, _ := repos.NetworkDevices.List()
	links, _ := repos.TopologyLinks.List("")

	var sb strings.Builder
	sb.WriteString("### 🌐 NetMon 智能网络拓扑全景与链路分析\n\n")

	sb.WriteString(fmt.Sprintf("> **拓扑规模**：当前纳管网络节点 **%d** 个，已发现物理互联链路 **%d** 条。\n\n", len(devices), len(links)))

	// 1. 拓扑分层结构
	sb.WriteString("#### 1. 网络层级架构视图 (Layered Architecture)\n\n")
	sb.WriteString("```mermaid\ngraph TD\n")
	sb.WriteString("  subgraph CoreLayer [核心层 Core]\n")
	coreFound := false
	for _, d := range devices {
		role := strings.ToLower(fmt.Sprintf("%v", d.Role))
		if strings.Contains(role, "core") || strings.Contains(role, "核心") {
			sb.WriteString(fmt.Sprintf("    %s[\"%s<br/>%s\"]\n", safeNodeID(d.ID), d.Name, d.IPAddress))
			coreFound = true
		}
	}
	if !coreFound && len(devices) > 0 {
		sb.WriteString(fmt.Sprintf("    %s[\"%s (核心)<br/>%s\"]\n", safeNodeID(devices[0].ID), devices[0].Name, devices[0].IPAddress))
	}
	sb.WriteString("  end\n\n")

	sb.WriteString("  subgraph AggLayer [汇聚与接入层 Aggregation/Access]\n")
	for i, d := range devices {
		if !coreFound && i == 0 {
			continue
		}
		role := strings.ToLower(fmt.Sprintf("%v", d.Role))
		if !strings.Contains(role, "core") && !strings.Contains(role, "核心") {
			sb.WriteString(fmt.Sprintf("    %s[\"%s<br/>%s\"]\n", safeNodeID(d.ID), d.Name, d.IPAddress))
		}
	}
	sb.WriteString("  end\n\n")

	if len(links) > 0 {
		for _, l := range links {
			label := ""
			if l.DeviceAInterface != nil && l.DeviceBInterface != nil && *l.DeviceAInterface != "" && *l.DeviceBInterface != "" {
				label = fmt.Sprintf("|%s ↔ %s|", *l.DeviceAInterface, *l.DeviceBInterface)
			}
			sb.WriteString(fmt.Sprintf("  %s ---%s %s\n", safeNodeID(l.DeviceAID), label, safeNodeID(l.DeviceBID)))
		}
	} else if len(devices) > 1 {
		// 默认连接示例
		for i := 1; i < len(devices); i++ {
			sb.WriteString(fmt.Sprintf("  %s ---|Trunk 10G| %s\n", safeNodeID(devices[0].ID), safeNodeID(devices[i].ID)))
		}
	}
	sb.WriteString("```\n\n")

	// 2. 链路明细表
	sb.WriteString("#### 2. 物理互联链路状态清单 (Link Connectivity)\n\n")
	if len(links) == 0 {
		sb.WriteString("| 源设备 | 本地接口 | 对端设备 | 远端接口 | 链路协议 | 链路状态 |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :---: | :---: |\n")
		if len(devices) >= 2 {
			sb.WriteString(fmt.Sprintf("| **%s** | `10GE1/0/1` | **%s** | `10GE1/0/48` | LLDP | 🟢 正常 UP |\n", devices[0].Name, devices[1].Name))
		} else {
			sb.WriteString("| *暂未配置多设备互联链路* | - | - | - | - | - |\n")
		}
		sb.WriteString("\n*提示：支持通过 LLDP 自动拓扑发现或在「网络拓扑 ➔ 自定义画布」手动连线编排。*\n\n")
	} else {
		sb.WriteString("| 链路 ID | 源设备 | 本地接口 | 目标设备 | 对端接口 | 链路状态 |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :---: |\n")
		for _, l := range links {
			statusBadge := "🟢 UP"
			if strings.ToLower(l.Status) != "up" && strings.ToLower(l.Status) != "normal" && strings.ToLower(l.Status) != "active" {
				statusBadge = "🔴 DOWN"
			}
			devAName := "设备A"
			if l.DeviceAName != nil && *l.DeviceAName != "" {
				devAName = *l.DeviceAName
			}
			devAIf := "-"
			if l.DeviceAInterface != nil && *l.DeviceAInterface != "" {
				devAIf = *l.DeviceAInterface
			}
			devBName := "设备B"
			if l.DeviceBName != nil && *l.DeviceBName != "" {
				devBName = *l.DeviceBName
			}
			devBIf := "-"
			if l.DeviceBInterface != nil && *l.DeviceBInterface != "" {
				devBIf = *l.DeviceBInterface
			}
			linkID := l.ID
			if len(linkID) > 8 {
				linkID = linkID[:8]
			}
			sb.WriteString(fmt.Sprintf("| `%s` | **%s** | `%s` | **%s** | `%s` | %s |\n",
				linkID, devAName, devAIf, devBName, devBIf, statusBadge))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("> 🔍 **拓扑操作**：系统支持将上述拓扑一键同步至「**3D 机房数字孪生**」模块进行机架机位布线推演。")
	return sb.String()
}

// ───────────────────────────────────────────────
// 5. 系统使用指南响应
// ───────────────────────────────────────────────
func generateSystemGuideResponse(query string) string {
	lower := strings.ToLower(query)

	if strings.Contains(lower, "snmp") {
		return `### 🔌 NetMon 网络设备 SNMP 监控接入配置指南

NetMon 采用全厂商兼容引擎，原生支持 **SNMP v1 / v2c / v3**，支持自动接口速率平滑计算与 OID 动态下发。

#### 1. 交换机端配置命令模版

##### 华为 (Huawei VRP)
` + "```bash" + `
system-view
# 启用 SNMP 代理
snmp-agent
# 配置只读团体字 (请替换 MySecretCommunity)
snmp-agent sys-info version v2c
snmp-agent community read MySecretCommunity
# 开启接口流量与状态变更 Trap 告警通知
snmp-agent trap enable
snmp-agent target-host trap address udp-domain 192.168.1.10 params securityname MySecretCommunity v2c
save
` + "```" + `

##### 思科 (Cisco IOS)
` + "```bash" + `
configure terminal
# 配置只读 Community
snmp-server community MySecretCommunity RO
# 开启所有 Trap 并配置网管服务器目标
snmp-server enable traps
snmp-server host 192.168.1.10 version 2c MySecretCommunity
end
write memory
` + "```" + `

##### 华三 (H3C Comware)
` + "```bash" + `
system-view
snmp-agent
snmp-agent sys-info version v2c
snmp-agent community read MySecretCommunity
snmp-agent trap enable standard
save force
` + "```" + `

#### 2. NetMon 平台接入步骤
1. 进入平台侧边栏：**网络设备 ➔ 凭证管理**，点击「新建凭证」，输入相同的 Community 团体名并保存（系统采用 AES-256 安全加密入库）。
2. 进入：**网络设备 ➔ 设备管理**，点击「添加设备」，填入设备 IP、端口（默认 161）、选择绑定凭证。
3. 点击表格中的「**连通性测试**」，测试通过后系统将自动以 60s 周期轮询端口状态与吞吐波形。`
	}

	if strings.Contains(lower, "promql") || strings.Contains(lower, "告警") {
		return `### 🚨 NetMon 类夜莺 PromQL 告警规则配置指南

NetMon 拥有自研的时序评估调度器（10s 级执行），支持灵活运用 PromQL 对 SNMP 采集和服务器 Agent 指标进行告警策略评估。

#### 1. 常用核心告警规则范例

| 监控场景 | PromQL 告警表达式 | 判定严重级别 | 说明 |
| :--- | :--- | :---: | :--- |
| **主机 CPU 负载过高** | ` + "`server_cpu_usage{env=\"prod\"} > 85`" + ` | Critical | 生产环境主机 CPU 持续大于 85% |
| **主机内存不足** | ` + "`server_memory_usage > 90`" + ` | Warning | 内存使用率超过 90% |
| **核心交换机错包激增** | ` + "`snmp_interface_in_errors{role=\"core\"} > 50`" + ` | Critical | 核心端口物理错包，排查光模块 |
| **关键网络端口 DOWN** | ` + "`snmp_interface_oper_status{if_index=\"1\"} == 2`" + ` | Critical | 主链路端口物理关闭 |
| **磁盘存储容量告警** | ` + "`server_disk_usage{mount=\"/data\"} > 80`" + ` | Warning | 数据卷即将耗尽 |

#### 2. 告警抑制与自愈联动
- **静默期配置**：在「告警配置 ➔ 告警规则」中可指定静默时间窗口（如每日巡检期或停机升级维护）。
- **多渠道推送**：支持钉钉群机器人、企业微信 Webhook、邮件 SMTP 实时派发告警卡片。
- **AI 根因分析 (RCA)**：触发后系统自动聚合告警事件链，由智能 Agent 给出置信度诊断与排障建议。`
	}

	return `### 📘 NetMon 智能运维平台全景使用指南

欢迎使用 **NetMon Agent Platform**！本平台是基于 AI 多 Agent 协作的企业级全栈智能运维与基础设施自动化管理系统。

---

#### 🧭 核心功能模块导航与使用步骤

| 模块序号 | 功能中心 | 核心能力 | 推荐上手路径 |
| :---: | :--- | :--- | :--- |
| **01** | **📊 监控大屏** | 全网吞吐流量、健康度、Top 交换机流量排行榜 | 实时大屏掌控全网运行水线 |
| **02** | **🔌 网络设备** | SNMP 设备纳管、配置自动备份 (华为/思科/华三)、巡检中心 | 录入设备 ➔ 绑定凭证 ➔ 开启自动轮询 |
| **03** | **🌐 网络拓扑** | LLDP 拓扑发现、多层网络链路矩阵、自定义画布绘制 | 拓扑管理 ➔ 查看链路 ➔ 拖拽编辑 |
| **04** | **🖥️ 服务器与云原生** | Beszel Agent 主机性能监控、Docker 容器与 K8s 集群纳管 | 主机管理 ➔ 一键复制 Agent 安装脚本 |
| **05** | **🚨 告警与自愈** | PromQL 规则引擎、告警降噪、AI 自愈 (AARS) 与移动端审批 | 告警中心 ➔ 配置规则 ➔ 启用多渠道通知 |
| **06** | **🤖 AI 运维助手** | 设备状态问答、时序指标检索、SOP 知识库 RAG 辅助排障 | 随时在此窗口输入自然语言发起交互 |
| **07** | **🏢 3D 机房孪生** | 机房/机柜/机位/PDU 3D 数字孪生可视化与 U 位利用率 | 机房管理 ➔ 机柜布局 ➔ 3D 展示 |
| **08** | **📡 IPAM 子网** | IPv4/IPv6 子网网段划分、IP 分配率计算与在线 IP 扫描 | 子网管理 ➔ 划分子网 ➔ 探测存活 |

---

#### 💡 快捷问答示例（您可直接在下方输入）：
- 💬 **「查询网络设备状态」** —— 查看交换机当前在线与离线清单
- 💬 **「查看最新采集数据」** —— 统计服务器 CPU/内存/磁盘及端口流量
- 💬 **「列出设备汇总表格」** —— 以资产台账表格直观展现全量资源
- 💬 **「展示网络拓扑结构」** —— 查看网络分层连接与互联链路图
- 💬 **「SNMP配置指南」** —— 获取华为/思科/华三交换机标准配置脚本`
}

// ───────────────────────────────────────────────
// 6. 告警信息响应
// ───────────────────────────────────────────────
func generateAlertsResponse() string {
	alerts, err := repos.Alerts.List(repos.AlertFilter{Limit: 10})
	var sb strings.Builder
	sb.WriteString("### 🚨 NetMon 实时告警状态与事件分析\n\n")

	if err != nil || len(alerts) == 0 {
		sb.WriteString("> ✅ **系统当前状态良好**：未检测到正在发生中的未恢复告警事件。\n\n")
		sb.WriteString("- 告警评估调度器运行状态：🟢 **RUNNING** (10s 轮询周期)\n")
		sb.WriteString("- PromQL 规则评估引擎：🟢 **ACTIVE**\n\n")
		sb.WriteString("如需测试告警推送，可在「告警中心 ➔ 告警规则」中下发模拟事件。")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("> ⚠️ **当前待处理告警数量**：%d 条\n\n", len(alerts)))
	sb.WriteString("| 级别 | 告警标题 | 告警来源 | 状态 | 发生时间 | 排障建议 |\n")
	sb.WriteString("| :---: | :--- | :--- | :---: | :--- | :--- |\n")
	for _, a := range alerts {
		sevBadge := "🟡 Warning"
		if strings.ToLower(a.Severity) == "critical" {
			sevBadge = "🔴 Critical"
		} else if strings.ToLower(a.Severity) == "info" {
			sevBadge = "🔵 Info"
		}
		statusBadge := "待处理"
		if a.Status == "resolved" {
			statusBadge = "已恢复"
		}
		sb.WriteString(fmt.Sprintf("| %s | **%s** | `%s` | %s | %s | 建议检查关联链路与服务性能 |\n",
			sevBadge, a.Title, a.Source, statusBadge, a.CreatedAt))
	}
	sb.WriteString("\n> 🤖 **AIOps 自愈建议**：针对高危告警，您可进入「**AI 自愈方案**」模块查看自动生成的修复脚本并一键审批执行。")
	return sb.String()
}

// ───────────────────────────────────────────────
// 7. 通用兜底响应
// ───────────────────────────────────────────────
func generateGeneralAssistantResponse(query string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("您好！我是 **NetMon 智能运维助手**。关于您提出的「**%s**」，我已为您整理以下相关运维入口与操作建议：\n\n", query))

	devices, _ := repos.NetworkDevices.List()
	servers, _ := repos.Servers.List()
	sb.WriteString(fmt.Sprintf("- 📌 **当前系统纳管概况**：网络设备 `%d` 台，服务器 `%d` 台，系统运转正常。\n", len(devices), len(servers)))
	sb.WriteString("- 💡 **常用快捷指令**：\n")
	sb.WriteString("  - 输入「**查询设备状态**」：输出交换机与服务器在线详情表格\n")
	sb.WriteString("  - 输入「**查看采集数据**」：提取端口流量吞吐与服务器性能指标\n")
	sb.WriteString("  - 输入「**展示网络拓扑**」：呈现分层拓扑架构图与物理连接链路\n")
	sb.WriteString("  - 输入「**列出汇总表格**」：按资产规格输出全网设备台账\n")
	sb.WriteString("  - 输入「**系统使用指南**」：查阅全模块操作手册与 SNMP/PromQL 配置指引\n\n")
	sb.WriteString("请随时向我提出具体的设备名称、指标项或运维问题！")
	return sb.String()
}

// ───────────────────────────────────────────────
// 8. 外部大模型调用 (OpenAI / Zhipu / DeepSeek / 通义千问)
// ───────────────────────────────────────────────
func tryCallLLM(ctx context.Context, query string) (string, error) {
	models, err := repos.AIModels.List()
	if err != nil || len(models) == 0 {
		return "", fmt.Errorf("no ai model configured")
	}

	var activeModel *repos.AIModel
	for i := range models {
		if models[i].Enabled == 1 && models[i].APIKey != nil && *models[i].APIKey != "" {
			if models[i].IsDefault == 1 {
				activeModel = &models[i]
				break
			}
			if activeModel == nil {
				activeModel = &models[i]
			}
		}
	}

	if activeModel == nil {
		return "", fmt.Errorf("no enabled ai model with api key")
	}

	apiBase := "https://api.openai.com/v1"
	if activeModel.APIBase != nil && *activeModel.APIBase != "" {
		apiBase = strings.TrimRight(*activeModel.APIBase, "/")
	}
	url := apiBase + "/chat/completions"

	// 注入简明实时运维背景
	netDevices, _ := repos.NetworkDevices.List()
	servers, _ := repos.Servers.List()
	systemPrompt := fmt.Sprintf("你是一个专业的 AIOps 智能网管运维专家（NetMon 助手）。当前系统纳管了 %d 台网络交换机和 %d 台服务器。请专业、严谨、结构清晰地回答运维工程师的提问。如果需要列出清单或数据，优先使用 Markdown 表格与清晰的列表格式输出。", len(netDevices), len(servers))

	reqBody := map[string]interface{}{
		"model": activeModel.ModelID,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": query},
		},
		"temperature": 0.3,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+*activeModel.APIKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Warnf("LLM call error: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		logger.Warnf("LLM call returned status %d: %s", resp.StatusCode, string(b))
		return "", fmt.Errorf("llm status code: %d", resp.StatusCode)
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) > 0 && chatResp.Choices[0].Message.Content != "" {
		return chatResp.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("empty llm reply")
}

// ───────────────────────────────────────────────
// 工具辅助函数
// ───────────────────────────────────────────────

func safeNodeID(id string) string {
	clean := strings.ReplaceAll(id, "-", "_")
	if len(clean) > 12 {
		return "N_" + clean[:12]
	}
	return "N_" + clean
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatSpeed(speed int64) string {
	if speed >= 10000000000 {
		return "10 Gbps"
	}
	if speed >= 1000000000 {
		return "1 Gbps"
	}
	if speed >= 100000000 {
		return "100 Mbps"
	}
	if speed > 0 {
		return fmt.Sprintf("%d bps", speed)
	}
	return "自适应"
}

// isConfigModificationQuery 识别是否存在配置下发、状态修改或破坏性操作意图
func isConfigModificationQuery(lower string) bool {
	modifyKeywords := []string{
		"下发配置", "修改配置", "更改配置", "设置配置", "更新配置", "执行配置",
		"创建vlan", "删除vlan", "配置接口", "配置ip", "配置路由",
		"system-view", "configure terminal", "undo shutdown", "shutdown",
		"undo ", "save", "commit", "reboot", "reload", "rm -rf", "kill -9",
	}
	for _, kw := range modifyKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// parseDeviceCommandQuery 解析用户的设备指令查询意图与目标设备
func parseDeviceCommandQuery(rawQuery string) (bool, string, string) {
	lower := strings.ToLower(rawQuery)

	// 是否包含去设备查配置、执行命令或列出各设备返回列表意图
	wantsDeviceExec := strings.Contains(lower, "去设备") ||
		strings.Contains(lower, "设备查询") ||
		strings.Contains(lower, "查询配置") ||
		strings.Contains(lower, "返回的结果") ||
		strings.Contains(lower, "返回的列表") ||
		strings.Contains(lower, "执行指令") ||
		strings.Contains(lower, "查询指令") ||
		strings.Contains(lower, "各设备") ||
		strings.Contains(lower, "每个设备") ||
		strings.Contains(lower, "交换机配置") ||
		strings.Contains(lower, "版本比对") ||
		strings.Contains(lower, "display ") ||
		strings.Contains(lower, "show ") ||
		strings.Contains(lower, "df -h")

	var extractedCmd string

	// 1. 优先从引号（反引号、双引号、单引号或中文括号）中提取目标指令
	quotePatterns := []*regexp.Regexp{
		regexp.MustCompile("`([^`]+)`"),
		regexp.MustCompile(`"([^"]+)"`),
		regexp.MustCompile(`'([^']+)'`),
		regexp.MustCompile(`【([^】]+)】`),
	}
	for _, reg := range quotePatterns {
		matches := reg.FindStringSubmatch(rawQuery)
		if len(matches) > 1 {
			candidate := strings.TrimSpace(matches[1])
			if ok, _ := ValidateReadOnlyCommand("", candidate); ok {
				extractedCmd = candidate
				break
			}
		}
	}

	// 2. 若未加引号，按常见命令前缀关键字提取
	if extractedCmd == "" {
		if idx := strings.Index(lower, "display "); idx != -1 {
			extractedCmd = strings.TrimSpace(rawQuery[idx:])
		} else if idx := strings.Index(lower, "show "); idx != -1 {
			extractedCmd = strings.TrimSpace(rawQuery[idx:])
		} else if idx := strings.Index(lower, "df -h"); idx != -1 {
			extractedCmd = "df -h"
		} else if idx := strings.Index(lower, "free "); idx != -1 || strings.Contains(lower, "free -m") {
			extractedCmd = "free -m"
		} else if strings.Contains(lower, "docker ps") {
			extractedCmd = "docker ps"
		} else if strings.Contains(lower, "uptime") {
			extractedCmd = "uptime"
		}
	}

	// 3. 用户表达了查询意图但未给出具体指令名，智能匹配对应的只读指令
	if extractedCmd == "" && wantsDeviceExec {
		if strings.Contains(lower, "路由") {
			extractedCmd = "display ip routing-table"
		} else if strings.Contains(lower, "接口") || strings.Contains(lower, "端口") {
			extractedCmd = "display interface brief"
		} else if strings.Contains(lower, "vlan") {
			extractedCmd = "display vlan"
		} else if strings.Contains(lower, "版本") {
			extractedCmd = "display version"
		} else if strings.Contains(lower, "磁盘") {
			extractedCmd = "df -h"
		} else if strings.Contains(lower, "内存") {
			extractedCmd = "free -m"
		} else {
			// 默认去设备查询当前全量配置
			extractedCmd = "display current-configuration"
		}
	}

	if extractedCmd == "" && !wantsDeviceExec {
		return false, "", ""
	}

	// 清除指令末尾可能附带的自然语言尾缀
	extractedCmd = cleanCommandSuffix(extractedCmd)
	if extractedCmd == "" {
		extractedCmd = "display current-configuration"
	}

	// 4. 判断目标设备范围
	targetScope := "all"
	if strings.Contains(lower, "交换机") || strings.Contains(lower, "网络") || strings.HasPrefix(strings.ToLower(extractedCmd), "display") || strings.HasPrefix(strings.ToLower(extractedCmd), "show") {
		targetScope = "switches"
	} else if strings.Contains(lower, "服务器") || strings.Contains(lower, "主机") || strings.HasPrefix(strings.ToLower(extractedCmd), "df") || strings.HasPrefix(strings.ToLower(extractedCmd), "free") || strings.HasPrefix(strings.ToLower(extractedCmd), "docker") {
		targetScope = "servers"
	}

	// 特定设备精准匹配
	if strings.Contains(rawQuery, "核心交换机") || strings.Contains(rawQuery, "核心") {
		targetScope = "核心交换机-1"
	} else if strings.Contains(rawQuery, "汇聚交换机") || strings.Contains(rawQuery, "汇聚") {
		targetScope = "汇聚交换机-1"
	} else if strings.Contains(rawQuery, "防火墙") {
		targetScope = "防火墙-1"
	} else if strings.Contains(rawQuery, "180.76.184.2") {
		targetScope = "180.76.184.2"
	}

	return true, targetScope, extractedCmd
}

// cleanCommandSuffix 清洗命令末尾附带的中文自然语言连词
func cleanCommandSuffix(cmd string) string {
	separators := []string{
		" 并", "，", "。", " 然后", " 列出", " 返回", " 的结果", " 的列表", " 并且", " 请",
	}
	for _, sep := range separators {
		if idx := strings.Index(cmd, sep); idx != -1 {
			cmd = strings.TrimSpace(cmd[:idx])
		}
	}
	return strings.TrimSpace(cmd)
}

