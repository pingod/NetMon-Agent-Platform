// 厂商适配器 — 其余厂商（PaloAlto/Arista/HPE/Mikrotik/Ubiquiti/Dell/TP-Link/F5）
package vendoradapter

// ───────────────────────────────────────────────
// Palo Alto PAN-OS 防火墙
// ───────────────────────────────────────────────

func newPaloAltoAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorPaloAlto,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show system resources | match cpu", Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show system resources | match mem", FallbackCommands: []string{"show system resources"}, Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show interface all", Description: "检查所有接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show system info | match sw-version", FallbackCommands: []string{"show system info"}, Description: "查看 PAN-OS 版本"},
			InspectLog:            {Type: InspectLog, Name: "最近日志", Command: "show log system direction equal forward | tail 30", Description: "查看最近系统日志"},
			InspectSecurityPolicy: {Type: InspectSecurityPolicy, Name: "安全策略", Command: "show running security-policy", Description: "查看所有安全策略", DeviceTypes: []DeviceType{DeviceFirewall}},
			InspectNAT:            {Type: InspectNAT, Name: "NAT 策略", Command: "show running nat-policy", Description: "查看 NAT 策略", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}},
			InspectSession:        {Type: InspectSession, Name: "会话统计", Command: "show session info", FallbackCommands: []string{"show session summary"}, Description: "查看会话统计", DeviceTypes: []DeviceType{DeviceFirewall}},
			InspectVPN:            {Type: InspectVPN, Name: "VPN 隧道", Command: "show vpn ipsec tunnel", Description: "查看 IPSec VPN 隧道", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show routing route", Description: "查看路由表", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter}},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "show routing protocol bgp summary", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter}, Description: "查看 BGP 邻居"},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "show routing protocol ospf neighbor", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter}, Description: "查看 OSPF 邻居"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp status", Description: "查看 NTP 同步"},
			InspectLicense:        {Type: InspectLicense, Name: "License 状态", Command: "show license info", Description: "查看 License 到期日"},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "show dhcp server lease", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway, DeviceRouter}, Description: "查看 DHCP 租约"},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "show dns proxy config", Description: "查看 DNS 代理配置"},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp neighbors", Description: "查看 LLDP 邻居"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "硬件状态", Command: "show system environment", Description: "查看温度/风扇"},
			InspectPower:          {Type: InspectPower, Name: "电源状态", Command: "show system environment power", Description: "查看电源模块状态"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置快照", Command: "show config diff | match serial", Description: "查看配置摘要"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "show arp all", Description: "查看 ARP 表"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 接口", Command: "show interface all | match vlan", DeviceTypes: []DeviceType{DeviceSwitch}, Description: "查看 VLAN 接口"},
		},
	}
}

// ───────────────────────────────────────────────
// Arista EOS
// ───────────────────────────────────────────────

func newAristaAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorArista,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show processes top | grep Cpu", FallbackCommands: []string{"show system resources"}, Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show system memory", Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show interfaces status", Description: "检查所有接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show version", Description: "检查 EOS 版本"},
			InspectLog:            {Type: InspectLog, Name: "最近日志", Command: "show logging last 30", Description: "查看最近日志"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show ip route summary", Description: "查看路由表概要"},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "show bgp summary", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 BGP 邻居"},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "show ip ospf neighbor", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 OSPF 邻居"},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "show spanning-tree", Description: "查看 STP 状态"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 信息", Command: "show vlan", Description: "查看 VLAN 配置"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "show ip arp", Description: "查看 ARP 表"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 地址表", Command: "show mac address-table", Description: "查看 MAC 表"},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp neighbors detail", Description: "查看 LLDP 邻居"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp status", Description: "查看 NTP 同步"},
			InspectOptic:          {Type: InspectOptic, Name: "光模块信息", Command: "show interfaces transceiver", DeviceTypes: []DeviceType{DeviceSwitch}, Description: "查看光模块参数"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "环境状态", Command: "show system environment", Description: "查看温度/电源/风扇"},
			InspectPower:          {Type: InspectPower, Name: "电源状态", Command: "show system environment power", Description: "查看电源状态"},
			InspectFan:            {Type: InspectFan, Name: "风扇状态", Command: "show system environment fan", Description: "查看风扇状态"},
			InspectLicense:        {Type: InspectLicense, Name: "License 信息", Command: "show license", Description: "查看 License"},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "show hosts", Description: "查看 DNS 配置"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置摘要", Command: "show running-config | grep hostname", Description: "查看配置摘要"},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "show ip dhcp server leases summary", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 DHCP 租约"},
		},
	}
}

// ───────────────────────────────────────────────
// HPE/Aruba ProVision/Comware
// ───────────────────────────────────────────────

func newHpeAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorHpe,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show system cpu", Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show system memory", Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show interfaces brief", Description: "检查所有接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show version", Description: "检查系统版本"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show ip route", Description: "查看路由表"},
			InspectLog:            {Type: InspectLog, Name: "最近日志", Command: "show logging -r", Description: "查看最近日志"},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "show spanning-tree", Description: "查看 STP 状态"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 信息", Command: "show vlan", Description: "查看 VLAN 配置"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "show arp", Description: "查看 ARP 表"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 地址表", Command: "show mac-address", Description: "查看 MAC 表"},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp info remote-device", Description: "查看 LLDP 邻居"},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "show bgp summary", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 BGP 邻居"},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "show ip ospf neighbor", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 OSPF 邻居"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp", Description: "查看 NTP 状态"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "环境状态", Command: "show system environment", Description: "查看温度/电源/风扇"},
			InspectPower:          {Type: InspectPower, Name: "电源状态", Command: "show system power-supply", Description: "查看电源状态"},
			InspectFan:            {Type: InspectFan, Name: "风扇状态", Command: "show system fan", Description: "查看风扇状态"},
			InspectOptic:          {Type: InspectOptic, Name: "光模块信息", Command: "show interfaces transceiver", DeviceTypes: []DeviceType{DeviceSwitch}, Description: "查看光模块参数"},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "show ip dns", Description: "查看 DNS 配置"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置快照", Command: "show running-config | include hostname", Description: "查看配置摘要"},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "show ip dhcp server", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 DHCP 池"},
			InspectLicense:        {Type: InspectLicense, Name: "License 信息", Command: "show license", Description: "查看 License"},
		},
	}
}

// ───────────────────────────────────────────────
// MikroTik RouterOS
// ───────────────────────────────────────────────

func newMikrotikAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorMikrotik,
		enablePassword: false, // RouterOS 不使用 enable 模式
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "/system resource print", Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "/system resource print", Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "/interface print detail", Description: "检查所有接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "/system resource print", Description: "检查 RouterOS 版本"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "/ip route print detail", Description: "查看路由表"},
			InspectLog:            {Type: InspectLog, Name: "最近日志", Command: "/log print where topics=critical,warning,error", Description: "查看最近重要日志"},
			InspectNAT:            {Type: InspectNAT, Name: "NAT 规则", Command: "/ip firewall nat print", Description: "查看 NAT 规则", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter, DeviceGateway}},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "/ip neighbor print", Description: "查看邻居发现信息"},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "/routing bgp peer print", DeviceTypes: []DeviceType{DeviceRouter}, Description: "查看 BGP 邻居"},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "/routing ospf neighbor print", DeviceTypes: []DeviceType{DeviceRouter}, Description: "查看 OSPF 邻居"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "/system ntp client print", Description: "查看 NTP 客户端状态"},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "/ip dhcp-server lease print", DeviceTypes: []DeviceType{DeviceRouter, DeviceGateway}, Description: "查看 DHCP 租约"},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "/ip dns print", Description: "查看 DNS 配置"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 接口", Command: "/interface vlan print", Description: "查看 VLAN 接口"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "/ip arp print", Description: "查看 ARP 表"},
			InspectWLAN:           {Type: InspectWLAN, Name: "无线状态", Command: "/interface wireless registration-table print", DeviceTypes: []DeviceType{DeviceAP, DeviceWLC}, Description: "查看无线客户端"},
			InspectLicense:        {Type: InspectLicense, Name: "License 级别", Command: "/system license print", Description: "查看 License 级别和到期时间"},
			InspectSecurityPolicy: {Type: InspectSecurityPolicy, Name: "防火墙规则", Command: "/ip firewall filter print", Description: "查看防火墙过滤规则", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter, DeviceGateway}},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置快照", Command: "/export terse | include /system identity", Description: "查看配置摘要"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "硬件状态", Command: "/system health print", Description: "查看温度/电压/风扇"},
		},
	}
}

// ───────────────────────────────────────────────
// Ubiquiti EdgeSwitch / UniFi
// ───────────────────────────────────────────────

func newUbiquitiAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorUbiquiti,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show system processes cpu", Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show system memory", Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show interfaces", Description: "检查所有接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show version", Description: "检查 EdgeOS 版本"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show ip route", Description: "查看路由表"},
			InspectLog:            {Type: InspectLog, Name: "最近日志", Command: "show log | tail -30", Description: "查看最近日志"},
			InspectNAT:            {Type: InspectNAT, Name: "NAT 规则", Command: "show nat", Description: "查看 NAT 规则", DeviceTypes: []DeviceType{DeviceFirewall, DeviceRouter, DeviceGateway}},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp neighbors", Description: "查看 LLDP 邻居"},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "show spanning-tree", Description: "查看 STP 状态"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 配置", Command: "show vlan", Description: "查看 VLAN 配置"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "show arp", Description: "查看 ARP 表"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 表", Command: "show mac-address-table", Description: "查看 MAC 表"},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "show dhcp leases", DeviceTypes: []DeviceType{DeviceRouter, DeviceGateway}, Description: "查看 DHCP 租约"},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "show dns", Description: "查看 DNS 配置"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp", Description: "查看 NTP 状态"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置摘要", Command: "show configuration | head -5", Description: "查看配置摘要"},
		},
	}
}

// ───────────────────────────────────────────────
// Dell PowerSwitch / N-series
// ───────────────────────────────────────────────

func newDellAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorDell,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show system resources cpu", Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show system resources memory", Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show interfaces status", Description: "检查所有接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show version", Description: "检查 Dell OS 版本"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show ip route", Description: "查看路由表"},
			InspectLog:            {Type: InspectLog, Name: "日志", Command: "show logging last 30", Description: "查看最近日志"},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "show spanning-tree", Description: "查看 STP 状态"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 信息", Command: "show vlan", Description: "查看 VLAN 配置"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "show arp", Description: "查看 ARP 表"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 地址表", Command: "show mac address-table", Description: "查看 MAC 表"},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp neighbors", Description: "查看 LLDP 邻居"},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "show ip bgp summary", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 BGP 邻居"},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "show ip ospf neighbor", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 OSPF 邻居"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp status", Description: "查看 NTP 状态"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "环境状态", Command: "show system environment", Description: "查看温度/电源/风扇"},
			InspectPower:          {Type: InspectPower, Name: "电源状态", Command: "show system power-supply", Description: "查看电源状态"},
			InspectFan:            {Type: InspectFan, Name: "风扇状态", Command: "show system fan", Description: "查看风扇状态"},
			InspectOptic:          {Type: InspectOptic, Name: "光模块信息", Command: "show interfaces transceiver", DeviceTypes: []DeviceType{DeviceSwitch}, Description: "查看光模块参数"},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 配置", Command: "show ip dns", Description: "查看 DNS 配置"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置摘要", Command: "show running-config | include hostname", Description: "查看配置摘要"},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "show ip dhcp server", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 DHCP 池"},
		},
	}
}

// ───────────────────────────────────────────────
// TP-Link JetStream
// ───────────────────────────────────────────────

func newTplinkAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorTplink,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show process cpu", Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show memory", Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show interface status", Description: "检查所有接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show version", Description: "检查固件版本"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show ip route", Description: "查看路由表"},
			InspectLog:            {Type: InspectLog, Name: "系统日志", Command: "show log buffer", Description: "查看系统日志"},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "show spanning-tree", Description: "查看 STP 状态"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 信息", Command: "show vlan", Description: "查看 VLAN 配置"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "show arp", Description: "查看 ARP 表"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 地址表", Command: "show mac-address-table", Description: "查看 MAC 表"},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp neighbor", Description: "查看 LLDP 邻居"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp status", Description: "查看 NTP 状态"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置摘要", Command: "show running-config | include hostname", Description: "查看配置摘要"},
			InspectPool:           {Type: InspectPool, Name: "DHCP 池", Command: "show ip dhcp pool", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 DHCP 池"},
		},
	}
}

// ───────────────────────────────────────────────
// F5 BIG-IP 负载均衡
// ───────────────────────────────────────────────

func newF5Adapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorF5,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "tmsh show sys performance module cpu | grep \"CPU\"", FallbackCommands: []string{"tmsh show sys cpu"}, Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "tmsh show sys memory", Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "tmsh show net interface", FallbackCommands: []string{"ifconfig -a"}, Description: "检查所有接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "tmsh show sys version", Description: "检查 BIG-IP 版本"},
			InspectLog:            {Type: InspectLog, Name: "最近日志", Command: "tmsh show ltm log last 30", FallbackCommands: []string{"cat /var/log/ltm | tail -30"}, Description: "查看最近 LTM 日志"},
			InspectPool:           {Type: InspectPool, Name: "节点池状态", Command: "tmsh show ltm pool", Description: "查看所有节点池及成员状态", DeviceTypes: []DeviceType{DeviceLoadBalancer}},
			InspectSession:        {Type: InspectSession, Name: "连接统计", Command: "tmsh show sys performance module connections", Description: "查看当前连接数统计", DeviceTypes: []DeviceType{DeviceLoadBalancer}},
			InspectDNS:            {Type: InspectDNS, Name: "DNS 解析", Command: "tmsh show ltm dns", Description: "查看 DNS 解析配置"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "tmsh show sys ntp", Description: "查看 NTP 同步状态"},
			InspectLicense:        {Type: InspectLicense, Name: "License 信息", Command: "tmsh show sys license", Description: "查看 License 有效期"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "tmsh show net route", Description: "查看路由表"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "tmsh show net arp", Description: "查看 ARP 表"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置快照", Command: "tmsh show sys version | uname -a", Description: "查看设备基础信息"},
			InspectVPN:            {Type: InspectVPN, Name: "VPN / APM 状态", Command: "tmsh show apm session", Description: "查看 APM VPN 会话", DeviceTypes: []DeviceType{DeviceLoadBalancer}},
		},
	}
}

// ───────────────────────────────────────────────
// 迈普 (Maipu) MyPowerOS
// ───────────────────────────────────────────────

func newMaipuAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorMaipu,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show cpu-usage", FallbackCommands: []string{"show cpu"}, Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show memory", FallbackCommands: []string{"show memory-usage"}, Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show interface brief", FallbackCommands: []string{"show ip interface brief"}, Description: "检查所有接口物理与协议状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show version", Description: "检查系统版本和运行时间"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show ip route", Description: "查看系统路由表"},
			InspectLog:            {Type: InspectLog, Name: "最近日志", Command: "show logging", FallbackCommands: []string{"show log"}, Description: "查看最近系统日志"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "环境状态", Command: "show environment", FallbackCommands: []string{"show temperature"}, Description: "查看设备温度和环境状态"},
			InspectPower:          {Type: InspectPower, Name: "电源状态", Command: "show power", Description: "查看电源模块状态"},
			InspectFan:            {Type: InspectFan, Name: "风扇状态", Command: "show fan", Description: "查看风扇运行状态"},
			InspectSTP:            {Type: InspectSTP, Name: "STP 状态", Command: "show spanning-tree", Description: "查看生成树状态"},
			InspectVLAN:           {Type: InspectVLAN, Name: "VLAN 信息", Command: "show vlan", Description: "查看 VLAN 配置"},
			InspectARP:            {Type: InspectARP, Name: "ARP 表", Command: "show arp", Description: "查看 ARP 表项"},
			InspectMAC:            {Type: InspectMAC, Name: "MAC 地址表", Command: "show mac-address-table", FallbackCommands: []string{"show mac-address"}, Description: "查看 MAC 地址表"},
			InspectNeighbor:       {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp neighbor brief", FallbackCommands: []string{"show lldp neighbor"}, Description: "查看 LLDP 邻居"},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp status", Description: "查看 NTP 同步状态"},
			InspectLicense:        {Type: InspectLicense, Name: "License 状态", Command: "show license", Description: "查看 License 信息"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置快照", Command: "show running-config | include hostname", Description: "查看运行配置快照"},
			InspectBGP:            {Type: InspectBGP, Name: "BGP 状态", Command: "show ip bgp summary", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 BGP 邻居"},
			InspectOSPF:           {Type: InspectOSPF, Name: "OSPF 状态", Command: "show ip ospf neighbor", DeviceTypes: []DeviceType{DeviceRouter, DeviceSwitch}, Description: "查看 OSPF 邻居"},
		},
	}
}

// ───────────────────────────────────────────────
// 深信服 (Sangfor) AC/AF/NGAF
// ───────────────────────────────────────────────

func newSangforAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorSangfor,
		enablePassword: true,
		templates: map[InspectionType]CommandTemplate{
			InspectCPU:            {Type: InspectCPU, Name: "CPU 使用率", Command: "show system cpu", FallbackCommands: []string{"show cpu", "sysctl hw.ncpu"}, Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:         {Type: InspectMemory, Name: "内存使用率", Command: "show system memory", FallbackCommands: []string{"show memory"}, Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			InspectInterface:      {Type: InspectInterface, Name: "接口状态", Command: "show interface", FallbackCommands: []string{"ifconfig -a", "show ip interface"}, Description: "检查所有网络接口状态"},
			InspectVersion:        {Type: InspectVersion, Name: "系统版本", Command: "show version", Description: "检查系统版本和运行时间"},
			InspectRoutes:         {Type: InspectRoutes, Name: "路由表", Command: "show ip route", FallbackCommands: []string{"netstat -rn"}, Description: "查看系统路由表"},
			InspectLog:            {Type: InspectLog, Name: "系统日志", Command: "show log system", FallbackCommands: []string{"show logging"}, Description: "查看系统日志"},
			InspectSession:        {Type: InspectSession, Name: "并发会话", Command: "show session count", FallbackCommands: []string{"show session"}, Description: "查看当前连接与并发会话数", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}},
			InspectSecurityPolicy: {Type: InspectSecurityPolicy, Name: "安全策略", Command: "show policy", FallbackCommands: []string{"show firewall policy"}, Description: "查看安全与访问控制策略", DeviceTypes: []DeviceType{DeviceFirewall}},
			InspectVPN:            {Type: InspectVPN, Name: "VPN 隧道", Command: "show vpn ipsec", FallbackCommands: []string{"show vpn"}, Description: "查看 VPN 隧道状态", DeviceTypes: []DeviceType{DeviceFirewall, DeviceGateway}},
			InspectNTP:            {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp status", Description: "查看 NTP 同步状态"},
			InspectLicense:        {Type: InspectLicense, Name: "License 状态", Command: "show license", Description: "查看 License 到期时间与模块授权"},
			InspectEnvironment:    {Type: InspectEnvironment, Name: "硬件状态", Command: "show hardware", FallbackCommands: []string{"show system environment"}, Description: "查看温度风扇及硬件健康"},
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置快照", Command: "show running-config | head -20", Description: "查看配置快照"},
		},
	}
}

// ───────────────────────────────────────────────
// 通用 / 自定义厂商（other）
// 使用最常见的跨厂商兼容命令，覆盖未知品牌或自定义设备
// ───────────────────────────────────────────────

func newOtherAdapter() VendorAdapter {
	return &baseAdapter{
		vendor:         VendorOther,
		enablePassword: false, // 通用设备不确定是否支持 enable 模式
		templates: map[InspectionType]CommandTemplate{
			// 基础信息 — 多种命令兼容
			InspectVersion:   {Type: InspectVersion, Name: "系统版本", Command: "show version", FallbackCommands: []string{"display version", "get system info"}, Description: "查看系统版本（自动尝试多种命令）"},
			InspectInterface: {Type: InspectInterface, Name: "接口状态", Command: "show interface", FallbackCommands: []string{"display interface", "show interfaces", "show ip interface brief"}, Description: "检查所有接口状态"},
			// CPU / 内存 — 尝试通用 SNMP 方式，CLI 命令不一定可用
			InspectCPU:       {Type: InspectCPU, Name: "CPU 使用率", Command: "show cpu", FallbackCommands: []string{"display cpu", "show processes cpu"}, Description: "检查 CPU 使用率", Thresholds: map[string]int{"warning": 70, "critical": 85}},
			InspectMemory:    {Type: InspectMemory, Name: "内存使用率", Command: "show memory", FallbackCommands: []string{"display memory", "show processes memory"}, Description: "检查内存使用率", Thresholds: map[string]int{"warning": 75, "critical": 90}},
			// 路由 / ARP / 邻居
			InspectRoutes:    {Type: InspectRoutes, Name: "路由表", Command: "show ip route", FallbackCommands: []string{"display ip routing-table", "show route"}, Description: "查看路由表"},
			InspectARP:       {Type: InspectARP, Name: "ARP 表", Command: "show arp", FallbackCommands: []string{"display arp"}, Description: "查看 ARP 表"},
			InspectNeighbor:  {Type: InspectNeighbor, Name: "LLDP 邻居", Command: "show lldp neighbors", FallbackCommands: []string{"display lldp neighbor", "show lldp info"}, Description: "查看 LLDP 邻居"},
			// 配置
			InspectConfigChecksum: {Type: InspectConfigChecksum, Name: "配置快照", Command: "show running-config", FallbackCommands: []string{"display current-configuration", "show config"}, Description: "查看当前运行配置"},
			// 日志
			InspectLog:       {Type: InspectLog, Name: "最近日志", Command: "show logging", FallbackCommands: []string{"display logbuffer", "show log"}, Description: "查看最近系统日志"},
			// NTP
			InspectNTP:       {Type: InspectNTP, Name: "NTP 状态", Command: "show ntp status", FallbackCommands: []string{"display ntp-service status"}, Description: "查看 NTP 同步状态"},
		},
	}
}

