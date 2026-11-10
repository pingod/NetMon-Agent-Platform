// 巡检模版管理 — 预置模版数据与种子函数
package inspection

import (
	"encoding/json"
	"fmt"
	"time"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
)

// PresetTemplate 预置模版定义（用于种子数据）
type PresetTemplate struct {
	Name        string          `json:"name"`
	Vendor      string          `json:"vendor"`
	Category    string          `json:"category"`   // network | server
	DeviceType  string          `json:"deviceType"` // switch/router/firewall/...
	OSType      string          `json:"osType"`     // linux/windows/...
	Description string          `json:"description"`
	Commands    []PresetCommand `json:"commands"`
	Variables   []string        `json:"variables"`
	Tags        []string        `json:"tags"`
}

// PresetCommand 单条巡检指令
type PresetCommand struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Command     string   `json:"command"`
	Fallbacks   []string `json:"fallbacks"`
	Description string   `json:"description"`
	Group       string   `json:"group"` // 分组：硬件层/系统层/资源层/安全层/应用层
}

// ───────────────────────────────────────────────
// 预置模版数据
// ───────────────────────────────────────────────

func presetTemplates() []PresetTemplate {
	return []PresetTemplate{
		// ═══ 网络设备 — 华为 ═══
		{
			Name: "华为 VRP 标准巡检模版", Vendor: "huawei", Category: "network", DeviceType: "switch",
			Description: "华为 VRP 系统标准巡检指令集（交换机/路由器/防火墙通用）",
			Commands: []PresetCommand{
				{Type: "cpu", Name: "CPU 使用率", Command: "display cpu-usage", Fallbacks: []string{"display cpu"}, Description: "检查设备 CPU 使用率，正常应低于 80%", Group: "资源层"},
				{Type: "memory", Name: "内存使用率", Command: "display memory-usage", Fallbacks: []string{"display memory"}, Description: "检查设备内存使用情况", Group: "资源层"},
				{Type: "interface", Name: "接口状态", Command: "display interface brief", Fallbacks: []string{"display ip interface brief"}, Description: "检查所有接口物理状态和协议状态", Group: "系统层"},
				{Type: "version", Name: "系统版本", Command: "display version", Description: "检查设备型号、软件版本和运行时间", Group: "系统层"},
				{Type: "routes", Name: "路由表", Command: "display ip routing-table", Fallbacks: []string{"display ip routing-table statistics"}, Description: "检查路由表状态和路由数量", Group: "系统层"},
				{Type: "log", Name: "日志缓冲区", Command: "display logbuffer", Fallbacks: []string{"display trapbuffer"}, Description: "检查最近的系统日志和告警信息", Group: "系统层"},
				{Type: "environment", Name: "环境状态", Command: "display temperature", Fallbacks: []string{"display device temperature", "display environment"}, Description: "检查设备温度和电压状态", Group: "硬件层"},
				{Type: "power", Name: "电源状态", Command: "display power", Fallbacks: []string{"display device power"}, Description: "检查电源模块状态", Group: "硬件层"},
				{Type: "fan", Name: "风扇状态", Command: "display fan", Fallbacks: []string{"display device fan"}, Description: "检查风扇模块运行状态", Group: "硬件层"},
				{Type: "stp", Name: "STP 状态", Command: "display stp", Fallbacks: []string{"display stp brief"}, Description: "检查生成树协议状态", Group: "系统层"},
				{Type: "vlan", Name: "VLAN 信息", Command: "display vlan", Description: "检查 VLAN 配置和端口成员", Group: "系统层"},
				{Type: "arp", Name: "ARP 表", Command: "display arp", Description: "检查 ARP 表项数量和状态", Group: "系统层"},
				{Type: "mac", Name: "MAC 地址表", Command: "display mac-address", Description: "检查 MAC 地址表项", Group: "系统层"},
				{Type: "optic", Name: "光模块信息", Command: "display transceiver verbose", Fallbacks: []string{"display optical-info"}, Description: "检查光模块收发光功率和温度", Group: "硬件层"},
				{Type: "neighbor", Name: "LLDP 邻居", Command: "display lldp neighbor brief", Description: "查看 LLDP 邻居发现信息", Group: "系统层"},
				{Type: "ntp", Name: "NTP 状态", Command: "display ntp status", Fallbacks: []string{"display ntp-service status"}, Description: "查看 NTP 同步状态", Group: "安全层"},
				{Type: "license", Name: "License 有效期", Command: "display license", Description: "查看 License 有效期和功能授权", Group: "安全层"},
				{Type: "config_checksum", Name: "配置快照", Command: "display current-configuration | include sysname", Description: "查看运行配置最后保存时间和摘要", Group: "安全层"},
			},
			Tags: []string{"华为", "VRP", "标准巡检"},
		},
		// ═══ 网络设备 — 思科 ═══
		{
			Name: "思科 IOS/IOS-XE 标准巡检模版", Vendor: "cisco", Category: "network", DeviceType: "switch",
			Description: "思科 IOS/IOS-XE/NX-OS 系统标准巡检指令集",
			Commands: []PresetCommand{
				{Type: "cpu", Name: "CPU 使用率", Command: "show processes cpu | include CPU utilization", Description: "检查设备 CPU 使用率", Group: "资源层"},
				{Type: "memory", Name: "内存使用率", Command: "show memory statistics", Description: "检查设备内存使用情况", Group: "资源层"},
				{Type: "interface", Name: "接口状态", Command: "show ip interface brief", Description: "检查所有接口物理状态和协议状态", Group: "系统层"},
				{Type: "version", Name: "系统版本", Command: "show version", Description: "检查设备型号、软件版本和运行时间", Group: "系统层"},
				{Type: "routes", Name: "路由表", Command: "show ip route", Description: "检查路由表状态和路由数量", Group: "系统层"},
				{Type: "log", Name: "日志缓冲区", Command: "show logging", Description: "检查最近的系统日志和告警信息", Group: "系统层"},
				{Type: "environment", Name: "环境状态", Command: "show environment all", Description: "检查设备温度和电压状态", Group: "硬件层"},
				{Type: "power", Name: "电源状态", Command: "show power", Description: "检查电源模块状态", Group: "硬件层"},
				{Type: "fan", Name: "风扇状态", Command: "show environment fan", Description: "检查风扇模块运行状态", Group: "硬件层"},
				{Type: "stp", Name: "STP 状态", Command: "show spanning-tree brief", Description: "检查生成树协议状态", Group: "系统层"},
				{Type: "vlan", Name: "VLAN 信息", Command: "show vlan brief", Description: "检查 VLAN 配置和端口成员", Group: "系统层"},
				{Type: "arp", Name: "ARP 表", Command: "show ip arp", Description: "检查 ARP 表项数量和状态", Group: "系统层"},
				{Type: "mac", Name: "MAC 地址表", Command: "show mac address-table", Description: "检查 MAC 地址表项", Group: "系统层"},
				{Type: "neighbor", Name: "LLDP/CDP 邻居", Command: "show lldp neighbors detail", Fallbacks: []string{"show cdp neighbors detail"}, Description: "查看 LLDP/CDP 邻居发现信息", Group: "系统层"},
				{Type: "ntp", Name: "NTP 状态", Command: "show ntp associations", Fallbacks: []string{"show ntp status"}, Description: "查看 NTP 同步状态", Group: "安全层"},
				{Type: "license", Name: "License 有效期", Command: "show license summary", Fallbacks: []string{"show license usage"}, Description: "查看 License 有效期和功能授权", Group: "安全层"},
				{Type: "config_checksum", Name: "配置快照", Command: "show running-config | include hostname", Description: "查看运行配置摘要快照", Group: "安全层"},
			},
			Tags: []string{"思科", "IOS", "标准巡检"},
		},
		// ═══ 网络设备 — 华三 ═══
		{
			Name: "华三 Comware 标准巡检模版", Vendor: "h3c", Category: "network", DeviceType: "switch",
			Description: "H3C Comware 系统标准巡检指令集",
			Commands: []PresetCommand{
				{Type: "cpu", Name: "CPU 使用率", Command: "display cpu-usage", Description: "检查 CPU 使用率", Group: "资源层"},
				{Type: "memory", Name: "内存使用率", Command: "display memory-usage", Fallbacks: []string{"display memory"}, Description: "检查内存使用情况", Group: "资源层"},
				{Type: "interface", Name: "接口状态", Command: "display interface brief", Description: "检查所有接口物理状态", Group: "系统层"},
				{Type: "version", Name: "系统版本", Command: "display version", Description: "检查设备型号、软件版本和运行时间", Group: "系统层"},
				{Type: "routes", Name: "路由表", Command: "display ip routing-table", Description: "检查路由表", Group: "系统层"},
				{Type: "log", Name: "日志缓冲区", Command: "display logbuffer", Fallbacks: []string{"display trapbuffer"}, Description: "检查最近系统日志", Group: "系统层"},
				{Type: "environment", Name: "环境状态", Command: "display environment", Fallbacks: []string{"display temperature"}, Description: "检查温度和电压", Group: "硬件层"},
				{Type: "power", Name: "电源状态", Command: "display power", Description: "检查电源状态", Group: "硬件层"},
				{Type: "fan", Name: "风扇状态", Command: "display fan", Description: "检查风扇状态", Group: "硬件层"},
				{Type: "stp", Name: "STP 状态", Command: "display stp", Fallbacks: []string{"display stp brief"}, Description: "检查 STP 状态", Group: "系统层"},
				{Type: "vlan", Name: "VLAN 信息", Command: "display vlan", Description: "检查 VLAN 配置", Group: "系统层"},
				{Type: "arp", Name: "ARP 表", Command: "display arp", Description: "检查 ARP 表项", Group: "系统层"},
				{Type: "mac", Name: "MAC 地址表", Command: "display mac-address", Description: "检查 MAC 地址表", Group: "系统层"},
				{Type: "neighbor", Name: "LLDP 邻居", Command: "display lldp neighbor brief", Description: "查看 LLDP 邻居发现信息", Group: "系统层"},
				{Type: "ntp", Name: "NTP 状态", Command: "display ntp status", Fallbacks: []string{"display ntp-service status"}, Description: "查看 NTP 同步状态", Group: "安全层"},
				{Type: "license", Name: "License 有效期", Command: "display license", Description: "查看 License 信息", Group: "安全层"},
			},
			Tags: []string{"华三", "H3C", "Comware"},
		},
		// ═══ 网络设备 — 锐捷 ═══
		{
			Name: "锐捷标准巡检模版", Vendor: "ruijie", Category: "network", DeviceType: "switch",
			Description: "锐捷 RGOS 系统标准巡检指令集",
			Commands: []PresetCommand{
				{Type: "cpu", Name: "CPU 使用率", Command: "show cpu", Description: "检查 CPU 使用率", Group: "资源层"},
				{Type: "memory", Name: "内存使用率", Command: "show memory", Description: "检查内存使用率", Group: "资源层"},
				{Type: "interface", Name: "接口状态", Command: "show interfaces status", Description: "检查所有接口状态", Group: "系统层"},
				{Type: "version", Name: "系统版本", Command: "show version", Description: "检查系统版本", Group: "系统层"},
				{Type: "routes", Name: "路由表", Command: "show ip route", Description: "检查路由表", Group: "系统层"},
				{Type: "log", Name: "日志缓冲区", Command: "show logging", Description: "检查日志", Group: "系统层"},
				{Type: "stp", Name: "STP 状态", Command: "show spanning-tree", Description: "检查 STP 状态", Group: "系统层"},
				{Type: "vlan", Name: "VLAN 信息", Command: "show vlan", Description: "检查 VLAN 配置", Group: "系统层"},
				{Type: "arp", Name: "ARP 表", Command: "show arp", Description: "检查 ARP 表", Group: "系统层"},
				{Type: "mac", Name: "MAC 地址表", Command: "show mac-address-table", Description: "检查 MAC 表", Group: "系统层"},
				{Type: "neighbor", Name: "LLDP 邻居", Command: "show lldp neighbors", Description: "查看 LLDP 邻居", Group: "系统层"},
				{Type: "ntp", Name: "NTP 状态", Command: "show ntp status", Description: "查看 NTP 状态", Group: "安全层"},
			},
			Tags: []string{"锐捷", "RGOS"},
		},
		// ═══ 网络设备 — 中兴 ═══
		{
			Name: "中兴 ZXR10 标准巡检模版", Vendor: "zte", Category: "network", DeviceType: "switch",
			Description: "中兴 ZXR10 系统标准巡检指令集",
			Commands: []PresetCommand{
				{Type: "cpu", Name: "CPU 使用率", Command: "show cpu", Description: "检查 CPU 使用率", Group: "资源层"},
				{Type: "memory", Name: "内存使用率", Command: "show memory", Description: "检查内存使用率", Group: "资源层"},
				{Type: "interface", Name: "接口状态", Command: "show interface brief", Description: "检查接口状态", Group: "系统层"},
				{Type: "version", Name: "系统版本", Command: "show version", Description: "检查系统版本", Group: "系统层"},
				{Type: "routes", Name: "路由表", Command: "show ip route", Description: "检查路由表", Group: "系统层"},
				{Type: "log", Name: "日志缓冲区", Command: "show log", Description: "检查日志", Group: "系统层"},
				{Type: "environment", Name: "环境状态", Command: "show environment", Description: "检查环境状态", Group: "硬件层"},
				{Type: "power", Name: "电源状态", Command: "show power", Description: "检查电源状态", Group: "硬件层"},
				{Type: "fan", Name: "风扇状态", Command: "show fan", Description: "检查风扇状态", Group: "硬件层"},
				{Type: "stp", Name: "STP 状态", Command: "show spanning-tree", Description: "检查 STP 状态", Group: "系统层"},
				{Type: "vlan", Name: "VLAN 信息", Command: "show vlan", Description: "检查 VLAN", Group: "系统层"},
				{Type: "arp", Name: "ARP 表", Command: "show arp", Description: "检查 ARP 表", Group: "系统层"},
				{Type: "mac", Name: "MAC 地址表", Command: "show mac-address-table", Description: "检查 MAC 表", Group: "系统层"},
			},
			Tags: []string{"中兴", "ZXR10"},
		},
		// ═══ 网络设备 — 深信服 ═══
		{
			Name: "深信服 AC/AF 标准巡检模版", Vendor: "sangfor", Category: "network", DeviceType: "firewall",
			Description: "深信服 AC（上网行为管理）/ AF（下一代防火墙）标准巡检指令集",
			Commands: []PresetCommand{
				{Type: "cpu", Name: "CPU 使用率", Command: "show system cpu", Fallbacks: []string{"sysctl hw.ncpu"}, Description: "检查 CPU 使用率", Group: "资源层"},
				{Type: "memory", Name: "内存使用率", Command: "show system memory", Description: "检查内存使用率", Group: "资源层"},
				{Type: "interface", Name: "接口状态", Command: "show interface", Fallbacks: []string{"ifconfig -a"}, Description: "检查所有接口状态", Group: "系统层"},
				{Type: "version", Name: "系统版本", Command: "show version", Description: "检查系统版本和运行时间", Group: "系统层"},
				{Type: "routes", Name: "路由表", Command: "show ip route", Fallbacks: []string{"netstat -rn"}, Description: "查看路由表", Group: "系统层"},
				{Type: "log", Name: "系统日志", Command: "show log system", Description: "查看最近系统日志", Group: "系统层"},
				{Type: "session", Name: "并发连接数", Command: "show session count", Description: "查看当前并发连接数", Group: "资源层"},
				{Type: "ntp", Name: "NTP 状态", Command: "show ntp status", Description: "查看 NTP 同步状态", Group: "安全层"},
				{Type: "license", Name: "License 状态", Command: "show license", Description: "查看 License 到期时间和模块授权", Group: "安全层"},
				{Type: "config_checksum", Name: "配置快照", Command: "show running-config | head -20", Description: "查看运行配置摘要", Group: "安全层"},
			},
			Tags: []string{"深信服", "Sangfor", "AC", "AF"},
		},
		// ═══ 网络设备 — 迈普 ═══
		{
			Name: "迈普 MyPowerOS 标准巡检模版", Vendor: "maipu", Category: "network", DeviceType: "switch",
			Description: "迈普 MyPowerOS 系统标准巡检指令集（交换机/路由器/网关通用）",
			Commands: []PresetCommand{
				{Type: "cpu", Name: "CPU 使用率", Command: "show cpu-usage", Fallbacks: []string{"show cpu"}, Description: "检查设备 CPU 使用率，正常应低于 80%", Group: "资源层"},
				{Type: "memory", Name: "内存使用率", Command: "show memory", Fallbacks: []string{"show memory-usage"}, Description: "检查设备内存使用情况", Group: "资源层"},
				{Type: "interface", Name: "接口状态", Command: "show interface brief", Fallbacks: []string{"show ip interface brief"}, Description: "检查所有接口物理状态和协议状态", Group: "系统层"},
				{Type: "version", Name: "系统版本", Command: "show version", Description: "检查设备型号、软件版本和运行时间", Group: "系统层"},
				{Type: "routes", Name: "路由表", Command: "show ip route", Description: "检查系统路由表状态和路由数量", Group: "系统层"},
				{Type: "log", Name: "日志缓冲区", Command: "show logging", Fallbacks: []string{"show log"}, Description: "检查最近的系统日志和告警信息", Group: "系统层"},
				{Type: "environment", Name: "环境状态", Command: "show environment", Fallbacks: []string{"show temperature"}, Description: "检查设备温度和运行环境", Group: "硬件层"},
				{Type: "power", Name: "电源状态", Command: "show power", Description: "检查电源模块运行状态", Group: "硬件层"},
				{Type: "fan", Name: "风扇状态", Command: "show fan", Description: "检查风扇模块运行状态", Group: "硬件层"},
				{Type: "stp", Name: "STP 状态", Command: "show spanning-tree", Description: "检查生成树协议状态", Group: "系统层"},
				{Type: "vlan", Name: "VLAN 信息", Command: "show vlan", Description: "检查 VLAN 配置和端口划分", Group: "系统层"},
				{Type: "arp", Name: "ARP 表", Command: "show arp", Description: "检查 ARP 表项数量和状态", Group: "系统层"},
				{Type: "mac", Name: "MAC 地址表", Command: "show mac-address-table", Fallbacks: []string{"show mac-address"}, Description: "检查 MAC 地址表项", Group: "系统层"},
				{Type: "neighbor", Name: "LLDP 邻居", Command: "show lldp neighbor brief", Fallbacks: []string{"show lldp neighbor"}, Description: "查看 LLDP 拓扑邻居发现信息", Group: "系统层"},
				{Type: "ntp", Name: "NTP 状态", Command: "show ntp status", Description: "查看 NTP 时间同步状态", Group: "安全层"},
				{Type: "license", Name: "License 状态", Command: "show license", Description: "查看 License 授权与有效期", Group: "安全层"},
				{Type: "config_checksum", Name: "配置快照", Command: "show running-config | include hostname", Description: "查看当前运行配置摘要", Group: "安全层"},
			},
			Tags: []string{"迈普", "Maipu", "MyPowerOS", "标准巡检"},
		},
		// ═══ 网络设备 — 飞塔 ═══
		{
			Name: "Fortinet FortiGate 标准巡检模版", Vendor: "fortinet", Category: "network", DeviceType: "firewall",
			Description: "Fortinet FortiGate 防火墙标准巡检指令集",
			Commands: []PresetCommand{
				{Type: "cpu", Name: "CPU 使用率", Command: "get system performance status", Fallbacks: []string{"get system perf"}, Description: "检查 CPU 使用率", Group: "资源层"},
				{Type: "memory", Name: "内存使用率", Command: "get system performance status", Description: "检查内存使用率", Group: "资源层"},
				{Type: "interface", Name: "接口状态", Command: "show system interface physical", Description: "检查所有接口状态", Group: "系统层"},
				{Type: "version", Name: "系统版本", Command: "get system status", Description: "检查固件版本和运行时间", Group: "系统层"},
				{Type: "log", Name: "最近日志", Command: "execute log filter category event", Description: "查看最近系统日志", Group: "系统层"},
				{Type: "security_policy", Name: "防火墙策略", Command: "show firewall policy", Description: "查看所有防火墙策略", Group: "安全层"},
				{Type: "session", Name: "会话统计", Command: "diagnose sys session count", Fallbacks: []string{"get system sessions"}, Description: "查看并发会话统计", Group: "资源层"},
				{Type: "vpn", Name: "VPN 隧道", Command: "diagnose vpn tunnel list", Description: "查看 IPSec VPN 隧道", Group: "安全层"},
				{Type: "routes", Name: "路由表", Command: "get router info routing-table all", Description: "查看路由表", Group: "系统层"},
				{Type: "license", Name: "License / 订阅", Command: "get system license", Fallbacks: []string{"show system fortiguard"}, Description: "检查 License 到期日和 FortiGuard 订阅", Group: "安全层"},
				{Type: "environment", Name: "硬件状态", Command: "get system hardware status", Description: "查看温度风扇电源", Group: "硬件层"},
			},
			Tags: []string{"Fortinet", "FortiGate"},
		},
		// ═══ 网络设备 — 瞻博 ═══
		{
			Name: "Juniper JunOS 标准巡检模版", Vendor: "juniper", Category: "network", DeviceType: "router",
			Description: "Juniper JunOS 系统标准巡检指令集",
			Commands: []PresetCommand{
				{Type: "cpu", Name: "CPU 使用率", Command: "show system processes extensive | match idle", Fallbacks: []string{"show chassis routing-engine"}, Description: "检查 CPU 使用率", Group: "资源层"},
				{Type: "memory", Name: "内存使用率", Command: "show system memory", Fallbacks: []string{"show chassis routing-engine"}, Description: "检查内存使用率", Group: "资源层"},
				{Type: "interface", Name: "接口状态", Command: "show interfaces terse", Description: "检查所有接口状态", Group: "系统层"},
				{Type: "version", Name: "系统版本", Command: "show version", Description: "检查 JunOS 版本和运行时间", Group: "系统层"},
				{Type: "routes", Name: "路由表", Command: "show route summary", Description: "查看路由表概要", Group: "系统层"},
				{Type: "log", Name: "最近日志", Command: "show log messages | last 30", Description: "查看最近系统日志", Group: "系统层"},
				{Type: "environment", Name: "环境状态", Command: "show chassis hardware", Fallbacks: []string{"show chassis environment"}, Description: "查看机框硬件信息", Group: "硬件层"},
				{Type: "power", Name: "电源状态", Command: "show chassis power", Description: "查看电源状态", Group: "硬件层"},
				{Type: "fan", Name: "风扇状态", Command: "show chassis fan", Description: "查看风扇状态", Group: "硬件层"},
				{Type: "neighbor", Name: "LLDP 邻居", Command: "show lldp neighbors", Description: "查看 LLDP 邻居", Group: "系统层"},
				{Type: "ntp", Name: "NTP 状态", Command: "show ntp associations", Description: "查看 NTP 同步状态", Group: "安全层"},
				{Type: "license", Name: "License 信息", Command: "show system license", Description: "查看 License 信息", Group: "安全层"},
			},
			Tags: []string{"Juniper", "JunOS"},
		},
		// ═══ 网络设备 — PaloAlto ═══
		{
			Name: "Palo Alto PAN-OS 标准巡检模版", Vendor: "paloalto", Category: "network", DeviceType: "firewall",
			Description: "Palo Alto PAN-OS 防火墙标准巡检指令集",
			Commands: []PresetCommand{
				{Type: "cpu", Name: "CPU 使用率", Command: "show system resources | match cpu", Description: "检查 CPU 使用率", Group: "资源层"},
				{Type: "memory", Name: "内存使用率", Command: "show system resources | match mem", Description: "检查内存使用率", Group: "资源层"},
				{Type: "interface", Name: "接口状态", Command: "show interface all", Description: "检查所有接口状态", Group: "系统层"},
				{Type: "version", Name: "系统版本", Command: "show system info", Description: "查看 PAN-OS 版本", Group: "系统层"},
				{Type: "log", Name: "最近日志", Command: "show log system direction equal forward | tail 30", Description: "查看最近系统日志", Group: "系统层"},
				{Type: "security_policy", Name: "安全策略", Command: "show running security-policy", Description: "查看所有安全策略", Group: "安全层"},
				{Type: "session", Name: "会话统计", Command: "show session info", Fallbacks: []string{"show session summary"}, Description: "查看会话统计", Group: "资源层"},
				{Type: "vpn", Name: "VPN 隧道", Command: "show vpn ipsec tunnel", Description: "查看 IPSec VPN 隧道", Group: "安全层"},
				{Type: "routes", Name: "路由表", Command: "show routing route", Description: "查看路由表", Group: "系统层"},
				{Type: "license", Name: "License 状态", Command: "show license info", Description: "查看 License 到期日", Group: "安全层"},
				{Type: "environment", Name: "硬件状态", Command: "show system environment", Description: "查看温度/风扇", Group: "硬件层"},
			},
			Tags: []string{"PaloAlto", "PAN-OS"},
		},
		// ═══ 网络设备 — HPE ═══
		{
			Name: "HPE/Aruba 标准巡检模版", Vendor: "hpe", Category: "network", DeviceType: "switch",
			Description: "HPE/Aruba ProVision/Comware 系统标准巡检指令集",
			Commands: []PresetCommand{
				{Type: "cpu", Name: "CPU 使用率", Command: "show system cpu", Description: "检查 CPU 使用率", Group: "资源层"},
				{Type: "memory", Name: "内存使用率", Command: "show system memory", Description: "检查内存使用率", Group: "资源层"},
				{Type: "interface", Name: "接口状态", Command: "show interfaces brief", Description: "检查所有接口状态", Group: "系统层"},
				{Type: "version", Name: "系统版本", Command: "show version", Description: "检查系统版本", Group: "系统层"},
				{Type: "routes", Name: "路由表", Command: "show ip route", Description: "查看路由表", Group: "系统层"},
				{Type: "log", Name: "最近日志", Command: "show logging -r", Description: "查看最近日志", Group: "系统层"},
				{Type: "stp", Name: "STP 状态", Command: "show spanning-tree", Description: "查看 STP 状态", Group: "系统层"},
				{Type: "vlan", Name: "VLAN 信息", Command: "show vlan", Description: "查看 VLAN 配置", Group: "系统层"},
				{Type: "neighbor", Name: "LLDP 邻居", Command: "show lldp info remote-device", Description: "查看 LLDP 邻居", Group: "系统层"},
				{Type: "environment", Name: "环境状态", Command: "show system environment", Description: "查看温度/电源/风扇", Group: "硬件层"},
				{Type: "power", Name: "电源状态", Command: "show system power-supply", Description: "查看电源状态", Group: "硬件层"},
				{Type: "fan", Name: "风扇状态", Command: "show system fan", Description: "查看风扇状态", Group: "硬件层"},
			},
			Tags: []string{"HPE", "Aruba"},
		},
		// ═══ 服务器 — 华为 SmartKit 服务器五层深度巡检 ═══
		{
			Name: "华为 SmartKit 服务器五层深度巡检模版", Vendor: "huawei", Category: "server", DeviceType: "server", OSType: "linux",
			Description: "参考华为 SmartKit 巡检工具标准规范，从硬件层、系统层、资源层、安全层、应用层 5 大维度对服务器进行深度健康检查与诊断",
			Commands: []PresetCommand{
				// 硬件层 (Hardware Layer)
				{Type: "hardware", Name: "服务器型号与SN", Command: "dmidecode -t system 2>/dev/null | grep -E 'Manufacturer|Product Name|Serial Number|UUID' || echo N/A", Description: "检查服务器整机型号、主板厂商和设备序列号(SN)", Group: "硬件层"},
				{Type: "cpu_info", Name: "CPU 规格与微码", Command: "lscpu 2>/dev/null | grep -E 'Model name|Socket|Thread|NUMA|CPU MHz' || cat /proc/cpuinfo | grep 'model name' | head -4", Description: "检查 CPU 型号、物理插槽、核心数、线程及 NUMA 架构", Group: "硬件层"},
				{Type: "memory_info", Name: "内存插槽与ECC状态", Command: "dmidecode -t memory 2>/dev/null | grep -E 'Size|Type|Speed|Error' | grep -v 'No Module' | head -24 || free -m", Description: "检查各内存条插槽容量、频率、规格及 ECC 报错", Group: "硬件层"},
				{Type: "disk_info", Name: "磁盘物理拓扑", Command: "lsblk -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINT,MODEL 2>/dev/null || fdisk -l 2>/dev/null | grep -E 'Disk /dev/'", Description: "检查所有物理磁盘、分区、文件系统及挂载点", Group: "硬件层"},
				{Type: "raid_status", Name: "RAID 卡与阵列健康", Command: "megacli -LDInfo -Lall -aALL 2>/dev/null || storcli /call show 2>/dev/null || arcconf getconfig 1 ld 2>/dev/null || echo 'RAID Check: No HW RAID utility or Passthrough'", Description: "检查硬件 RAID 卡控制器状态、BBU 电池与虚拟驱动器健康", Group: "硬件层"},
				{Type: "ipmi_bmc", Name: "iBMC/IPMI 硬件传感器与事件", Command: "ipmitool sdr list 2>/dev/null | head -25 || ipmitool sel elist 2>/dev/null | tail -15 || echo 'IPMI/BMC SDR: Normal or Not Configured'", Description: "检查 iBMC/IPMI 硬件传感器（电压、温度、风扇）与系统事件日志(SEL)", Group: "硬件层"},
				{Type: "power_fan", Name: "电源冗余与风扇转速", Command: "ipmitool sdr type 'Power Supply' 2>/dev/null ; ipmitool sdr type Fan 2>/dev/null || echo 'PSU/Fan: Health Check OK'", Description: "检查主备电源双冗余供电状态与各风扇模块转速", Group: "硬件层"},
				// 系统层 (System Layer)
				{Type: "os_version", Name: "OS 发行版与内核", Command: "cat /etc/os-release 2>/dev/null | head -5 ; uname -r ; uname -m", Description: "检查 Linux 操作系统发行版本、内核版本与系统架构", Group: "系统层"},
				{Type: "uptime", Name: "系统运行时间与开机时长", Command: "uptime", Description: "检查系统运行天数、平均负载 (1/5/15分钟)", Group: "系统层"},
				{Type: "hostname", Name: "主机名与网卡配置", Command: "hostname ; ip -brief address 2>/dev/null || ifconfig -s", Description: "检查主机名解析与各物理网卡 IP 配置", Group: "系统层"},
				{Type: "time_sync", Name: "系统时区与时间同步", Command: "timedatectl 2>/dev/null || date -R", Description: "检查系统时区配置与 NTP/Chrony 同步精度", Group: "系统层"},
				{Type: "kernel_params", Name: "内核关键调优参数", Command: "sysctl vm.swappiness vm.max_map_count net.core.somaxconn fs.file-max 2>/dev/null", Description: "检查虚拟内存、连接队列与最大文件句柄等关键内核参数", Group: "系统层"},
				// 资源层 (Resource Layer)
				{Type: "cpu_usage", Name: "CPU 综合负载与 TOP 进程", Command: "top -bn1 | head -15", Description: "查看当前 CPU 总体利用率及占用最高的进程 TOP10", Group: "资源层"},
				{Type: "memory_usage", Name: "内存与 Swap 占用", Command: "free -h", Description: "查看物理内存总量、已用、可用及 Swap 交换分区使用率", Group: "资源层"},
				{Type: "disk_usage", Name: "磁盘空间使用率", Command: "df -hT -x tmpfs -x devtmpfs 2>/dev/null || df -h", Description: "检查各持久化挂载分区磁盘占用率（预警阈值 >85%）", Group: "资源层"},
				{Type: "io_stats", Name: "磁盘 I/O 读写性能", Command: "iostat -xz 1 2 2>/dev/null || vmstat 1 2 2>/dev/null || echo 'I/O Stats Captured'", Description: "检查磁盘 I/O 吞吐、iops 读写延迟与 util 饱和度", Group: "资源层"},
				{Type: "network_stats", Name: "网络连接与套接字统计", Command: "ss -s 2>/dev/null || netstat -s | head -20", Description: "查看 TCP/UDP 连接数、TIME_WAIT 与 ESTABLISHED 状态统计", Group: "资源层"},
				// 安全层 (Security Layer)
				{Type: "ssh_config", Name: "SSH 安全基线", Command: "grep -E '^(PermitRootLogin|PasswordAuthentication|Port|MaxAuthTries)' /etc/ssh/sshd_config 2>/dev/null || echo N/A", Description: "检查 SSH 远程访问协议安全基线配置", Group: "安全层"},
				{Type: "firewall_ports", Name: "防火墙与监听端口", Command: "systemctl is-active firewalld ufw 2>/dev/null ; ss -tunlp 2>/dev/null | head -20 || echo N/A", Description: "检查防火墙运行状态与当前系统开放监听的公网端口", Group: "安全层"},
				{Type: "login_audit", Name: "登录审计与爆破排查", Command: "last -n 5 2>/dev/null ; grep 'Failed password' /var/log/secure /var/log/auth.log 2>/dev/null | tail -5 || echo 'No abnormal login failure'", Description: "排查最近成功登录记录与 SSH 暴力破解告警", Group: "安全层"},
				{Type: "cron_audit", Name: "定时任务安全审计", Command: "crontab -l 2>/dev/null ; ls -la /etc/cron.* 2>/dev/null || echo N/A", Description: "检查用户 crontab 与系统级定时任务是否存在异常后门", Group: "安全层"},
				// 应用层 (Application Layer)
				{Type: "failed_services", Name: "崩溃与故障服务", Command: "systemctl --failed --plain --no-pager 2>/dev/null || echo 'All services active'", Description: "检查 systemd 中处于 failed 异常退出的后台服务", Group: "应用层"},
				{Type: "running_services", Name: "关键运行服务列表", Command: "systemctl list-units --type=service --state=running 2>/dev/null | head -20", Description: "查看当前系统活跃运行的核心服务", Group: "应用层"},
				{Type: "container_status", Name: "容器引擎状态", Command: "docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' 2>/dev/null || podman ps 2>/dev/null || echo 'Container Engine: Not Installed or Inactive'", Description: "检查 Docker / Podman 容器运行时与容器实例健康", Group: "应用层"},
				{Type: "dmesg_errors", Name: "内核 dmesg 严重告警", Command: "dmesg -T --level=err,crit,alert,emerg 2>/dev/null | tail -15 || dmesg | grep -iE 'error|fault|segfault|oom-killer' | tail -10 || echo 'dmesg: Clean'", Description: "检查 Linux 内核日志是否存在 OOM Killer、硬件报错或段错误", Group: "应用层"},
			},
			Tags: []string{"SmartKit", "华为", "服务器", "五层巡检", "硬件诊断"},
		},
		// ═══ 服务器 — 浪潮 (Inspur) ═══
		{
			Name: "浪潮 NF/英信服务器五层标准巡检模版", Vendor: "inspur", Category: "server", DeviceType: "server", OSType: "linux",
			Description: "浪潮 (Inspur) NF5280/NF8480 等英信系列服务器 5 层标准健康巡检指令集",
			Commands: []PresetCommand{
				{Type: "hardware", Name: "浪潮整机与BMC", Command: "dmidecode -t system | grep -E 'Manufacturer|Product Name|Serial Number' ; ipmitool mc info 2>/dev/null || echo N/A", Description: "检查浪潮整机主板型号与 BMC 固件", Group: "硬件层"},
				{Type: "cpu_info", Name: "CPU 拓扑与规格", Command: "lscpu | grep -E 'Model name|Socket|Thread|NUMA' ; cat /proc/loadavg", Description: "检查 CPU 核心规格与负载", Group: "硬件层"},
				{Type: "memory_info", Name: "内存槽位与ECC健康", Command: "dmidecode -t memory | grep -E 'Size|Type|Speed|Error' | grep -v 'No Module'", Description: "检查物理内存插槽与 ECC 校验", Group: "硬件层"},
				{Type: "disk_info", Name: "磁盘与SAS控制器", Command: "lsblk -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINT ; megacli -LDInfo -Lall -aALL 2>/dev/null || echo Passthrough", Description: "检查硬盘拓扑与 LSI/Inspur RAID 状态", Group: "硬件层"},
				{Type: "sensors", Name: "风扇转速与主板温度", Command: "ipmitool sensor 2>/dev/null | grep -iE 'temp|fan|power' | head -15 || sensors 2>/dev/null || echo N/A", Description: "检查风扇转速与环境温度", Group: "硬件层"},
				{Type: "os_version", Name: "OS 与内核版本", Command: "cat /etc/os-release ; uname -r", Description: "检查操作系统发行版与内核", Group: "系统层"},
				{Type: "uptime", Name: "连续运行时间", Command: "uptime", Description: "检查服务器连续开机时长与平均负载", Group: "系统层"},
				{Type: "cpu_usage", Name: "CPU 负载 TOP10", Command: "top -bn1 | head -15", Description: "检查 CPU 利用率最高进程", Group: "资源层"},
				{Type: "memory_usage", Name: "内存与 Swap 占用", Command: "free -h", Description: "检查物理内存与 Swap 占用", Group: "资源层"},
				{Type: "disk_usage", Name: "磁盘空间利用率", Command: "df -hT -x tmpfs -x devtmpfs", Description: "检查分区挂载与剩余容量", Group: "资源层"},
				{Type: "ssh_config", Name: "SSH 安全基线", Command: "grep -E '^(PermitRootLogin|PasswordAuthentication|Port)' /etc/ssh/sshd_config", Description: "检查 SSH 安全配置", Group: "安全层"},
				{Type: "firewall", Name: "防火墙运行状态", Command: "systemctl is-active firewalld ufw 2>/dev/null || iptables -L -n | head -10", Description: "检查系统防火墙", Group: "安全层"},
				{Type: "failed_services", Name: "异常崩溃服务", Command: "systemctl --failed --plain --no-pager", Description: "检查异常退出的服务", Group: "应用层"},
				{Type: "dmesg_errors", Name: "内核 dmesg 报错", Command: "dmesg -T --level=err,crit 2>/dev/null | tail -10 || echo 'Clean'", Description: "排查内核严重告警", Group: "应用层"},
			},
			Tags: []string{"浪潮", "Inspur", "服务器", "五层巡检"},
		},
		// ═══ 服务器 — 清华同方 (Tongfang) ═══
		{
			Name: "清华同方信创超强服务器五层巡检模版", Vendor: "tongfang", Category: "server", DeviceType: "server", OSType: "linux",
			Description: "清华同方超强信创服务器（飞腾/鲲鹏/海光架构）五层深度巡检指令集",
			Commands: []PresetCommand{
				{Type: "hardware", Name: "同方整机与架构", Command: "dmidecode -t system | grep -E 'Manufacturer|Product Name|Serial Number' ; uname -m", Description: "检查同方整机型号与 CPU 指令架构", Group: "硬件层"},
				{Type: "cpu_info", Name: "CPU 与 NUMA 状态", Command: "lscpu ; numactl --hardware 2>/dev/null || echo N/A", Description: "检查信创多核 CPU 拓扑与 NUMA 节点", Group: "硬件层"},
				{Type: "memory_info", Name: "内存插槽与可用容量", Command: "dmidecode -t memory | grep -E 'Size|Speed|Type' | grep -v 'No Module' ; free -h", Description: "检查物理内存条规格与可用容量", Group: "硬件层"},
				{Type: "disk_info", Name: "存储设备拓扑", Command: "lsblk -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINT", Description: "检查 NVMe/SATA 硬盘与分区挂载", Group: "硬件层"},
				{Type: "sensors", Name: "BMC 传感器读数", Command: "ipmitool sdr elist 2>/dev/null | head -15 || echo N/A", Description: "检查主板电压、温度与风扇读数", Group: "硬件层"},
				{Type: "os_version", Name: "信创 OS 与内核", Command: "cat /etc/os-release ; uname -a", Description: "检查统信 UOS/麒麟 Kylin 发行版与内核版本", Group: "系统层"},
				{Type: "uptime", Name: "系统运行时间", Command: "uptime", Description: "检查系统运行天数与负载", Group: "系统层"},
				{Type: "cpu_usage", Name: "CPU 使用率分析", Command: "top -bn1 | head -15", Description: "检查 CPU 占用最高进程", Group: "资源层"},
				{Type: "memory_usage", Name: "内存使用率", Command: "free -m ; vmstat 1 2", Description: "检查物理内存剩余与虚拟内存交换", Group: "资源层"},
				{Type: "disk_usage", Name: "磁盘空间利用率", Command: "df -hT -x tmpfs -x devtmpfs", Description: "检查各挂载点存储占用", Group: "资源层"},
				{Type: "ssh_config", Name: "SSH 安全基线", Command: "grep -E '^(PermitRootLogin|PasswordAuthentication|Port)' /etc/ssh/sshd_config", Description: "检查 SSH 安全配置", Group: "安全层"},
				{Type: "login_audit", Name: "最近登录审计", Command: "last -n 5 ; who", Description: "查看近期登录会话", Group: "安全层"},
				{Type: "failed_services", Name: "系统故障服务", Command: "systemctl --failed --plain --no-pager", Description: "排查退出异常的服务", Group: "应用层"},
				{Type: "dmesg_errors", Name: "内核与硬件告警", Command: "dmesg -T --level=err,crit 2>/dev/null | tail -10 || echo 'Clean'", Description: "检查内核硬件级报错", Group: "应用层"},
			},
			Tags: []string{"清华同方", "Tongfang", "信创", "服务器", "五层巡检"},
		},
		// ═══ 服务器 — 联想 (Lenovo) ═══
		{
			Name: "联想 ThinkSystem 服务器五层巡检模版", Vendor: "lenovo", Category: "server", DeviceType: "server", OSType: "linux",
			Description: "联想 ThinkSystem SR650/SR850 等系列服务器五层标准健康巡检指令集",
			Commands: []PresetCommand{
				{Type: "hardware", Name: "ThinkSystem整机信息", Command: "dmidecode -t system | grep -E 'Manufacturer|Product Name|Serial Number' ; ipmitool mc info 2>/dev/null || echo N/A", Description: "检查联想主机型号与 XCC 固件", Group: "硬件层"},
				{Type: "cpu_info", Name: "CPU 处理器状态", Command: "lscpu | grep -E 'Model name|Socket|Thread'", Description: "检查 Xeon / EPYC 处理器规格", Group: "硬件层"},
				{Type: "memory_info", Name: "内存槽位与 ECC", Command: "dmidecode -t memory | grep -E 'Size|Type|Speed' | grep -v 'No Module'", Description: "检查内存通道与健康", Group: "硬件层"},
				{Type: "disk_info", Name: "RAID 与磁盘拓扑", Command: "lsblk -o NAME,SIZE,TYPE,MOUNTPOINT ; megacli -LDInfo -Lall -aALL 2>/dev/null || echo Passthrough", Description: "检查 ThinkSystem RAID 卡与硬盘拓扑", Group: "硬件层"},
				{Type: "os_version", Name: "OS 与内核版本", Command: "cat /etc/os-release ; uname -r", Description: "检查系统版本与内核", Group: "系统层"},
				{Type: "uptime", Name: "系统运行时间", Command: "uptime", Description: "检查系统运行时间", Group: "系统层"},
				{Type: "cpu_usage", Name: "CPU 负载", Command: "top -bn1 | head -15", Description: "检查 CPU 负载情况", Group: "资源层"},
				{Type: "memory_usage", Name: "内存使用率", Command: "free -h", Description: "检查内存占用", Group: "资源层"},
				{Type: "disk_usage", Name: "磁盘空间利用率", Command: "df -hT -x tmpfs -x devtmpfs", Description: "检查磁盘分区", Group: "资源层"},
				{Type: "ssh_config", Name: "SSH 安全基线", Command: "grep -E '^(PermitRootLogin|PasswordAuthentication|Port)' /etc/ssh/sshd_config", Description: "检查 SSH 安全", Group: "安全层"},
				{Type: "failed_services", Name: "异常服务检查", Command: "systemctl --failed --plain --no-pager", Description: "检查异常服务", Group: "应用层"},
			},
			Tags: []string{"联想", "Lenovo", "ThinkSystem", "服务器", "五层巡检"},
		},
		// ═══ 服务器 — 戴尔 (Dell) ═══
		{
			Name: "戴尔 PowerEdge 服务器五层巡检模版", Vendor: "dell", Category: "server", DeviceType: "server", OSType: "linux",
			Description: "戴尔 PowerEdge R740/R750/R650 等机架服务器及 iDRAC 五层标准巡检指令集",
			Commands: []PresetCommand{
				{Type: "hardware", Name: "PowerEdge 整机与服务标签", Command: "dmidecode -t system | grep -E 'Manufacturer|Product Name|Serial Number' ; racadm getsysinfo 2>/dev/null || ipmitool mc info 2>/dev/null || echo N/A", Description: "检查戴尔主机 Service Tag 与 iDRAC 状态", Group: "硬件层"},
				{Type: "cpu_info", Name: "CPU 处理器规格", Command: "lscpu | grep -E 'Model name|Socket|Thread'", Description: "检查 CPU 规格与核心", Group: "硬件层"},
				{Type: "memory_info", Name: "内存槽位与 ECC", Command: "dmidecode -t memory | grep -E 'Size|Type|Speed' | grep -v 'No Module'", Description: "检查物理内存规格与槽位分布", Group: "硬件层"},
				{Type: "disk_info", Name: "PERC RAID 与磁盘拓扑", Command: "lsblk -o NAME,SIZE,TYPE,MOUNTPOINT ; perccli /call show 2>/dev/null || megacli -LDInfo -Lall -aALL 2>/dev/null || echo Passthrough", Description: "检查戴尔 PERC RAID 卡与物理硬盘健康", Group: "硬件层"},
				{Type: "os_version", Name: "OS 与内核版本", Command: "cat /etc/os-release ; uname -r", Description: "检查系统发行版与内核", Group: "系统层"},
				{Type: "uptime", Name: "系统运行时间", Command: "uptime", Description: "检查服务器连续开机时长与负载", Group: "系统层"},
				{Type: "cpu_usage", Name: "CPU 负载 TOP10", Command: "top -bn1 | head -15", Description: "检查 CPU 占用最高进程", Group: "资源层"},
				{Type: "memory_usage", Name: "内存与 Swap 占用", Command: "free -h", Description: "检查内存剩余量", Group: "资源层"},
				{Type: "disk_usage", Name: "磁盘空间利用率", Command: "df -hT -x tmpfs -x devtmpfs", Description: "检查分区挂载容量", Group: "资源层"},
				{Type: "ssh_config", Name: "SSH 安全基线", Command: "grep -E '^(PermitRootLogin|PasswordAuthentication|Port)' /etc/ssh/sshd_config", Description: "检查 SSH 安全", Group: "安全层"},
				{Type: "failed_services", Name: "异常故障服务", Command: "systemctl --failed --plain --no-pager", Description: "检查异常服务", Group: "应用层"},
			},
			Tags: []string{"戴尔", "Dell", "PowerEdge", "iDRAC", "服务器", "五层巡检"},
		},
		// ═══ 服务器 — 惠普 (HPE) ═══
		{
			Name: "惠普 ProLiant / Apollo 服务器五层巡检模版", Vendor: "hpe", Category: "server", DeviceType: "server", OSType: "linux",
			Description: "惠普 ProLiant DL380/DL360/Apollo 等机架服务器及 iLO 5 层标准健康巡检指令集",
			Commands: []PresetCommand{
				{Type: "hardware", Name: "HPE 整机与 iLO 状态", Command: "dmidecode -t system | grep -E 'Manufacturer|Product Name|Serial Number' ; hplog -v 2>/dev/null || ipmitool mc info 2>/dev/null || echo N/A", Description: "检查 HPE 主机型号与 iLO 状态", Group: "硬件层"},
				{Type: "cpu_info", Name: "CPU 规格拓扑", Command: "lscpu | grep -E 'Model name|Socket|Thread'", Description: "检查 CPU 核心", Group: "硬件层"},
				{Type: "memory_info", Name: "内存槽位与 ECC", Command: "dmidecode -t memory | grep -E 'Size|Type|Speed' | grep -v 'No Module'", Description: "检查内存条与槽位", Group: "硬件层"},
				{Type: "disk_info", Name: "Smart Array 与存储", Command: "lsblk -o NAME,SIZE,TYPE,MOUNTPOINT ; ssacli ctrl all show config 2>/dev/null || megacli -LDInfo -Lall -aALL 2>/dev/null || echo Passthrough", Description: "检查 HPE Smart Array 阵列卡与磁盘状态", Group: "硬件层"},
				{Type: "os_version", Name: "OS 与内核版本", Command: "cat /etc/os-release ; uname -r", Description: "检查操作系统版本", Group: "系统层"},
				{Type: "uptime", Name: "系统运行时间", Command: "uptime", Description: "检查系统运行时间", Group: "系统层"},
				{Type: "cpu_usage", Name: "CPU 负载", Command: "top -bn1 | head -15", Description: "检查 CPU 负载", Group: "资源层"},
				{Type: "memory_usage", Name: "内存使用率", Command: "free -h", Description: "检查物理内存与 Swap", Group: "资源层"},
				{Type: "disk_usage", Name: "磁盘空间利用率", Command: "df -hT -x tmpfs -x devtmpfs", Description: "检查磁盘分区占用", Group: "资源层"},
				{Type: "ssh_config", Name: "SSH 安全基线", Command: "grep -E '^(PermitRootLogin|PasswordAuthentication|Port)' /etc/ssh/sshd_config", Description: "检查 SSH 安全", Group: "安全层"},
				{Type: "failed_services", Name: "异常服务检查", Command: "systemctl --failed --plain --no-pager", Description: "检查故障服务", Group: "应用层"},
			},
			Tags: []string{"惠普", "HPE", "ProLiant", "iLO", "服务器", "五层巡检"},
		},
		// ═══ 服务器 — 新华三 (H3C) ═══
		{
			Name: "新华三 H3C UniServer 服务器五层巡检模版", Vendor: "h3c", Category: "server", DeviceType: "server", OSType: "linux",
			Description: "新华三 H3C UniServer R4900/R4300 等系列服务器五层标准健康巡检指令集",
			Commands: []PresetCommand{
				{Type: "hardware", Name: "UniServer 整机与 HDM", Command: "dmidecode -t system | grep -E 'Manufacturer|Product Name|Serial Number' ; ipmitool mc info 2>/dev/null || echo N/A", Description: "检查 H3C 整机型号与 HDM 管理模块", Group: "硬件层"},
				{Type: "cpu_info", Name: "CPU 规格与负载", Command: "lscpu | grep -E 'Model name|Socket|Thread'", Description: "检查 CPU 核心", Group: "硬件层"},
				{Type: "memory_info", Name: "内存插槽与 ECC", Command: "dmidecode -t memory | grep -E 'Size|Type|Speed' | grep -v 'No Module'", Description: "检查内存规格", Group: "硬件层"},
				{Type: "disk_info", Name: "存储与 RAID 阵列", Command: "lsblk -o NAME,SIZE,TYPE,MOUNTPOINT ; megacli -LDInfo -Lall -aALL 2>/dev/null || echo Passthrough", Description: "检查磁盘与 RAID 状态", Group: "硬件层"},
				{Type: "os_version", Name: "OS 与内核版本", Command: "cat /etc/os-release ; uname -r", Description: "检查系统版本", Group: "系统层"},
				{Type: "uptime", Name: "系统运行时间", Command: "uptime", Description: "检查系统运行时间", Group: "系统层"},
				{Type: "cpu_usage", Name: "CPU 使用率", Command: "top -bn1 | head -15", Description: "检查 CPU 使用率", Group: "资源层"},
				{Type: "memory_usage", Name: "内存使用率", Command: "free -h", Description: "检查内存使用情况", Group: "资源层"},
				{Type: "disk_usage", Name: "磁盘空间利用率", Command: "df -hT -x tmpfs -x devtmpfs", Description: "检查分区利用率", Group: "资源层"},
				{Type: "ssh_config", Name: "SSH 安全基线", Command: "grep -E '^(PermitRootLogin|PasswordAuthentication|Port)' /etc/ssh/sshd_config", Description: "检查 SSH 安全", Group: "安全层"},
				{Type: "failed_services", Name: "异常服务排查", Command: "systemctl --failed --plain --no-pager", Description: "检查异常服务", Group: "应用层"},
			},
			Tags: []string{"新华三", "H3C", "UniServer", "服务器", "五层巡检"},
		},
		// ═══ 服务器 — 中科曙光 (Sugon) ═══
		{
			Name: "中科曙光天阔/I系列服务器五层巡检模版", Vendor: "sugon", Category: "server", DeviceType: "server", OSType: "linux",
			Description: "中科曙光 (Sugon) I620 / 天阔系列服务器五层标准健康巡检指令集",
			Commands: []PresetCommand{
				{Type: "hardware", Name: "曙光整机与 BMC", Command: "dmidecode -t system | grep -E 'Manufacturer|Product Name|Serial Number' ; ipmitool mc info 2>/dev/null || echo N/A", Description: "检查中科曙光整机型号与 BMC", Group: "硬件层"},
				{Type: "cpu_info", Name: "CPU 处理器规格", Command: "lscpu | grep -E 'Model name|Socket|Thread|NUMA'", Description: "检查 CPU 核心与架构", Group: "硬件层"},
				{Type: "memory_info", Name: "内存通道与容量", Command: "dmidecode -t memory | grep -E 'Size|Type|Speed' | grep -v 'No Module'", Description: "检查物理内存规格", Group: "硬件层"},
				{Type: "disk_info", Name: "存储与硬盘拓扑", Command: "lsblk -o NAME,SIZE,TYPE,MOUNTPOINT ; megacli -LDInfo -Lall -aALL 2>/dev/null || echo Passthrough", Description: "检查存储控制器与硬盘", Group: "硬件层"},
				{Type: "os_version", Name: "系统发行版与内核", Command: "cat /etc/os-release ; uname -r", Description: "检查操作系统版本", Group: "系统层"},
				{Type: "uptime", Name: "连续运行时间", Command: "uptime", Description: "检查系统运行时间", Group: "系统层"},
				{Type: "cpu_usage", Name: "CPU 负载", Command: "top -bn1 | head -15", Description: "检查 CPU 负载", Group: "资源层"},
				{Type: "memory_usage", Name: "内存使用率", Command: "free -h", Description: "检查内存占用", Group: "资源层"},
				{Type: "disk_usage", Name: "磁盘空间利用率", Command: "df -hT -x tmpfs -x devtmpfs", Description: "检查挂载分区占用", Group: "资源层"},
				{Type: "ssh_config", Name: "SSH 安全基线", Command: "grep -E '^(PermitRootLogin|PasswordAuthentication|Port)' /etc/ssh/sshd_config", Description: "检查 SSH 安全", Group: "安全层"},
				{Type: "failed_services", Name: "异常服务检查", Command: "systemctl --failed --plain --no-pager", Description: "检查崩溃服务", Group: "应用层"},
			},
			Tags: []string{"中科曙光", "Sugon", "服务器", "五层巡检"},
		},
		// ═══ 服务器 — 超微 (Supermicro) ═══
		{
			Name: "超微 Supermicro 高性能服务器巡检模版", Vendor: "supermicro", Category: "server", DeviceType: "server", OSType: "linux",
			Description: "超微 Supermicro 模块化与高密度计算服务器 5 层标准巡检指令集",
			Commands: []PresetCommand{
				{Type: "hardware", Name: "超微主板与 IPMI", Command: "dmidecode -t system | grep -E 'Manufacturer|Product Name|Serial Number' ; ipmitool mc info 2>/dev/null || echo N/A", Description: "检查超微主板型号与 IPMI 固件", Group: "硬件层"},
				{Type: "cpu_info", Name: "CPU 规格与核心", Command: "lscpu | grep -E 'Model name|Socket|Thread'", Description: "检查 CPU 规格", Group: "硬件层"},
				{Type: "memory_info", Name: "内存插槽与 ECC", Command: "dmidecode -t memory | grep -E 'Size|Type|Speed' | grep -v 'No Module'", Description: "检查内存条与 ECC", Group: "硬件层"},
				{Type: "disk_info", Name: "磁盘与 SAS/SATA 拓扑", Command: "lsblk -o NAME,SIZE,TYPE,MOUNTPOINT", Description: "检查磁盘拓扑", Group: "硬件层"},
				{Type: "os_version", Name: "系统版本与内核", Command: "cat /etc/os-release ; uname -r", Description: "检查系统版本", Group: "系统层"},
				{Type: "uptime", Name: "系统运行时间", Command: "uptime", Description: "检查系统运行时间", Group: "系统层"},
				{Type: "cpu_usage", Name: "CPU 负载", Command: "top -bn1 | head -15", Description: "检查 CPU 负载", Group: "资源层"},
				{Type: "memory_usage", Name: "内存使用率", Command: "free -h", Description: "检查内存使用情况", Group: "资源层"},
				{Type: "disk_usage", Name: "磁盘空间利用率", Command: "df -hT -x tmpfs -x devtmpfs", Description: "检查磁盘空间", Group: "资源层"},
				{Type: "ssh_config", Name: "SSH 安全基线", Command: "grep -E '^(PermitRootLogin|PasswordAuthentication|Port)' /etc/ssh/sshd_config", Description: "检查 SSH 安全", Group: "安全层"},
				{Type: "failed_services", Name: "异常服务检查", Command: "systemctl --failed --plain --no-pager", Description: "检查异常服务", Group: "应用层"},
			},
			Tags: []string{"超微", "Supermicro", "服务器", "五层巡检"},
		},
		// ═══ 服务器 — Linux 标准巡检（通用 SmartKit 五层） ═══
		{
			Name: "Linux 标准巡检模版（SmartKit 五层）", Vendor: "other", Category: "server", DeviceType: "server", OSType: "linux",
			Description: "仿照华为 SmartKit 分层巡检逻辑：硬件层→系统层→资源层→安全层→应用层",
			Commands: []PresetCommand{
				// 硬件层
				{Type: "hardware", Name: "硬件信息", Command: "dmidecode -t system 2>/dev/null | head -20 || echo N/A", Description: "查看服务器硬件型号、序列号", Group: "硬件层"},
				{Type: "cpu_info", Name: "CPU 信息", Command: "lscpu 2>/dev/null || cat /proc/cpuinfo | head -30", Description: "查看 CPU 型号、核心数、架构", Group: "硬件层"},
				{Type: "memory_info", Name: "内存硬件", Command: "dmidecode -t memory 2>/dev/null | grep -E 'Size|Speed|Type' | head -20 || free -m", Description: "查看内存条规格", Group: "硬件层"},
				{Type: "disk_info", Name: "磁盘硬件", Command: "lsblk -o NAME,SIZE,TYPE,MODEL 2>/dev/null || fdisk -l 2>/dev/null | head -30", Description: "查看磁盘设备列表", Group: "硬件层"},
				{Type: "raid_status", Name: "RAID 状态", Command: "megacli -LDInfo -Lall -aALL 2>/dev/null || cmdtool /c0 show 2>/dev/null || echo N/A", Description: "检查 RAID 阵列状态", Group: "硬件层"},
				{Type: "temperature", Name: "温度传感器", Command: "sensors 2>/dev/null || ipmitool sdr type temperature 2>/dev/null || echo N/A", Description: "查看温度传感器读数", Group: "硬件层"},
				// 系统层
				{Type: "os_version", Name: "系统版本", Command: "cat /etc/os-release 2>/dev/null | head -5", Description: "查看操作系统版本", Group: "系统层"},
				{Type: "kernel", Name: "内核版本", Command: "uname -a", Description: "查看内核版本和架构", Group: "系统层"},
				{Type: "uptime", Name: "运行时间", Command: "uptime", Description: "查看系统运行时间和负载", Group: "系统层"},
				{Type: "hostname", Name: "主机名", Command: "hostname && hostname -I 2>/dev/null", Description: "查看主机名和 IP", Group: "系统层"},
				{Type: "time", Name: "系统时间", Command: "date && timedatectl 2>/dev/null", Description: "查看系统时间和时区", Group: "系统层"},
				// 资源层
				{Type: "cpu_usage", Name: "CPU 负载", Command: "top -bn1 | head -5", Description: "查看 CPU 使用率和负载", Group: "资源层"},
				{Type: "memory_usage", Name: "内存使用", Command: "free -m", Description: "查看内存使用情况", Group: "资源层"},
				{Type: "disk_usage", Name: "磁盘使用", Command: "df -h", Description: "查看各挂载点磁盘使用率", Group: "资源层"},
				{Type: "io_stats", Name: "I/O 统计", Command: "iostat -x 1 1 2>/dev/null || echo N/A", Description: "查看磁盘 I/O 性能统计", Group: "资源层"},
				{Type: "network_stats", Name: "网络统计", Command: "ss -s 2>/dev/null || netstat -s | head -20", Description: "查看网络连接统计", Group: "资源层"},
				{Type: "process_count", Name: "进程统计", Command: "ps aux | wc -l && ps aux --sort=-%cpu | head -10", Description: "查看进程数和 CPU 占用 TOP10", Group: "资源层"},
				// 安全层
				{Type: "ssh_config", Name: "SSH 配置", Command: "grep -E 'PermitRootLogin|PasswordAuthentication|Port ' /etc/ssh/sshd_config 2>/dev/null || echo N/A", Description: "检查 SSH 安全配置", Group: "安全层"},
				{Type: "firewall", Name: "防火墙状态", Command: "systemctl is-active firewalld 2>/dev/null || ufw status 2>/dev/null || iptables -L -n 2>/dev/null | head -20 || echo N/A", Description: "检查防火墙运行状态", Group: "安全层"},
				{Type: "open_ports", Name: "开放端口", Command: "ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null || echo N/A", Description: "检查监听端口", Group: "安全层"},
				{Type: "recent_logins", Name: "最近登录", Command: "last -n 10 2>/dev/null || echo N/A", Description: "查看最近登录记录", Group: "安全层"},
				{Type: "failed_logins", Name: "失败登录", Command: "grep 'Failed password' /var/log/auth.log 2>/dev/null | tail -10 || grep 'Failed password' /var/log/secure 2>/dev/null | tail -10 || echo N/A", Description: "查看 SSH 登录失败记录", Group: "安全层"},
				{Type: "selinux", Name: "SELinux 状态", Command: "getenforce 2>/dev/null || sestatus 2>/dev/null || echo N/A", Description: "检查 SELinux 状态", Group: "安全层"},
				{Type: "ntp_sync", Name: "时间同步", Command: "chronyc tracking 2>/dev/null || ntpq -p 2>/dev/null || echo N/A", Description: "检查 NTP 时间同步状态", Group: "安全层"},
				// 应用层
				{Type: "service_status", Name: "关键服务", Command: "systemctl list-units --type=service --state=running 2>/dev/null | head -20", Description: "查看运行中的关键服务", Group: "应用层"},
				{Type: "docker_status", Name: "Docker 状态", Command: "systemctl is-active docker 2>/dev/null && docker ps --format 'table {{.Names}}\\t{{.Status}}' 2>/dev/null || echo N/A", Description: "检查 Docker 服务和容器状态", Group: "应用层"},
				{Type: "cron_jobs", Name: "定时任务", Command: "crontab -l 2>/dev/null && ls /etc/cron.d/ 2>/dev/null || echo N/A", Description: "查看定时任务配置", Group: "应用层"},
				{Type: "dmesg_errors", Name: "内核错误", Command: "dmesg --level=err,crit,alert,emerg 2>/dev/null | tail -20 || dmesg | grep -i error | tail -10 || echo N/A", Description: "查看内核日志中的错误信息", Group: "应用层"},
			},
			Tags: []string{"Linux", "SmartKit", "分层巡检", "标准巡检"},
		},
		// ═══ 服务器 — Windows 标准巡检 ═══
		{
			Name: "Windows 标准巡检模版（SmartKit 五层）", Vendor: "other", Category: "server", DeviceType: "server", OSType: "windows",
			Description: "Windows Server 分层巡检：硬件层→系统层→资源层→安全层→应用层",
			Commands: []PresetCommand{
				// 硬件层
				{Type: "hardware", Name: "硬件信息", Command: "systeminfo | findstr /C:\"System Manufacturer\" /C:\"System Model\" /C:\"System Serial\"", Description: "查看服务器硬件型号", Group: "硬件层"},
				{Type: "cpu_info", Name: "CPU 信息", Command: "wmic cpu get Name,NumberOfCores,NumberOfLogicalProcessors /format:list", Description: "查看 CPU 型号和核心数", Group: "硬件层"},
				{Type: "memory_info", Name: "内存硬件", Command: "wmic memorychip get Capacity,Speed,Manufacturer /format:list", Description: "查看内存条规格", Group: "硬件层"},
				{Type: "disk_info", Name: "磁盘硬件", Command: "wmic diskdrive get Model,Size,InterfaceType /format:list", Description: "查看磁盘设备", Group: "硬件层"},
				// 系统层
				{Type: "os_version", Name: "系统版本", Command: "systeminfo | findstr /C:\"OS Name\" /C:\"OS Version\" /C:\"System Boot Time\"", Description: "查看操作系统版本", Group: "系统层"},
				{Type: "hostname", Name: "主机名", Command: "hostname & ipconfig | findstr IPv4", Description: "查看主机名和 IP", Group: "系统层"},
				{Type: "uptime", Name: "运行时间", Command: "systeminfo | findstr /C:\"System Boot Time\"", Description: "查看系统启动时间", Group: "系统层"},
				// 资源层
				{Type: "cpu_usage", Name: "CPU 负载", Command: "wmic cpu get LoadPercentage /format:list", Description: "查看 CPU 使用率", Group: "资源层"},
				{Type: "memory_usage", Name: "内存使用", Command: "wmic OS get TotalVisibleMemorySize,FreePhysicalMemory /format:list", Description: "查看内存使用情况", Group: "资源层"},
				{Type: "disk_usage", Name: "磁盘使用", Command: "wmic logicaldisk get DeviceID,FreeSpace,Size /format:list", Description: "查看各盘符磁盘使用率", Group: "资源层"},
				{Type: "network_stats", Name: "网络统计", Command: "netstat -an | findstr LISTEN", Description: "查看网络连接状态", Group: "资源层"},
				// 安全层
				{Type: "firewall", Name: "防火墙状态", Command: "netsh advfirewall show allprofiles state", Description: "检查防火墙运行状态", Group: "安全层"},
				{Type: "open_ports", Name: "开放端口", Command: "netstat -an | findstr LISTENING", Description: "检查监听端口", Group: "安全层"},
				{Type: "recent_logins", Name: "最近登录", Command: "wevtutil qe Security /q:\"*[System[(EventID=4624)]]\" /c:10 /f:text", Description: "查看最近登录记录", Group: "安全层"},
				{Type: "failed_logins", Name: "失败登录", Command: "wevtutil qe Security /q:\"*[System[(EventID=4625)]]\" /c:10 /f:text", Description: "查看登录失败记录", Group: "安全层"},
				{Type: "ntp_sync", Name: "时间同步", Command: "w32tm /query /status", Description: "检查时间同步状态", Group: "安全层"},
				// 应用层
				{Type: "service_status", Name: "关键服务", Command: "sc query state= active", Description: "查看运行中的服务", Group: "应用层"},
				{Type: "scheduled_tasks", Name: "计划任务", Command: "schtasks /query /fo TABLE | findstr Running", Description: "查看计划任务状态", Group: "应用层"},
				{Type: "event_errors", Name: "系统错误", Command: "wevtutil qe System /q:\"*[System[(Level=1 or Level=2)]]\" /c:10 /f:text", Description: "查看系统日志错误", Group: "应用层"},
			},
			Tags: []string{"Windows", "SmartKit", "分层巡检"},
		},
		// ═══ 服务器 — Linux 安全基线巡检 ═══
		{
			Name: "Linux 安全基线巡检模版", Vendor: "other", Category: "server", DeviceType: "server", OSType: "linux",
			Description: "Linux 安全基线检查：密码策略、文件权限、审计配置、内核安全参数",
			Commands: []PresetCommand{
				{Type: "password_policy", Name: "密码策略", Command: "grep -E 'PASS_MAX_DAYS|PASS_MIN_DAYS|PASS_MIN_LEN|PASS_WARN_AGE' /etc/login.defs 2>/dev/null", Description: "检查密码过期策略", Group: "安全层"},
				{Type: "password_complexity", Name: "密码复杂度", Command: "grep pam_pwquality /etc/pam.d/system-auth 2>/dev/null || grep pam_cracklib /etc/pam.d/system-auth 2>/dev/null || echo N/A", Description: "检查密码复杂度规则", Group: "安全层"},
				{Type: "file_perms", Name: "关键文件权限", Command: "stat -c '%a %n' /etc/passwd /etc/shadow /etc/group /etc/gshadow /etc/ssh/sshd_config 2>/dev/null", Description: "检查关键系统文件权限", Group: "安全层"},
				{Type: "suid_files", Name: "SUID 文件", Command: "find / -perm -4000 -type f 2>/dev/null | head -20", Description: "检查 SUID 权限文件", Group: "安全层"},
				{Type: "auditd", Name: "审计服务", Command: "systemctl is-active auditd 2>/dev/null && auditctl -l 2>/dev/null || echo N/A", Description: "检查审计服务状态", Group: "安全层"},
				{Type: "kernel_params", Name: "内核安全参数", Command: "sysctl net.ipv4.ip_forward net.ipv4.conf.all.send_redirects net.ipv4.tcp_syncookies 2>/dev/null", Description: "检查内核安全参数", Group: "安全层"},
				{Type: "ssh_config", Name: "SSH 安全配置", Command: "grep -E 'PermitRootLogin|PasswordAuthentication|MaxAuthTries|Protocol' /etc/ssh/sshd_config 2>/dev/null", Description: "检查 SSH 安全配置", Group: "安全层"},
				{Type: "users_check", Name: "空密码用户", Command: "awk -F: '($2==\"\"){print \"Empty password: \"$1}' /etc/shadow 2>/dev/null || echo N/A", Description: "检查空密码用户", Group: "安全层"},
				{Type: "sudoers", Name: "Sudo 配置", Command: "cat /etc/sudoers 2>/dev/null | grep -v '^#' | grep -v '^$' || echo N/A", Description: "检查 sudo 权限配置", Group: "安全层"},
			},
			Tags: []string{"Linux", "安全基线", "合规检查"},
		},
	}
}

// TemplateDetail 模版详情结构
type TemplateDetail struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Vendor      string          `json:"vendor"`
	Category    string          `json:"category"`
	DeviceType  string          `json:"deviceType"`
	OSType      string          `json:"osType"`
	Description string          `json:"description"`
	Commands    []PresetCommand `json:"commands"`
	Variables   []string        `json:"variables"`
	Tags        []string        `json:"tags"`
	IsPreset    bool            `json:"isPreset"`
	Enabled     bool            `json:"enabled"`
	UsageCount  int             `json:"usageCount"`
}

// GetTemplateDetail 从数据库读取指定模版完整详情
func GetTemplateDetail(templateID string) (*TemplateDetail, error) {
	var row TemplateDetail
	var commandsJSON, varsJSON, tagsJSON string
	var isPreset, enabled int
	err := appdb.DB().QueryRow(`SELECT id, name, vendor, category, device_type, os_type, description, commands, variables, tags, is_preset, enabled, usage_count
		FROM inspection_templates WHERE id = ?`, templateID).
		Scan(&row.ID, &row.Name, &row.Vendor, &row.Category, &row.DeviceType, &row.OSType, &row.Description, &commandsJSON, &varsJSON, &tagsJSON, &isPreset, &enabled, &row.UsageCount)
	if err != nil {
		return nil, err
	}
	row.IsPreset = isPreset == 1
	row.Enabled = enabled == 1
	if commandsJSON != "" {
		_ = json.Unmarshal([]byte(commandsJSON), &row.Commands)
	}
	if varsJSON != "" {
		_ = json.Unmarshal([]byte(varsJSON), &row.Variables)
	}
	if tagsJSON != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &row.Tags)
	}
	return &row, nil
}

// CommandMap 返回 name -> command 的映射
func (t *TemplateDetail) CommandMap() map[string]string {
	m := make(map[string]string, len(t.Commands))
	for _, c := range t.Commands {
		m[c.Name] = c.Command
	}
	return m
}

// IncrementTemplateUsage 累加模版使用次数
func IncrementTemplateUsage(templateID string) {
	if templateID == "" {
		return
	}
	_, _ = appdb.DB().Exec("UPDATE inspection_templates SET usage_count = usage_count + 1 WHERE id = ?", templateID)
}

// EnsurePresets 确保预置模版已写入数据库（幂等增量插入）
func EnsurePresets() error {
	presets := presetTemplates()
	inserted := 0

	for idx, p := range presets {
		// 检查是否已存在（按 name 查重）
		var existingID string
		err := appdb.DB().QueryRow("SELECT id FROM inspection_templates WHERE name = ?", p.Name).Scan(&existingID)
		if err == nil {
			continue // 已存在，跳过
		}

		commandsJSON, _ := json.Marshal(p.Commands)
		varsJSON, _ := json.Marshal(p.Variables)
		tagsJSON, _ := json.Marshal(p.Tags)
		osType := p.OSType
		if osType == "" {
			osType = "linux"
		}

		id := fmt.Sprintf("tpl-%s-%d", p.Vendor, idx+1)
		// 如果 id 已存在则附加唯一时间戳
		var clash string
		if appdb.DB().QueryRow("SELECT id FROM inspection_templates WHERE id = ?", id).Scan(&clash) == nil {
			id = fmt.Sprintf("tpl-%s-%d-%d", p.Vendor, idx+1, time.Now().UnixNano()%10000)
		}

		_, err = appdb.DB().Exec(`INSERT INTO inspection_templates (id, name, vendor, category, device_type, os_type, description, commands, variables, tags, is_preset, enabled, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 1, 'system')`,
			id, p.Name, p.Vendor, p.Category, p.DeviceType, osType, p.Description, string(commandsJSON), string(varsJSON), string(tagsJSON))
		if err != nil {
			logger.Warnf("Failed to insert preset template '%s': %v", p.Name, err)
			continue
		}
		inserted++
	}

	if inserted > 0 {
		logger.Infof("Inspection templates: %d presets inserted/updated", inserted)
	}
	return nil
}

// GetTemplateCommands 从数据库读取指定模版的命令列表
// 返回 name → command 的映射（用于服务器巡检模版驱动）
func GetTemplateCommands(templateID string) (map[string]string, error) {
	var commandsJSON string
	err := appdb.DB().QueryRow("SELECT commands FROM inspection_templates WHERE id = ? AND enabled = 1", templateID).Scan(&commandsJSON)
	if err != nil {
		return nil, err
	}

	var cmds []PresetCommand
	if err := json.Unmarshal([]byte(commandsJSON), &cmds); err != nil {
		return nil, err
	}

	checks := make(map[string]string, len(cmds))
	for _, c := range cmds {
		checks[c.Name] = c.Command
	}
	return checks, nil
}

// FindBestTemplateForServer 根据服务器厂商和系统类型查找最匹配的服务器模版
func FindBestTemplateForServer(vendor, osType string) (*TemplateDetail, error) {
	var tplID string
	if vendor != "" && vendor != "other" {
		_ = appdb.DB().QueryRow(`
			SELECT id FROM inspection_templates 
			WHERE category = 'server' AND vendor = ? AND enabled = 1 
			ORDER BY is_preset DESC, created_at ASC LIMIT 1`, vendor).Scan(&tplID)
	}

	if tplID == "" {
		if osType == "windows" {
			_ = appdb.DB().QueryRow(`
				SELECT id FROM inspection_templates 
				WHERE category = 'server' AND os_type = 'windows' AND enabled = 1 
				ORDER BY is_preset DESC LIMIT 1`).Scan(&tplID)
		} else {
			_ = appdb.DB().QueryRow(`
				SELECT id FROM inspection_templates 
				WHERE category = 'server' AND os_type = 'linux' AND enabled = 1 AND (name LIKE '%SmartKit%' OR name LIKE '%标准%') 
				ORDER BY is_preset DESC LIMIT 1`).Scan(&tplID)
			if tplID == "" {
				_ = appdb.DB().QueryRow(`
					SELECT id FROM inspection_templates 
					WHERE category = 'server' AND enabled = 1 
					ORDER BY is_preset DESC LIMIT 1`).Scan(&tplID)
			}
		}
	}

	if tplID == "" {
		return nil, fmt.Errorf("未找到适合的服务器巡检模版")
	}

	return GetTemplateDetail(tplID)
}

// GetDefaultTemplateForServer 根据服务器 os_type 获取默认巡检模版 ID
func GetDefaultTemplateForServer(osType string) string {
	var id string
	if osType == "windows" {
		_ = appdb.DB().QueryRow("SELECT id FROM inspection_templates WHERE category = 'server' AND os_type = 'windows' AND is_preset = 1 LIMIT 1").Scan(&id)
	} else {
		_ = appdb.DB().QueryRow("SELECT id FROM inspection_templates WHERE category = 'server' AND os_type = 'linux' AND is_preset = 1 AND name LIKE '%SmartKit%' LIMIT 1").Scan(&id)
		if id == "" {
			_ = appdb.DB().QueryRow("SELECT id FROM inspection_templates WHERE category = 'server' AND os_type = 'linux' AND is_preset = 1 AND name LIKE '%标准巡检%' LIMIT 1").Scan(&id)
		}
	}
	return id
}


