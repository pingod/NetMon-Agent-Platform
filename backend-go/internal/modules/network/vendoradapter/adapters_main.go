// 厂商适配器 — 主要厂商（华为/思科/华三/锐捷/锐捷EG/中兴/飞塔/瞻博）
package vendoradapter

// ───────────────────────────────────────────────
// 华为 VRP 适配器
// ───────────────────────────────────────────────

func newHuaweiAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorHuawei,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "display cpu-usage", FallbackCommands: []string{"display cpu"}, Description: "检查设备 CPU 使用率，正常应低于 80%", ExpectedPattern: "CPU utilization", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "display memory-usage", FallbackCommands: []string{"display memory", "display memory-threshold"}, Description: "检查设备内存使用情况，正常应低于 85%", ExpectedPattern: "Memory", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "display interface brief", FallbackCommands: []string{"display ip interface brief"}, Description: "检查所有接口物理状态和协议状态", ExpectedPattern: "PHY|Protocol"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "display version", Description: "检查设备型号、软件版本和运行时间"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "display ip routing-table", FallbackCommands: []string{"display ip routing-table statistics"}, Description: "检查路由表状态和路由数量"},
			InspectLog:            {Type: InspectLog, Name: "日志缓冲区", Command: "display logbuffer", FallbackCommands: []string{"display trapbuffer", "display syslog"}, Description: "检查最近的系统日志和告警信息"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "环境状态", Command: "display temperature", FallbackCommands: []string{"display device temperature", "display environment"}, Description: "检查设备温度和电压状态", Models: []string{"NE", "CE", "S5700", "S6700"}},
			InspectPower:          {Type: InspectPower, Name: "电源状态", Command: "display power", FallbackCommands: []string{"display device power", "display system-power"}, Description: "检查电源模块状态", Models: []string{"NE", "CE", "S5700", "S6700"}},
			InspectFan:            {Type: InspectFan, Name: "风扇状态", Command: "display fan", FallbackCommands: []string{"display device fan"}, Description: "检查风扇模块运行状态", Models: []string{"NE", "CE", "S5700", "S6700"}},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "display stp", FallbackCommands: []string{"display stp brief", "display spanning-tree"}, Description: "检查生成树协议状态和端口角色"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 信息", Command: "display vlan", FallbackCommands: []string{"display vlan summary"}, Description: "检查 VLAN 配置和端口成员"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "display arp", Description: "检查 ARP 表项数量和状态"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 地址表", Command: "display mac-address", FallbackCommands: []string{"display mac-address summary"}, Description: "检查 MAC 地址表项"},
			InspectOptic:          {Type: InspectOptic, Name: "光模块信息", Command: "display optical-info", FallbackCommands: []string{"display transceiver verbose", "display interface transceiver"}, Description: "检查光模块收发光功率和温度", DeviceTypes: []DeviceType{DeviceSwitch}},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "display lldp neighbor brief", FallbackCommands: []string{"display cdp neighbor"}, Description: "查看 LLDP 邻居发现信息"},
			InspectSecurityPolicy: {Type: InspectSecurityPolicy, Name: "安全策略", Command: "display security-policy rule all", FallbackCommands: []string{"display acl all", "display firewall rule"}, Description: "查看防火墙安全策略配置", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}},
			InspectNAT:            {Type: InspectNAT, Name: "NAT 转换", Command: "display nat session summary", FallbackCommands: []string{"display nat outbound", "display nat server"}, Description: "查看 NAT 会话统计和映射", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter, DeviceGateway}},
			InspectSession:        {Type: InspectSession, Name: "会话统计", Command: "display firewall session table", FallbackCommands: []string{"display session statistics"}, Description: "查看防火墙会话状态", DeviceTypes: []DeviceType{DeviceFirewall}},
			InspectVPN:            {Type: InspectVPN, Name: "VPN 隧道", Command: "display ike sa", FallbackCommands: []string{"display ipsec sa", "display ipsec tunnel"}, Description: "查看 IPSec VPN 隧道状态", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter, DeviceGateway}},
			InspectWLAN:           {Type: InspectWLAN, Name: "无线信息", Command: "display wlan ap all", FallbackCommands: []string{"display wlan client", "display wlan radio"}, Description: "查看无线接入点和客户端状态", DeviceTypes: []DeviceType{DeviceWLC, DeviceAP}},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "display ip pool", FallbackCommands: []string{"display dhcp server statistics"}, Description: "查看 DHCP 地址池使用情况", DeviceTypes: []DeviceType{DeviceRouter, DeviceGateway, DeviceSwitch}},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "display dns server", Description: "查看 DNS 服务器配置"},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "display bgp peer", FallbackCommands: []string{"display bgp peer verbose"}, Description: "查看 BGP 邻居状态", DeviceTypes: []DeviceType{DeviceRouter, DeviceFirewall}},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "display ospf peer", FallbackCommands: []string{"display ospf interface", "display ospf routing"}, Description: "查看 OSPF 邻居状态", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "display ntp status", FallbackCommands: []string{"display ntp-service status"}, Description: "查看 NTP 同步状态"},
			InspectLicense:        {Type: InspectLicense, Name: "License 有效期", Command: "display license", Description: "查看 License 有效期和功能授权"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置快照", Command: "display current-configuration | include sysname", FallbackCommands: []string{"display saved-configuration last-time"}, Description: "查看运行配置最后保存时间和摘要"},
		},
	}
}

// ───────────────────────────────────────────────
// 思科 IOS/IOS-XE/NX-OS 适配器
// ───────────────────────────────────────────────

func newCiscoAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorCisco,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show processes cpu | include CPU utilization", Description: "检查设备 CPU 使用率，正常应低于 80%", ExpectedPattern: "CPU utilization", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show memory statistics", Description: "检查设备内存使用情况，正常应低于 85%", ExpectedPattern: "Pool", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show ip interface brief", Description: "检查所有接口物理状态和协议状态", ExpectedPattern: "Interface|Status"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show version", Description: "检查设备型号、软件版本和运行时间"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show ip route", Description: "检查路由表状态和路由数量"},
			InspectLog:            {Type: InspectLog, Name: "日志缓冲区", Command: "show logging", Description: "检查最近的系统日志和告警信息"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "环境状态", Command: "show environment all", Description: "检查设备温度和电压状态"},
			InspectPower:          {Type: InspectPower, Name: "电源状态", Command: "show power", Description: "检查电源模块状态"},
			InspectFan:            {Type: InspectFan, Name: "风扇状态", Command: "show environment fan", Description: "检查风扇模块运行状态"},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "show spanning-tree brief", Description: "检查生成树协议状态和端口角色"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 信息", Command: "show vlan brief", Description: "检查 VLAN 配置和端口成员"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "show ip arp", Description: "检查 ARP 表项数量和状态"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 地址表", Command: "show mac address-table", Description: "检查 MAC 地址表项"},
			InspectOptic:          {Type: InspectOptic, Name: "光模块信息", Command: "show interfaces transceiver detail", FallbackCommands: []string{"show interfaces transceiver"}, Description: "检查光模块收发光功率和温度", DeviceTypes: []DeviceType{DeviceSwitch}},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp neighbors detail", FallbackCommands: []string{"show cdp neighbors detail"}, Description: "查看 LLDP/CDP 邻居发现信息"},
			InspectSecurityPolicy: {Type: InspectSecurityPolicy, Name: "安全策略", Command: "show access-list", FallbackCommands: []string{"show ip access-list", "show running-config | section ip access-list"}, Description: "查看 ACL/安全策略配置", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter}},
			InspectNAT:            {Type: InspectNAT, Name: "NAT 转换", Command: "show ip nat translations", FallbackCommands: []string{"show running-config | include nat"}, Description: "查看 NAT 转换表", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter, DeviceGateway}},
			InspectVPN:            {Type: InspectVPN, Name: "VPN 隧道", Command: "show crypto isakmp sa", FallbackCommands: []string{"show crypto ipsec sa", "show vpn-sessiondb"}, Description: "查看 IPSec VPN 隧道状态", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter, DeviceGateway}},
			InspectWLAN:           {Type: InspectWLAN, Name: "无线信息", Command: "show ap summary", FallbackCommands: []string{"show wireless client summary"}, Description: "查看无线接入点和客户端", DeviceTypes: []DeviceType{DeviceWLC, DeviceAP}},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "show ip dhcp binding", FallbackCommands: []string{"show ip dhcp pool"}, Description: "查看 DHCP 地址池使用情况", DeviceTypes: []DeviceType{DeviceRouter, DeviceGateway, DeviceSwitch}},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "show running-config | include name-server", FallbackCommands: []string{"show hosts"}, Description: "查看 DNS 服务器配置"},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "show bgp summary", FallbackCommands: []string{"show ip bgp summary"}, Description: "查看 BGP 邻居状态", DeviceTypes: []DeviceType{DeviceRouter, DeviceFirewall}},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "show ip ospf neighbor", Description: "查看 OSPF 邻居状态", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp associations", FallbackCommands: []string{"show ntp status"}, Description: "查看 NTP 同步状态"},
			InspectLicense:        {Type: InspectLicense, Name: "License 有效期", Command: "show license summary", FallbackCommands: []string{"show license usage"}, Description: "查看 License 有效期和功能授权"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置快照", Command: "show running-config | include hostname", Description: "查看运行配置摘要快照"},
			InspectSession:        {Type: InspectSession, Name: "防火墙会话", Command: "show conn count", Description: "查看防火墙并发连接数", DeviceTypes: []DeviceType{DeviceFirewall}},
		},
	}
}

// ───────────────────────────────────────────────
// 华三 Comware 适配器
// ───────────────────────────────────────────────

func newH3cAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorH3c,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "display cpu-usage", Description: "检查设备 CPU 使用率", ExpectedPattern: "CPU utilization", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "display memory-usage", FallbackCommands: []string{"display memory"}, Description: "检查设备内存使用情况", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "display interface brief", Description: "检查所有接口物理状态", ExpectedPattern: "Link|Protocol"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "display version", Description: "检查设备型号、软件版本和运行时间"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "display ip routing-table", Description: "检查路由表"},
			InspectLog:            {Type: InspectLog, Name: "日志缓冲区", Command: "display logbuffer", FallbackCommands: []string{"display trapbuffer"}, Description: "检查最近的系统日志"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "环境状态", Command: "display environment", FallbackCommands: []string{"display temperature"}, Description: "检查温度和电压"},
			InspectPower:          {Type: InspectPower, Name: "电源状态", Command: "display power", FallbackCommands: []string{"display device power"}, Description: "检查电源状态"},
			InspectFan:            {Type: InspectFan, Name: "风扇状态", Command: "display fan", FallbackCommands: []string{"display device fan"}, Description: "检查风扇状态"},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "display stp", FallbackCommands: []string{"display stp brief"}, Description: "检查 STP 状态"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 信息", Command: "display vlan", FallbackCommands: []string{"display vlan all"}, Description: "检查 VLAN 配置"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "display arp", Description: "检查 ARP 表项"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 地址表", Command: "display mac-address", FallbackCommands: []string{"display mac-address statistics"}, Description: "检查 MAC 地址表"},
			InspectOptic:          {Type: InspectOptic, Name: "光模块信息", Command: "display transceiver verbose", FallbackCommands: []string{"display optical-info"}, Description: "检查光模块收发光功率", DeviceTypes: []DeviceType{DeviceSwitch}},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "display lldp neighbor brief", Description: "查看 LLDP 邻居发现信息"},
			InspectSecurityPolicy: {Type: InspectSecurityPolicy, Name: "安全策略", Command: "display security-policy rule all", FallbackCommands: []string{"display acl all"}, Description: "查看安全策略配置", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}},
			InspectNAT:            {Type: InspectNAT, Name: "NAT 转换", Command: "display nat session summary", FallbackCommands: []string{"display nat outbound"}, Description: "查看 NAT 会话", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter, DeviceGateway}},
			InspectSession:        {Type: InspectSession, Name: "会话统计", Command: "display session statistics", Description: "查看会话状态", DeviceTypes: []DeviceType{DeviceFirewall}},
			InspectVPN:            {Type: InspectVPN, Name: "VPN 隧道", Command: "display ike sa", FallbackCommands: []string{"display ipsec sa"}, Description: "查看 VPN 隧道状态", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter, DeviceGateway}},
			InspectWLAN:           {Type: InspectWLAN, Name: "无线信息", Command: "display wlan ap all", FallbackCommands: []string{"display wlan client"}, Description: "查看无线信息", DeviceTypes: []DeviceType{DeviceWLC, DeviceAP}},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "display dhcp server statistics", FallbackCommands: []string{"display ip pool"}, Description: "查看 DHCP 池", DeviceTypes: []DeviceType{DeviceRouter, DeviceGateway, DeviceSwitch}},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "display dns server", Description: "查看 DNS 配置"},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "display bgp peer", Description: "查看 BGP 邻居", DeviceTypes: []DeviceType{DeviceRouter, DeviceFirewall}},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "display ospf peer", Description: "查看 OSPF 邻居", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "display ntp status", FallbackCommands: []string{"display ntp-service status"}, Description: "查看 NTP 同步状态"},
			InspectLicense:        {Type: InspectLicense, Name: "License 有效期", Command: "display license", Description: "查看 License 信息"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置快照", Command: "display current-configuration | include sysname", Description: "查看配置摘要"},
		},
	}
}

// ───────────────────────────────────────────────
// 锐捷（传统命令集）
// ───────────────────────────────────────────────

func newRuijieAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorRuijie,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show cpu", Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show memory", Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show interfaces status", Description: "检查所有接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show version", Description: "检查系统版本"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show ip route", Description: "检查路由表"},
			InspectLog:            {Type: InspectLog, Name: "日志缓冲区", Command: "show logging", Description: "检查日志"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "环境状态", Command: "show environment", Description: "检查环境状态"},
			InspectPower:          {Type: InspectPower, Name: "电源状态", Command: "show power", Description: "检查电源状态"},
			InspectFan:            {Type: InspectFan, Name: "风扇状态", Command: "show fan", Description: "检查风扇状态"},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "show spanning-tree", Description: "检查 STP 状态"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 信息", Command: "show vlan", Description: "检查 VLAN 配置"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "show arp", Description: "检查 ARP 表"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 地址表", Command: "show mac-address-table", Description: "检查 MAC 表"},
			InspectOptic:          {Type: InspectOptic, Name: "光模块信息", Command: "show interfaces transceiver", DeviceTypes: []DeviceType{DeviceSwitch}, Description: "检查光模块"},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp neighbors", Description: "查看 LLDP 邻居"},
			InspectNAT:            {Type: InspectNAT, Name: "NAT 转换", Command: "show ip nat translations", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter, DeviceGateway}, Description: "查看 NAT 表"},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "show ip dhcp binding", DeviceTypes: []DeviceType{DeviceRouter, DeviceGateway, DeviceSwitch}, Description: "查看 DHCP 池"},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "show ip bgp summary", DeviceTypes: []DeviceType{DeviceRouter, DeviceFirewall}, Description: "查看 BGP 邻居"},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "show ip ospf neighbor", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 OSPF 邻居"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp status", Description: "查看 NTP 状态"},
			InspectSecurityPolicy: {Type: InspectSecurityPolicy, Name: "ACL 策略", Command: "show access-lists", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter}, Description: "查看 ACL 策略"},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "show running-config | include nameserver", Description: "查看 DNS 配置"},
			InspectVPN:            {Type: InspectVPN, Name: "VPN 隧道", Command: "show crypto isakmp sa", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter, DeviceGateway}, Description: "查看 VPN 隧道"},
			InspectWLAN:           {Type: InspectWLAN, Name: "无线信息", Command: "show wlan ap summary", DeviceTypes: []DeviceType{DeviceWLC, DeviceAP}, Description: "查看无线信息"},
			InspectSession:        {Type: InspectSession, Name: "会话统计", Command: "show conn count", DeviceTypes: []DeviceType{DeviceFirewall}, Description: "查看会话数"},
			InspectLicense:        {Type: InspectLicense, Name: "License 信息", Command: "show license", Description: "查看 License 信息"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置摘要", Command: "show running-config | include hostname", Description: "查看配置摘要"},
		},
	}
}

// ───────────────────────────────────────────────
// 锐捷 EG 出口网关
// ───────────────────────────────────────────────

func newRuijieEGAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorRuijieEG,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show cpu", Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show memory", Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show interface brief", Description: "检查接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show version", Description: "检查系统版本"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show ip route", Description: "查看路由表"},
			InspectLog:            {Type: InspectLog, Name: "系统日志", Command: "show log", Description: "查看系统日志"},
			InspectNAT:            {Type: InspectNAT, Name: "NAT 转换", Command: "show ip nat translations", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway, DeviceRouter}, Description: "查看 NAT 转换表"},
			InspectSecurityPolicy: {Type: InspectSecurityPolicy, Name: "安全策略", Command: "show security-policy", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}, Description: "查看安全策略"},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "show ip dhcp server statistics", DeviceTypes: []DeviceType{DeviceRouter, DeviceGateway}, Description: "查看 DHCP 状态"},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "show dns server", Description: "查看 DNS 服务器"},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp neighbors", Description: "查看 LLDP 邻居"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp status", Description: "查看 NTP 状态"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置摘要", Command: "show running-config | include hostname", Description: "查看配置摘要"},
			InspectSession:        {Type: InspectSession, Name: "连接数统计", Command: "show session statistics", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}, Description: "查看并发连接统计"},
			InspectVPN:            {Type: InspectVPN, Name: "VPN 隧道", Command: "show ike sa", FallbackCommands: []string{"show ipsec sa"}, DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}, Description: "查看 VPN 隧道"},
		},
	}
}

// ───────────────────────────────────────────────
// 中兴 ZTE ZXR10
// ───────────────────────────────────────────────

func newZteAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorZte,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show cpu", Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show memory", Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show interface brief", Description: "检查接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show version", Description: "检查系统版本"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show ip route", Description: "检查路由表"},
			InspectLog:            {Type: InspectLog, Name: "日志缓冲区", Command: "show log", Description: "检查日志"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "环境状态", Command: "show environment", Description: "检查环境状态"},
			InspectPower:          {Type: InspectPower, Name: "电源状态", Command: "show power", Description: "检查电源状态"},
			InspectFan:            {Type: InspectFan, Name: "风扇状态", Command: "show fan", Description: "检查风扇状态"},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "show spanning-tree", Description: "检查 STP 状态"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 信息", Command: "show vlan", Description: "检查 VLAN"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "show arp", Description: "检查 ARP 表"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 地址表", Command: "show mac-address-table", Description: "检查 MAC 表"},
			InspectOptic:          {Type: InspectOptic, Name: "光模块信息", Command: "show transceiver", DeviceTypes: []DeviceType{DeviceSwitch}, Description: "检查光模块"},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp neighbors", Description: "查看 LLDP 邻居"},
			InspectNAT:            {Type: InspectNAT, Name: "NAT 转换", Command: "show ip nat translations", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter}, Description: "查看 NAT 表"},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "show bgp summary", DeviceTypes: []DeviceType{DeviceRouter}, Description: "查看 BGP"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp status", Description: "查看 NTP"},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "show ip dhcp binding", DeviceTypes: []DeviceType{DeviceRouter}, Description: "查看 DHCP"},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "show dns server", Description: "查看 DNS"},
			InspectSession:        {Type: InspectSession, Name: "会话统计", Command: "show session statistics", Description: "查看会话统计", DeviceTypes: []DeviceType{DeviceFirewall}},
			InspectWLAN:           {Type: InspectWLAN, Name: "无线信息", Command: "show wlan ap summary", DeviceTypes: []DeviceType{DeviceWLC, DeviceAP}, Description: "查看无线 AP 信息"},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "show ip ospf neighbor", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 OSPF 邻居"},
			InspectSecurityPolicy: {Type: InspectSecurityPolicy, Name: "ACL 策略", Command: "show access-list", DeviceTypes: []DeviceType{DeviceFirewall}, Description: "查看 ACL"},
			InspectVPN:            {Type: InspectVPN, Name: "VPN 隧道", Command: "show ipsec sa", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter}, Description: "查看 VPN"},
			InspectLicense:        {Type: InspectLicense, Name: "License 信息", Command: "show license", Description: "查看 License"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置摘要", Command: "show running-config | include sysname", Description: "查看配置摘要"},
		},
	}
}

// ───────────────────────────────────────────────
// Fortinet FortiGate 防火墙
// ───────────────────────────────────────────────

func newFortinetAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorFortinet,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "get system performance status", FallbackCommands: []string{"get system perf"}, Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "get system performance status", Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show system interface physical", FallbackCommands: []string{"get system interface"}, Description: "检查所有接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "get system status", Description: "检查固件版本和运行时间"},
			InspectLog:            {Type: InspectLog, Name: "最近日志", Command: "execute log filter category event\nexecute log display", Description: "查看最近系统日志"},
			InspectSecurityPolicy: {Type: InspectSecurityPolicy, Name: "防火墙策略", Command: "show firewall policy", Description: "查看所有防火墙策略", DeviceTypes: []DeviceType{DeviceFirewall}},
			InspectNAT:            {Type: InspectNAT, Name: "NAT 策略", Command: "show firewall ippool", FallbackCommands: []string{"show firewall central-snat-map"}, Description: "查看 NAT 策略", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}},
			InspectSession:        {Type: InspectSession, Name: "会话统计", Command: "get system sessions", FallbackCommands: []string{"diagnose sys session count"}, Description: "查看并发会话统计", DeviceTypes: []DeviceType{DeviceFirewall}},
			InspectVPN:            {Type: InspectVPN, Name: "VPN 隧道", Command: "diagnose vpn tunnel list", FallbackCommands: []string{"get vpn ipsec tunnel details"}, Description: "查看 IPSec VPN 隧道", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "get router info routing-table all", Description: "查看路由表", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter}},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "get lldp neighbors", Description: "查看 LLDP 邻居"},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "get router info bgp summary", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter}, Description: "查看 BGP 邻居"},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "get router info ospf neighbor", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter}, Description: "查看 OSPF 邻居"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show system ntp", FallbackCommands: []string{"get system ntp status"}, Description: "查看 NTP 状态"},
			InspectLicense:        {Type: InspectLicense, Name: "License / 订阅", Command: "get system license", FallbackCommands: []string{"show system fortiguard"}, Description: "检查 License 到期日和 FortiGuard 订阅"},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "show system dhcp server", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}, Description: "查看 DHCP 配置"},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "show system dns", Description: "查看 DNS 服务器配置"},
			InspectWLAN:           {Type: InspectWLAN, Name: "无线信息", Command: "get wireless ap list", FallbackCommands: []string{"get wireless client list"}, Description: "查看无线接入点和客户端", DeviceTypes: []DeviceType{DeviceWLC, DeviceAP}},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置快照", Command: "execute backup config checksum", Description: "查看配置校验和"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "硬件状态", Command: "get system hardware status", Description: "查看硬件温度风扇电源"},
			InspectPower:          {Type: InspectPower, Name: "电源状态", Command: "get system hardware status", Description: "查看电源模块状态"},
			InspectFan:            {Type: InspectFan, Name: "风扇状态", Command: "get system hardware status", Description: "查看风扇状态"},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "diagnose system bridge mac", Description: "查看生成树状态"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 接口", Command: "show system interface | grep -E \"vlan|Vlan\"", Description: "查看 VLAN 接口"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "get system arp", Description: "查看 ARP 表"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 地址表", Command: "diagnose device-switch mac-address", Description: "查看 MAC 地址表"},
			InspectOptic:          {Type: InspectOptic, Name: "光模块信息", Command: "diagnose system transceiver", DeviceTypes: []DeviceType{DeviceSwitch}, Description: "查看光模块参数"},
		},
	}
}

// ───────────────────────────────────────────────
// Juniper JunOS
// ───────────────────────────────────────────────

func newJuniperAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorJuniper,
		enablePassword: false, // JunOS 通常用 SSH key
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show system processes extensive | match idle", FallbackCommands: []string{"show chassis routing-engine"}, Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show system memory", FallbackCommands: []string{"show chassis routing-engine"}, Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show interfaces terse", Description: "检查所有接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show version", Description: "检查 JunOS 版本和运行时间"},
			InspectLog:            {Type: InspectLog, Name: "最近日志", Command: "show log messages | last 30", Description: "查看最近系统日志"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show route summary", FallbackCommands: []string{"show route protocol static"}, Description: "查看路由表概要"},
			InspectSecurityPolicy: {Type: InspectSecurityPolicy, Name: "安全策略", Command: "show security policies", Description: "查看安全策略", DeviceTypes: []DeviceType{DeviceFirewall}},
			InspectNAT:            {Type: InspectNAT, Name: "NAT 策略", Command: "show security nat source rule", FallbackCommands: []string{"show security nat dest rule"}, Description: "查看 NAT 规则", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}},
			InspectSession:        {Type: InspectSession, Name: "会话统计", Command: "show security flow session summary", Description: "查看会话统计", DeviceTypes: []DeviceType{DeviceFirewall}},
			InspectVPN:            {Type: InspectVPN, Name: "VPN 隧道", Command: "show security ipsec security-associations", FallbackCommands: []string{"show security ike security-associations"}, Description: "查看 IPSec VPN 隧道", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "show bgp summary", DeviceTypes: []DeviceType{DeviceRouter, DeviceFirewall}, Description: "查看 BGP 邻居"},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "show ospf neighbor", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 OSPF 邻居"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp associations", Description: "查看 NTP 同步状态"},
			InspectLicense:        {Type: InspectLicense, Name: "License 信息", Command: "show system license", Description: "查看 License 信息"},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp neighbors", Description: "查看 LLDP 邻居"},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "show dhcp server binding", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 DHCP 绑定信息"},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "show system name-server", Description: "查看 DNS 服务器配置"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "环境状态", Command: "show chassis hardware", FallbackCommands: []string{"show chassis environment"}, Description: "查看机框硬件信息"},
			InspectPower:          {Type: InspectPower, Name: "电源状态", Command: "show chassis power", Description: "查看电源状态"},
			InspectFan:            {Type: InspectFan, Name: "风扇状态", Command: "show chassis fan", Description: "查看风扇状态"},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "show spanning-tree interface", Description: "查看 STP 状态"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 信息", Command: "show vlans", Description: "查看 VLAN 配置"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "show arp", Description: "查看 ARP 表"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 地址表", Command: "show ethernet-switching table", DeviceTypes: []DeviceType{DeviceSwitch}, Description: "查看 MAC 地址表"},
			InspectOptic:          {Type: InspectOptic, Name: "光模块信息", Command: "show interfaces diagnostics optics", DeviceTypes: []DeviceType{DeviceSwitch}, Description: "查看光模块参数"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置签名", Command: "show configuration checksum", Description: "查看运行配置 MD5 签名"},
		},
	}
}
