// SNMP OID 注册表 — 等价于 TS snmpOidRegistry.ts
package services

// ───────────────────────────────────────────────
// 标准系统 OID (RFC 1213 / RFC 4293)
// ───────────────────────────────────────────────

var SystemOIDs = struct {
	SysDescr    string
	SysObjectID string
	SysUptime   string
	SysContact  string
	SysName     string
	SysLocation string
	SysServices string
}{
	SysDescr:    "1.3.6.1.2.1.1.1.0",
	SysObjectID: "1.3.6.1.2.1.1.2.0",
	SysUptime:   "1.3.6.1.2.1.1.3.0",
	SysContact:  "1.3.6.1.2.1.1.4.0",
	SysName:     "1.3.6.1.2.1.1.5.0",
	SysLocation: "1.3.6.1.2.1.1.6.0",
	SysServices: "1.3.6.1.2.1.1.7.0",
}

// ───────────────────────────────────────────────
// 接口 MIB (IF-MIB)
// ───────────────────────────────────────────────

var IfMibOIDs = struct {
	IfNumber      string
	IfTable       string
	IfIndex       string
	IfDescr       string
	IfType        string
	IfMtu         string
	IfSpeed       string
	IfPhysAddress string
	IfAdminStatus string
	IfOperStatus  string
	IfInOctets    string
	IfInUcastPkts string
	IfInErrors    string
	IfOutOctets   string
	IfOutUcastPkts string
	IfOutErrors   string
	IfLastChange  string
	IfHighSpeed   string
	IfHCInOctets  string
	IfHCOutOctets string
	IfName        string
	IfAlias       string
}{
	IfNumber:      "1.3.6.1.2.1.2.1.0",
	IfTable:       "1.3.6.1.2.1.2.2",
	IfIndex:       "1.3.6.1.2.1.2.2.1.1",
	IfDescr:       "1.3.6.1.2.1.2.2.1.2",
	IfType:        "1.3.6.1.2.1.2.2.1.3",
	IfMtu:         "1.3.6.1.2.1.2.2.1.4",
	IfSpeed:       "1.3.6.1.2.1.2.2.1.5",
	IfPhysAddress: "1.3.6.1.2.1.2.2.1.6",
	IfAdminStatus: "1.3.6.1.2.1.2.2.1.7",
	IfOperStatus:  "1.3.6.1.2.1.2.2.1.8",
	IfInOctets:    "1.3.6.1.2.1.2.2.1.10",
	IfInUcastPkts: "1.3.6.1.2.1.2.2.1.11",
	IfInErrors:    "1.3.6.1.2.1.2.2.1.14",
	IfOutOctets:   "1.3.6.1.2.1.2.2.1.16",
	IfOutUcastPkts: "1.3.6.1.2.1.2.2.1.17",
	IfOutErrors:   "1.3.6.1.2.1.2.2.1.20",
	IfLastChange:  "1.3.6.1.2.1.2.2.1.22",
	IfHighSpeed:   "1.3.6.1.2.1.31.1.1.1.15",
	IfHCInOctets:  "1.3.6.1.2.1.31.1.1.1.6",
	IfHCOutOctets: "1.3.6.1.2.1.31.1.1.1.10",
	IfName:        "1.3.6.1.2.1.31.1.1.1.1",
	IfAlias:       "1.3.6.1.2.1.31.1.1.1.18",
}

// ───────────────────────────────────────────────
// LLDP MIB (IEEE 802.1AB)
// ───────────────────────────────────────────────

var LldpMibOIDs = struct {
	LldpLocChassisId    string
	LldpLocSysName      string
	LldpLocSysDesc      string
	LldpRemTable        string
	LldpRemTimeMark     string
	LldpRemLocalPortNum string
	LldpRemChassisId    string
	LldpRemSysName      string
	LldpRemSysDesc      string
	LldpRemPortDescr    string
}{
	LldpLocChassisId:    "1.0.8802.1.1.2.1.3.1.0",
	LldpLocSysName:      "1.0.8802.1.1.2.1.3.3.0",
	LldpLocSysDesc:      "1.0.8802.1.1.2.1.3.4.0",
	LldpRemTable:        "1.0.8802.1.1.2.1.4.1.1",
	LldpRemTimeMark:     "1.0.8802.1.1.2.1.4.1.1.2",
	LldpRemLocalPortNum: "1.0.8802.1.1.2.1.4.1.1.4",
	LldpRemChassisId:    "1.0.8802.1.1.2.1.4.1.1.5",
	LldpRemSysName:      "1.0.8802.1.1.2.1.4.1.1.9",
	LldpRemSysDesc:      "1.0.8802.1.1.2.1.4.1.1.10",
	LldpRemPortDescr:    "1.0.8802.1.1.2.1.4.1.1.8",
}

// ───────────────────────────────────────────────
// 接口速率阈值（bps）
// ───────────────────────────────────────────────

const (
	InterfaceWarningBps  int64 = 800_000_000  // 800Mbps 告警
	InterfaceCriticalBps int64 = 950_000_000  // 950Mbps 告警
)

// ───────────────────────────────────────────────
// 厂商私有 MIB OID 映射
// ───────────────────────────────────────────────

// VendorOidMapping 厂商 OID 映射
type VendorOidMapping struct {
	CPUUsage       string
	MemoryUsage    string
	MemTotal       string
	MemFree        string
	MemUsed        string
	CPU5sec        string
	CPU1min        string
	CPU5min        string
	Temperature    []string
	PowerStatus    string
	FanStatus      string
	OpticRxPower   string
	OpticTxPower   string
	OpticTemperature string
}

// VendorOIDs 厂商私有 OID 注册表
var VendorOIDs = map[string]VendorOidMapping{
	"huawei": {
		CPUUsage:         "1.3.6.1.4.1.2011.6.3.4.1.2.0",
		MemoryUsage:      "1.3.6.1.4.1.2011.6.3.5.1.1.2.0",
		Temperature:      []string{"1.3.6.1.4.1.2011.6.3.4.1.1.2"},
		PowerStatus:      "1.3.6.1.4.1.2011.6.3.4.1.3",
		FanStatus:        "1.3.6.1.4.1.2011.6.3.4.1.1.3",
		OpticRxPower:     "1.3.6.1.4.1.2011.10.2.6.1.1.1.1.5",  // hwOpticalModuleInfoRxPower
		OpticTxPower:     "1.3.6.1.4.1.2011.10.2.6.1.1.1.1.6",  // hwOpticalModuleInfoTxPower
		OpticTemperature: "1.3.6.1.4.1.2011.10.2.6.1.1.1.1.4",  // hwOpticalModuleInfoTemperature
	},
	"cisco": {
		CPU5sec:          "1.3.6.1.4.1.9.9.109.1.1.1.1.7.1",
		CPU1min:          "1.3.6.1.4.1.9.9.109.1.1.1.1.8.1",
		CPU5min:          "1.3.6.1.4.1.9.9.109.1.1.1.1.6.1",
		MemFree:          "1.3.6.1.4.1.9.9.48.1.1.1.6.1",
		MemUsed:          "1.3.6.1.4.1.9.9.48.1.1.1.5.1",
		Temperature:      []string{"1.3.6.1.4.1.9.9.13.1.3.1.3"},
		PowerStatus:      "1.3.6.1.4.1.9.9.117.1.1.2.1.2",
		FanStatus:        "1.3.6.1.4.1.9.9.117.1.1.3.1.2",
		OpticRxPower:     "1.3.6.1.4.1.9.9.760.1.1.1.1.6",   // cisqIfEtherStatsRxPower (需 IOS-XE 15.7+)
		OpticTxPower:     "1.3.6.1.4.1.9.9.760.1.1.1.1.5",   // cisqIfEtherStatsTxPower
		OpticTemperature: "1.3.6.1.4.1.9.9.760.1.1.1.1.7",   // cisqIfEtherStatsTemp
	},
	"h3c": {
		CPUUsage:         "1.3.6.1.4.1.25506.2.6.1.1.1.1.6.1",
		MemoryUsage:      "1.3.6.1.4.1.25506.2.6.1.1.1.1.8.1",
		Temperature:      []string{"1.3.6.1.4.1.25506.2.6.1.1.1.1.12"},
		PowerStatus:      "1.3.6.1.4.1.25506.2.6.1.1.1.1.10",
		FanStatus:        "1.3.6.1.4.1.25506.2.6.1.1.1.1.11",
		OpticRxPower:     "1.3.6.1.4.1.25506.2.70.1.1.1.6",   // hh3cTransceiverRxPower
		OpticTxPower:     "1.3.6.1.4.1.25506.2.70.1.1.1.5",   // hh3cTransceiverTxPower
		OpticTemperature: "1.3.6.1.4.1.25506.2.70.1.1.1.4",   // hh3cTransceiverTemperature
	},
	"fortinet": {
		CPUUsage:         "1.3.6.1.4.1.12356.101.4.1.4.0",
		MemoryUsage:      "1.3.6.1.4.1.12356.101.4.1.3.0",
		MemTotal:         "1.3.6.1.4.1.12356.101.4.1.5.0",
		MemFree:          "1.3.6.1.4.1.12356.101.4.1.6.0",
		CPU5sec:          "1.3.6.1.4.1.12356.101.4.1.1.0",
		CPU1min:          "1.3.6.1.4.1.12356.101.4.1.2.0",
	},
	"juniper": {
		CPUUsage:         "1.3.6.1.4.1.2636.3.1.13.1.7.7.1",
		MemoryUsage:      "1.3.6.1.4.1.2636.3.1.13.1.11.7.1",
		Temperature:      []string{"1.3.6.1.4.1.2636.3.1.13.1.8.7.1"},
	},
	"paloalto": {
		CPUUsage:         "1.3.6.1.4.1.25461.2.1.2.1.5.0",
		MemoryUsage:      "1.3.6.1.4.1.25461.2.1.2.1.2.0",
		CPU5sec:          "1.3.6.1.4.1.25461.2.1.2.5.1.0",
		CPU1min:          "1.3.6.1.4.1.25461.2.1.2.5.2.0",
	},
	"arista": {
		CPUUsage:         "1.3.6.1.4.1.30065.3.2.1.1.6.1",
		MemoryUsage:      "1.3.6.1.4.1.30065.3.2.1.1.7.1",
		Temperature:      []string{"1.3.6.1.4.1.30065.3.1.1.1.1.4.8"},   // aristaWireless... 温度
		PowerStatus:      "1.3.6.1.4.1.30065.3.1.1.1.1.4.2",            // 电源状态
		FanStatus:        "1.3.6.1.4.1.30065.3.1.1.1.1.4.5",            // 风扇状态
	},
	"mikrotik": {
		CPUUsage:         "1.3.6.1.4.1.14988.1.1.1.1.0",
		MemTotal:         "1.3.6.1.4.1.14988.1.1.1.2.0",
		MemFree:          "1.3.6.1.4.1.14988.1.1.1.3.0",
		Temperature:      []string{"1.3.6.1.4.1.14988.1.1.3.10.0"},     // mtxrHlTemperature
		PowerStatus:      "1.3.6.1.4.1.14988.1.1.3.7.0",               // mtxrHlVoltage (电源电压)
		FanStatus:        "1.3.6.1.4.1.14988.1.1.3.17.0",              // mtxrHlFanSpeed1
	},
	"dell": {
		CPUUsage:         "1.3.6.1.4.1.674.10895.5000.2.6132.1.1.1.1.4.1.0",
		MemoryUsage:      "1.3.6.1.4.1.674.10895.5000.2.6132.1.1.1.1.16.1.0",
		Temperature:      []string{"1.3.6.1.4.1.674.10895.5000.2.6132.1.1.1.1.8.1"},  // 环境温度
		PowerStatus:      "1.3.6.1.4.1.674.10895.5000.2.6132.1.1.1.1.12.1",           // 电源状态
		FanStatus:        "1.3.6.1.4.1.674.10895.5000.2.6132.1.1.1.1.10.1",           // 风扇状态
	},
	// ── 以下为新增厂商 OID 映射 ──
	"ruijie": {
		CPUUsage:         "1.3.6.1.4.1.4881.1.1.10.2.1.1.1.0",         // 锐捷 CPU 使用率
		MemoryUsage:      "1.3.6.1.4.1.4881.1.1.10.2.1.1.2.0",         // 锐捷内存使用率
		Temperature:      []string{"1.3.6.1.4.1.4881.1.1.10.2.7.1.1.1.3"},  // 温度
		PowerStatus:      "1.3.6.1.4.1.4881.1.1.10.2.6.1.1.1.2",           // 电源状态
		FanStatus:        "1.3.6.1.4.1.4881.1.1.10.2.5.1.1.1.2",           // 风扇状态
	},
	"ruijie_eg": {
		CPUUsage:         "1.3.6.1.4.1.4881.1.1.10.2.1.1.1.0",
		MemoryUsage:      "1.3.6.1.4.1.4881.1.1.10.2.1.1.2.0",
	},
	"zte": {
		CPUUsage:         "1.3.6.1.4.1.3902.3.3005.1.1.4.0",            // 中兴 CPU 使用率
		MemoryUsage:      "1.3.6.1.4.1.3902.3.3005.1.1.5.0",            // 中兴内存使用率
		Temperature:      []string{"1.3.6.1.4.1.3902.3.3005.1.1.11.1.1.2"},  // 温度
		PowerStatus:      "1.3.6.1.4.1.3902.3.3005.1.1.12.1.1.2",           // 电源状态
		FanStatus:        "1.3.6.1.4.1.3902.3.3005.1.1.13.1.1.2",           // 风扇状态
	},
	"hpe": {
		CPUUsage:         "1.3.6.1.4.1.11.2.14.11.5.1.9.1.1.1",         // HPE CPU 使用率
		MemoryUsage:      "1.3.6.1.4.1.11.2.14.11.5.1.9.2.1.1",         // HPE 内存使用率
		Temperature:      []string{"1.3.6.1.4.1.11.2.14.11.5.1.9.3.1.1"},  // 温度
		PowerStatus:      "1.3.6.1.4.1.11.2.14.11.5.1.9.4.1.1",           // 电源状态
		FanStatus:        "1.3.6.1.4.1.11.2.14.11.5.1.9.5.1.1",           // 风扇状态
	},
	"ubiquiti": {
		CPUUsage:         "1.3.6.1.4.1.41112.1.1.3.0",                  // Ubiquiti CPU 使用率
		MemoryUsage:      "1.3.6.1.4.1.41112.1.1.4.0",                  // Ubiquiti 内存使用率
		Temperature:      []string{"1.3.6.1.4.1.41112.1.1.5.0"},         // 温度
	},
	"tplink": {
		CPUUsage:         "1.3.6.1.4.1.11863.3.3.1.1.1.0",             // TP-Link CPU 使用率
		MemoryUsage:      "1.3.6.1.4.1.11863.3.3.1.1.2.0",             // TP-Link 内存使用率
		Temperature:      []string{"1.3.6.1.4.1.11863.3.3.1.2.1.0"},    // 温度
	},
	"f5": {
		CPUUsage:         "1.3.6.1.4.1.3375.2.1.7.5.2.1.1.1.1",         // F5 CPU 使用率（per-slot）
		MemoryUsage:      "1.3.6.1.4.1.3375.2.1.7.5.3.1.1.1.1",         // F5 内存使用率
		Temperature:      []string{"1.3.6.1.4.1.3375.2.1.3.17.2.1.2"},  // 温度传感器
		PowerStatus:      "1.3.6.1.4.1.3375.2.1.3.17.1.1.2",            // 电源状态
		FanStatus:        "1.3.6.1.4.1.3375.2.1.3.17.3.1.2",            // 风扇状态
	},
}

// ───────────────────────────────────────────────
// IANA 接口类型映射 (RFC 2863 IANAifType)
// ───────────────────────────────────────────────

var IANAIfType = map[int]string{
	1:   "other",
	6:   "ethernetCsmacd",
	7:   "iso88023Csmacd",
	9:   "iso88025TokenRing",
	23:  "ppp",
	24:  "softwareLoopback",
	28:  "slip",
	53:  "propVirtual",
	62:  "fastEther",
	69:  "atm",
	71:  "frameRelay",
	72:  "rs232",
	78:  "sonet",
	94:  "trunkVlan",
	97:  "gigabitEthernet",
	101: "tunnel",
	108: "mpls",
	117: "gigabitEthernetFX",
	120: "gigabitEthernetLX",
	121: "gigabitEthernetSX",
	122: "gigabitEthernetT",
	131: "tenGigabitEthernet",
	135: "l2vlan",
	136: "l3ipvlan",
	161: "ieee8023adLag",
	176: "propChannel",
	209: "vdsl",
	216: "ieee8023adPort",
	223: "softwareLoopback",
	233: "fortyGigabitEthernet",
	243: "hundredGigabitEthernet",
	259: "twentyFiveGigabitEthernet",
	277: "twoHundredGigabitEthernet",
	283: "fourHundredGigabitEthernet",
	285: "eightHundredGigabitEthernet",
	287: "oneThousandGigabitEthernet",
}

// GetIfTypeName 根据接口类型码获取名称
func GetIfTypeName(typeCode int) string {
	if name, ok := IANAIfType[typeCode]; ok {
		return name
	}
	return "unknown"
}
