// SNMP 类型定义 — 等价于 TS snmpTypes.ts
package services

import (
	"fmt"
	"math/big"
	"strings"
)

// SnmpVersion SNMP 版本
type SnmpVersion string

const (
	SNMPv1   SnmpVersion = "v1"
	SNMPv2c  SnmpVersion = "v2c"
	SNMPv3   SnmpVersion = "v3"
)

// DefaultSnmpVersion 默认 SNMP 版本
const DefaultSnmpVersion = SNMPv2c

// SnmpCredential SNMP 凭证
type SnmpCredential struct {
	ID               string      `json:"id"`
	DeviceID         string      `json:"device_id,omitempty"`
	Name             string      `json:"name"`
	Community        string      `json:"community,omitempty"`        // v1/v2c
	SnmpUser         string      `json:"snmp_user,omitempty"`        // v3
	SnmpAuthProtocol string      `json:"snmp_auth_protocol,omitempty"` // MD5 | SHA
	SnmpAuthKey      string      `json:"snmp_auth_key,omitempty"`
	SnmpPrivProtocol string      `json:"snmp_priv_protocol,omitempty"` // DES | AES
	SnmpPrivKey      string      `json:"snmp_priv_key,omitempty"`
	SnmpVersion      SnmpVersion `json:"snmp_version"`
	SnmpPort         int         `json:"snmp_port"`
}

// SnmpResult SNMP 查询结果
type SnmpResult struct {
	OID      string `json:"oid"`
	Value    string `json:"value"`
	Type     int    `json:"type,omitempty"`
	TypeName string `json:"typeName,omitempty"`
}

// InterfaceInfo 接口信息
type InterfaceInfo struct {
	Index          int     `json:"index"`
	Name           string  `json:"name"`
	Descr          string  `json:"descr"`
	Type           int     `json:"type"`
	TypeName       string  `json:"typeName"`
	Speed          int64   `json:"speed"`           // bps
	MTU            int     `json:"mtu"`
	MAC            string  `json:"mac"`
	AdminStatus    string  `json:"adminStatus"`     // up | down
	OperStatus     string  `json:"operStatus"`      // up | down
	Alias          string  `json:"alias"`
	InOctets       uint64  `json:"inOctets"`
	OutOctets      uint64  `json:"outOctets"`
	InErrors       int64   `json:"inErrors"`
	OutErrors      int64   `json:"outErrors"`
	InUcastPkts    int64   `json:"inUcastPkts"`     // 单播入包数
	OutUcastPkts   int64   `json:"outUcastPkts"`    // 单播出包数
	LastChange     int64   `json:"lastChange"`      // 接口状态最后变更时间 (centi-seconds since sysUpTime)
	InUtilization  float64 `json:"inUtilization"`   // %
	OutUtilization float64 `json:"outUtilization"`  // %
}

// SnmpInterfaceMetric SNMP 巡检接口指标（返回给前端）
type SnmpInterfaceMetric struct {
	Index          int     `json:"index"`
	Name           string  `json:"name"`
	OperStatus     string  `json:"operStatus"`
	AdminStatus    string  `json:"adminStatus"`
	Speed          int64   `json:"speed"`
	MTU            int     `json:"mtu"`
	MAC            string  `json:"mac"`
	InBps          int64   `json:"inBps"`
	OutBps         int64   `json:"outBps"`
	InUtilization  float64 `json:"inUtilization"`
	OutUtilization float64 `json:"outUtilization"`
	InErrors       int64   `json:"inErrors"`
	OutErrors      int64   `json:"outErrors"`
}

// SnmpInspectionResult SNMP 巡检结果（返回给前端）
type SnmpInspectionResult struct {
	Reachable      bool                   `json:"reachable"`
	SysName        string                 `json:"sysName"`
	SysDescr       string                 `json:"sysDescr"`
	SysUptime      int64                  `json:"sysUptime"`
	Interfaces     []SnmpInterfaceMetric  `json:"interfaces"`
	InterfaceCount int                    `json:"interfaceCount"`
	UpCount        int                    `json:"upCount"`
	DownCount      int                    `json:"downCount"`
	Alerts         []string               `json:"alerts"`
	PollDurationMs int64                  `json:"pollDurationMs"`
}

// DeviceHealth 设备健康状态
type DeviceHealth struct {
	SysName           string   `json:"sysName"`
	SysDescr          string   `json:"sysDescr"`
	SysUptime         int64    `json:"sysUptime"`
	SysLocation       string   `json:"sysLocation"`
	SysContact        string   `json:"sysContact"`
	CpuUsage          *float64 `json:"cpuUsage"`
	MemoryUsage       *float64 `json:"memoryUsage"`
	Temperature       *float64 `json:"temperature"`
	InterfaceCount    int      `json:"interfaceCount"`
	InterfacesUp      int      `json:"interfacesUp"`
	InterfacesDown    int      `json:"interfacesDown"`
	InterfaceHighUtil int      `json:"interfaceHighUtil"` // >80% 接口数
	// 环境监控指标
	PowerStatus       *string  `json:"powerStatus"`        // 电源状态摘要
	FanStatus         *string  `json:"fanStatus"`          // 风扇状态摘要
	OpticRxPower      *float64 `json:"opticRxPower"`      // 光模块接收功率(dBm)
	OpticTxPower      *float64 `json:"opticTxPower"`      // 光模块发送功率(dBm)
	OpticTemperature  *float64 `json:"opticTemperature"`  // 光模块温度(℃)
}

// SystemInfo 系统基本信息
type SystemInfo struct {
	SysName     string `json:"sysName"`
	SysDescr    string `json:"sysDescr"`
	SysUptime   int64  `json:"sysUptime"`
	SysLocation string `json:"sysLocation"`
	SysContact  string `json:"sysContact"`
}

// SnmpCounter64Type gosnmp Counter64 类型常量
const SnmpCounter64Type = 0x46 // 70 = AsnCounter64

// Counter64BytesToString 将 Counter64 字节转为大整数字符串
func Counter64BytesToString(buf []byte) string {
	if len(buf) == 0 {
		return "0"
	}
	n := new(big.Int)
	n.SetBytes(buf)
	return n.String()
}

// NormalizeSnmpValue 统一处理 SNMP 返回值，转为字符串
func NormalizeSnmpValue(typeCode int, value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case uint64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%v", v)
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// SnmpTypeName 将 gosnmp 类型常量转为可读名称
func SnmpTypeName(typeCode int) string {
	switch typeCode {
	case 0x02:
		return "Integer32"
	case 0x04:
		return "OctetString"
	case 0x05:
		return "Null"
	case 0x06:
		return "ObjectIdentifier"
	case 0x10:
		return "IpAddress"
	case 0x40:
		return "IpAddress"
	case 0x41:
		return "Counter32"
	case 0x42:
		return "Gauge32"
	case 0x43:
		return "TimeTicks"
	case 0x44:
		return "Opaque"
	case 0x45:
		return "NsapAddress"
	case 0x46:
		return "Counter64"
	case 0x47:
		return "Unsigned32"
	default:
		return fmt.Sprintf("Type(%d)", typeCode)
	}
}

// FormatMAC 将十六进制 MAC 字符串格式化为 XX:XX:XX:XX:XX:XX
func FormatMAC(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// 移除可能的分隔符
	raw = strings.ReplaceAll(raw, ":", "")
	raw = strings.ReplaceAll(raw, "-", "")
	raw = strings.ReplaceAll(raw, " ", "")

	if len(raw) < 12 {
		return raw
	}

	var parts []string
	for i := 0; i < 12; i += 2 {
		parts = append(parts, raw[i:i+2])
	}
	return strings.Join(parts, ":")
}
