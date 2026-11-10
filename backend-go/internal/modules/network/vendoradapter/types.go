// 厂商适配器 — 共享类型定义和辅助函数，等价于 TS vendorAdapter/types.ts + shared.ts
package vendoradapter

// VendorType 厂商类型
type VendorType string

const (
	VendorHuawei    VendorType = "huawei"
	VendorCisco     VendorType = "cisco"
	VendorH3c       VendorType = "h3c"
	VendorRuijie    VendorType = "ruijie"
	VendorRuijieEG  VendorType = "ruijie_eg"
	VendorZte       VendorType = "zte"
	VendorFortinet  VendorType = "fortinet"
	VendorPaloAlto  VendorType = "paloalto"
	VendorJuniper   VendorType = "juniper"
	VendorArista    VendorType = "arista"
	VendorHpe       VendorType = "hpe"
	VendorMikrotik  VendorType = "mikrotik"
	VendorUbiquiti  VendorType = "ubiquiti"
	VendorDell      VendorType = "dell"
	VendorTplink    VendorType = "tplink"
	VendorF5        VendorType = "f5"
	VendorMaipu     VendorType = "maipu"
	VendorSangfor   VendorType = "sangfor"
	VendorOther     VendorType = "other" // 通用/自定义厂商
)

// DeviceType 设备类型
type DeviceType string

const (
	DeviceSwitch       DeviceType = "switch"
	DeviceRouter       DeviceType = "router"
	DeviceFirewall     DeviceType = "firewall"
	DeviceLoadBalancer DeviceType = "loadbalancer"
	DeviceWLC          DeviceType = "wlc"
	DeviceAP           DeviceType = "ap"
	DeviceGateway      DeviceType = "gateway"
	DeviceUnknown      DeviceType = "unknown"
)

// InspectionType 巡检类型
type InspectionType string

const (
	InspectCPU            InspectionType = "cpu"
	InspectMemory         InspectionType = "memory"
	InspectInterface      InspectionType = "interface"
	InspectVersion        InspectionType = "version"
	InspectRoutes         InspectionType = "routes"
	InspectLog            InspectionType = "log"
	InspectEnvironment    InspectionType = "environment"
	InspectPower          InspectionType = "power"
	InspectFan            InspectionType = "fan"
	InspectSTP            InspectionType = "stp"
	InspectVLAN           InspectionType = "vlan"
	InspectARP            InspectionType = "arp"
	InspectMAC            InspectionType = "mac"
	InspectOptic          InspectionType = "optic"
	InspectNeighbor       InspectionType = "neighbor"
	InspectSecurityPolicy InspectionType = "security_policy"
	InspectNAT            InspectionType = "nat"
	InspectSession        InspectionType = "session"
	InspectVPN            InspectionType = "vpn"
	InspectWLAN           InspectionType = "wlan"
	InspectPool           InspectionType = "pool"
	InspectDNS            InspectionType = "dns"
	InspectBGP            InspectionType = "bgp"
	InspectOSPF           InspectionType = "ospf"
	InspectNTP            InspectionType = "ntp"
	InspectLicense        InspectionType = "license"
	InspectConfigChecksum InspectionType = "config_checksum"
)

// CommandTemplate 命令模板
type CommandTemplate struct {
	Type             InspectionType `json:"type"`
	Name             string         `json:"name"`
	Command          string         `json:"command"`
	FallbackCommands []string      `json:"fallbackCommands,omitempty"`
	Description      string         `json:"description"`
	ExpectedPattern  string         `json:"expectedPattern,omitempty"`
	Thresholds       map[string]int `json:"thresholds,omitempty"`
	MinFirmware      string         `json:"minFirmware,omitempty"`
	Models           []string       `json:"models,omitempty"`
	DeviceTypes      []DeviceType   `json:"deviceTypes,omitempty"`
}

// VendorAdapter 厂商适配器接口
type VendorAdapter interface {
	Vendor() VendorType
	GetCommands(types []InspectionType, deviceType DeviceType) []CommandTemplate
	GetCommand(typ InspectionType) (CommandTemplate, bool)
	SupportsEnablePassword() bool
}

// baseAdapter 基础适配器实现，所有厂商适配器嵌入此结构体
type baseAdapter struct {
	vendor          VendorType
	templates       map[InspectionType]CommandTemplate
	enablePassword  bool
}

func (a *baseAdapter) Vendor() VendorType {
	return a.vendor
}

func (a *baseAdapter) GetCommand(typ InspectionType) (CommandTemplate, bool) {
	t, ok := a.templates[typ]
	return t, ok
}

func (a *baseAdapter) SupportsEnablePassword() bool {
	return a.enablePassword
}

func (a *baseAdapter) GetCommands(types []InspectionType, deviceType DeviceType) []CommandTemplate {
	var all []CommandTemplate
	if len(types) > 0 {
		for _, t := range types {
			if tpl, ok := a.templates[t]; ok {
				all = append(all, tpl)
			}
		}
	} else {
		for _, tpl := range a.templates {
			all = append(all, tpl)
		}
	}
	return filterByDeviceType(all, deviceType)
}

// filterByDeviceType 根据 deviceType 过滤指令模板
func filterByDeviceType(templates []CommandTemplate, deviceType DeviceType) []CommandTemplate {
	if deviceType == "" || deviceType == DeviceUnknown {
		return templates
	}
	var out []CommandTemplate
	for _, t := range templates {
		if len(t.DeviceTypes) == 0 {
			out = append(out, t)
			continue
		}
		for _, dt := range t.DeviceTypes {
			if dt == deviceType {
				out = append(out, t)
				break
			}
		}
	}
	return out
}
