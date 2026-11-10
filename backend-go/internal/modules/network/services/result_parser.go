// 网络设备 CLI 输出解析器 — 等价于 TS networkResultParser.ts
// 30+ 解析函数，按厂商和巡检类型分派
package services

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ───────────────────────────────────────────────
// 类型定义
// ───────────────────────────────────────────────

// ParsedResult 解析结果
type ParsedResult struct {
	Type      string      `json:"type"`
	Success   bool        `json:"success"`
	Value     interface{} `json:"value,omitempty"`
	Unit      string      `json:"unit,omitempty"`
	Status    string      `json:"status"` // normal | warning | critical | error
	Details   string      `json:"details"`
	RawOutput string      `json:"rawOutput"`
	Timestamp string      `json:"timestamp"`
}

// InterfaceEntry 接口条目
type InterfaceEntry struct {
	Name             string `json:"name"`
	PhysicalStatus   string `json:"physicalStatus"`   // up | down | admin_down
	ProtocolStatus   string `json:"protocolStatus"`   // up | down
	Description      string `json:"description,omitempty"`
}

// ParseFunc 解析函数类型
type ParseFunc func(output string) ParsedResult

// nowISO 返回当前时间的 ISO 格式字符串
func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// truncateOutput 截断输出到指定长度
func truncateOutput(output string, maxLen int) string {
	if len(output) > maxLen {
		return output[:maxLen]
	}
	return output
}

// ───────────────────────────────────────────────
// CPU 解析器
// ───────────────────────────────────────────────

var cpuPercentRegex = regexp.MustCompile(`(\d+)%`)
var ciscoCpuRegex = regexp.MustCompile(`(?i)CPU utilization.*?(\d+)%`)

// parseHuaweiCpu 华为 CPU 解析
func parseHuaweiCpu(output string) ParsedResult {
	match := cpuPercentRegex.FindStringSubmatch(output)
	var cpuUsage int
	success := false
	if match != nil {
		cpuUsage, _ = strconv.Atoi(match[1])
		success = true
	}

	status := "normal"
	if success {
		if cpuUsage > 85 {
			status = "critical"
		} else if cpuUsage > 70 {
			status = "warning"
		}
	}

	details := "无法解析 CPU 使用率"
	if success {
		details = fmt.Sprintf("CPU 使用率: %d%%", cpuUsage)
	}

	return ParsedResult{
		Type:      "cpu",
		Success:   success,
		Value:     cpuUsage,
		Unit:      "%",
		Status:    status,
		Details:   details,
		RawOutput: truncateOutput(output, 500),
		Timestamp: nowISO(),
	}
}

// parseCiscoCpu 思科 CPU 解析
func parseCiscoCpu(output string) ParsedResult {
	match := ciscoCpuRegex.FindStringSubmatch(output)
	var cpuUsage int
	success := false
	if match != nil {
		cpuUsage, _ = strconv.Atoi(match[1])
		success = true
	}

	status := "normal"
	if success {
		if cpuUsage > 85 {
			status = "critical"
		} else if cpuUsage > 70 {
			status = "warning"
		}
	}

	details := "无法解析 CPU 使用率"
	if success {
		details = fmt.Sprintf("CPU 使用率: %d%%", cpuUsage)
	}

	return ParsedResult{
		Type:      "cpu",
		Success:   success,
		Value:     cpuUsage,
		Unit:      "%",
		Status:    status,
		Details:   details,
		RawOutput: truncateOutput(output, 500),
		Timestamp: nowISO(),
	}
}

// parseRuijieCpu 锐捷 CPU 解析（同华为）
func parseRuijieCpu(output string) ParsedResult {
	return parseHuaweiCpu(output)
}

// ───────────────────────────────────────────────
// 内存解析器
// ───────────────────────────────────────────────

var huaweiMemRegex = regexp.MustCompile(`(?i)Memory Using.*?(\d+)%`)
var genericPercentRegex = regexp.MustCompile(`(?i)(\d+)%`)

// parseHuaweiMemory 华为内存解析
func parseHuaweiMemory(output string) ParsedResult {
	match := huaweiMemRegex.FindStringSubmatch(output)
	if match == nil {
		match = genericPercentRegex.FindStringSubmatch(output)
	}
	var memUsage int
	success := false
	if match != nil {
		memUsage, _ = strconv.Atoi(match[1])
		success = true
	}

	status := "normal"
	if success {
		if memUsage > 90 {
			status = "critical"
		} else if memUsage > 75 {
			status = "warning"
		}
	}

	details := "无法解析内存使用率"
	if success {
		details = fmt.Sprintf("内存使用率: %d%%", memUsage)
	}

	return ParsedResult{
		Type:      "memory",
		Success:   success,
		Value:     memUsage,
		Unit:      "%",
		Status:    status,
		Details:   details,
		RawOutput: truncateOutput(output, 500),
		Timestamp: nowISO(),
	}
}

// parseCiscoMemory 思科内存解析
func parseCiscoMemory(output string) ParsedResult {
	match := genericPercentRegex.FindStringSubmatch(output)
	var memUsage int
	success := false
	if match != nil {
		memUsage, _ = strconv.Atoi(match[1])
		success = true
	}

	status := "normal"
	if success {
		if memUsage > 90 {
			status = "critical"
		} else if memUsage > 75 {
			status = "warning"
		}
	}

	details := "无法解析内存使用率"
	if success {
		details = fmt.Sprintf("内存使用率: %d%%", memUsage)
	}

	return ParsedResult{
		Type:      "memory",
		Success:   success,
		Value:     memUsage,
		Unit:      "%",
		Status:    status,
		Details:   details,
		RawOutput: truncateOutput(output, 500),
		Timestamp: nowISO(),
	}
}

// ───────────────────────────────────────────────
// 接口解析器
// ───────────────────────────────────────────────

var interfaceBriefRegex = regexp.MustCompile(`(?i)^(\S+)\s+(UP|DOWN|ADM)\s+(UP|DOWN)`)

// parseInterfaceBrief 接口简要信息解析
func parseInterfaceBrief(output string) ParsedResult {
	var interfaces []InterfaceEntry
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		match := interfaceBriefRegex.FindStringSubmatch(line)
		if match != nil {
			name := match[1]
			phys := strings.ToUpper(match[2])
			proto := strings.ToLower(match[3])

			physStatus := strings.ToLower(phys)
			if phys == "ADM" {
				physStatus = "admin_down"
			}

			interfaces = append(interfaces, InterfaceEntry{
				Name:           name,
				PhysicalStatus: physStatus,
				ProtocolStatus: proto,
			})
		}
	}

	upIfs := 0
	downIfs := 0
	for _, iface := range interfaces {
		if iface.PhysicalStatus == "up" {
			upIfs++
		} else if iface.PhysicalStatus == "down" {
			downIfs++
		}
	}

	status := "normal"
	if len(interfaces) > 0 && downIfs > len(interfaces)*30/100 {
		status = "warning"
	}

	return ParsedResult{
		Type:      "interface",
		Success:   true,
		Status:    status,
		Details:   fmt.Sprintf("总接口: %d, UP: %d, DOWN: %d", len(interfaces), upIfs, downIfs),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// ───────────────────────────────────────────────
// 版本解析器
// ───────────────────────────────────────────────

var versionRegex = regexp.MustCompile(`(?i)Version\s+(\S+)`)
var uptimeRegex = regexp.MustCompile(`(?i)uptime is\s+(.+)`)

// parseVersion 版本信息解析
func parseVersion(output string) ParsedResult {
	lines := strings.Split(output, "\n")
	var firstLines []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			firstLines = append(firstLines, l)
			if len(firstLines) >= 10 {
				break
			}
		}
	}
	firstLinesStr := strings.Join(firstLines, "\n")

	versionMatch := versionRegex.FindStringSubmatch(output)
	uptimeMatch := uptimeRegex.FindStringSubmatch(output)

	details := "无法解析版本信息"
	value := ""
	if versionMatch != nil {
		value = versionMatch[1]
		if uptimeMatch != nil {
			details = fmt.Sprintf("版本: %s, 运行时间: %s", versionMatch[1], uptimeMatch[1])
		} else {
			details = fmt.Sprintf("版本: %s", versionMatch[1])
		}
	}

	return ParsedResult{
		Type:      "version",
		Success:   true,
		Value:     value,
		Status:    "normal",
		Details:   details,
		RawOutput: firstLinesStr,
		Timestamp: nowISO(),
	}
}

// ───────────────────────────────────────────────
// 路由解析器
// ───────────────────────────────────────────────

var routeIpRegex = regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+`)
var routeProtoRegex = regexp.MustCompile(`(?i)^[OCSRB]`)

// parseRoutes 路由表解析
func parseRoutes(output string) ParsedResult {
	routeCount := 0
	for _, line := range strings.Split(output, "\n") {
		if routeIpRegex.MatchString(line) || routeProtoRegex.MatchString(line) {
			routeCount++
		}
	}

	status := "normal"
	if routeCount == 0 {
		status = "warning"
	}

	return ParsedResult{
		Type:      "routes",
		Success:   true,
		Value:     routeCount,
		Status:    status,
		Details:   fmt.Sprintf("路由表条目数: %d", routeCount),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// ───────────────────────────────────────────────
// 日志解析器
// ───────────────────────────────────────────────

var logErrorRegex = regexp.MustCompile(`(?i)error|critical|alert|emergency`)
var logWarningRegex = regexp.MustCompile(`(?i)warning|notice`)

// parseLogBuffer 日志缓冲解析
func parseLogBuffer(output string) ParsedResult {
	errorCount := len(logErrorRegex.FindAllString(output, -1))
	warningCount := len(logWarningRegex.FindAllString(output, -1))

	status := "normal"
	if errorCount > 5 {
		status = "critical"
	} else if errorCount > 0 {
		status = "warning"
	} else if warningCount > 10 {
		status = "warning"
	}

	return ParsedResult{
		Type:      "log",
		Success:   true,
		Status:    status,
		Details:   fmt.Sprintf("错误: %d, 警告: %d", errorCount, warningCount),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// ───────────────────────────────────────────────
// 环境/电源/风扇解析器
// ───────────────────────────────────────────────

var tempRegex = regexp.MustCompile(`(?i)(\d+)\s*°?C`)
var overheatRegex = regexp.MustCompile(`(?i)overheat|over\s*temp|高温|告警`)

// parseEnvironment 环境状态解析
func parseEnvironment(output string) ParsedResult {
	tempMatch := tempRegex.FindStringSubmatch(output)
	overheat := overheatRegex.MatchString(output)

	var tempVal int
	value := interface{}(nil)
	if tempMatch != nil {
		tempVal, _ = strconv.Atoi(tempMatch[1])
		value = tempVal
	}

	status := "normal"
	if overheat {
		status = "critical"
	}

	details := "环境温度信息"
	if tempMatch != nil {
		details = fmt.Sprintf("最高温度: %d°C", tempVal)
	}

	return ParsedResult{
		Type:      "environment",
		Success:   strings.TrimSpace(output) != "",
		Value:     value,
		Unit:      "°C",
		Status:    status,
		Details:   details,
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

var powerNormalRegex = regexp.MustCompile(`(?i)normal|正常|present|ok|up`)
var powerAbnormalRegex = regexp.MustCompile(`(?i)abnormal|absent|fault|error|down|fail|异常|故障|无`)

// parsePower 电源状态解析
func parsePower(output string) ParsedResult {
	normalCount := len(powerNormalRegex.FindAllString(output, -1))
	abnormalCount := len(powerAbnormalRegex.FindAllString(output, -1))

	status := "warning"
	if abnormalCount > 0 {
		status = "critical"
	} else if normalCount > 0 {
		status = "normal"
	}

	return ParsedResult{
		Type:      "power",
		Success:   strings.TrimSpace(output) != "",
		Status:    status,
		Details:   fmt.Sprintf("正常电源: %d, 异常电源: %d", normalCount, abnormalCount),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

var fanNormalRegex = regexp.MustCompile(`(?i)normal|正常|present|ok|up|running`)
var fanAbnormalRegex = regexp.MustCompile(`(?i)abnormal|absent|fault|error|stop|down|fail|异常|故障|停`)

// parseFan 风扇状态解析
func parseFan(output string) ParsedResult {
	normalCount := len(fanNormalRegex.FindAllString(output, -1))
	abnormalCount := len(fanAbnormalRegex.FindAllString(output, -1))

	status := "warning"
	if abnormalCount > 0 {
		status = "critical"
	} else if normalCount > 0 {
		status = "normal"
	}

	return ParsedResult{
		Type:      "fan",
		Success:   strings.TrimSpace(output) != "",
		Status:    status,
		Details:   fmt.Sprintf("正常风扇: %d, 异常风扇: %d", normalCount, abnormalCount),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// ───────────────────────────────────────────────
// STP/VLAN/ARP/MAC 解析器
// ───────────────────────────────────────────────

var stpPortRegex = regexp.MustCompile(`(?i)\S+\s+(DESI|ROOT|ALTE|BACK|MAST|DISA|EDGE|FWD|BLK|LIS|LRN)`)
var stpRootRegex = regexp.MustCompile(`(?i)This bridge is root|Root bridge|is rooted`)
var stpBlockedRegex = regexp.MustCompile(`(?i)BLK|DISA|ALTE|BACK`)

// parseStp STP 状态解析
func parseStp(output string) ParsedResult {
	portCount := len(stpPortRegex.FindAllString(output, -1))
	rootBridge := stpRootRegex.MatchString(output)
	blockedPorts := len(stpBlockedRegex.FindAllString(output, -1))

	status := "normal"
	if blockedPorts > 3 {
		status = "warning"
	}

	rootStr := "STP 状态"
	if rootBridge {
		rootStr = "STP 根桥"
	}

	return ParsedResult{
		Type:      "stp",
		Success:   strings.TrimSpace(output) != "",
		Value:     portCount,
		Status:    status,
		Details:   fmt.Sprintf("%s, %d 端口参与, %d 端口阻塞", rootStr, portCount, blockedPorts),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

var vlanStaticRegex = regexp.MustCompile(`(?i)\d+\s+(?:static|dynamic|STATIC)`)
var vlanTotalRegex = regexp.MustCompile(`(?i)Total.*?(\d+)`)
var vlanLineRegex = regexp.MustCompile(`^\d+\s`)

// parseVlan VLAN 信息解析
func parseVlan(output string) ParsedResult {
	matches := vlanStaticRegex.FindAllString(output, -1)
	vlanCount := len(matches)
	if vlanCount == 0 {
		if m := vlanTotalRegex.FindStringSubmatch(output); m != nil {
			vlanCount, _ = strconv.Atoi(m[1])
		}
	}

	// 也统计以数字开头的行
	lineCount := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" && vlanLineRegex.MatchString(line) {
			lineCount++
		}
	}
	if lineCount > vlanCount {
		vlanCount = lineCount
	}

	return ParsedResult{
		Type:      "vlan",
		Success:   strings.TrimSpace(output) != "",
		Value:     vlanCount,
		Status:    "normal",
		Details:   fmt.Sprintf("VLAN 数量: %d", vlanCount),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

var arpEntryRegex = regexp.MustCompile(`^\d{1,3}\.`)
var arpTotalRegex = regexp.MustCompile(`(?i)Total[\s:]+(\d+)|(\d+)\s+entr`)

// parseArp ARP 表解析
func parseArp(output string) ParsedResult {
	arpEntries := 0
	for _, line := range strings.Split(output, "\n") {
		if arpEntryRegex.MatchString(strings.TrimSpace(line)) {
			arpEntries++
		}
	}

	total := arpEntries
	if total == 0 {
		if m := arpTotalRegex.FindStringSubmatch(output); m != nil {
			if m[1] != "" {
				total, _ = strconv.Atoi(m[1])
			} else if m[2] != "" {
				total, _ = strconv.Atoi(m[2])
			}
		}
	}

	status := "warning"
	if total > 0 {
		status = "normal"
	}

	return ParsedResult{
		Type:      "arp",
		Success:   strings.TrimSpace(output) != "",
		Value:     total,
		Status:    status,
		Details:   fmt.Sprintf("ARP 表项总数: %d", total),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

var macEntryRegex = regexp.MustCompile(`(?i)^[0-9a-f]{4}[-.][0-9a-f]{4}[-.][0-9a-f]{4}`)
var macTotalRegex = regexp.MustCompile(`(?i)Total[\s:]+(\d+)`)

// parseMac MAC 地址表解析
func parseMac(output string) ParsedResult {
	macEntries := 0
	for _, line := range strings.Split(output, "\n") {
		if macEntryRegex.MatchString(strings.TrimSpace(line)) {
			macEntries++
		}
	}

	total := macEntries
	if total == 0 {
		if m := macTotalRegex.FindStringSubmatch(output); m != nil {
			total, _ = strconv.Atoi(m[1])
		}
	}

	status := "warning"
	if total > 0 {
		status = "normal"
	}

	return ParsedResult{
		Type:      "mac",
		Success:   strings.TrimSpace(output) != "",
		Value:     total,
		Status:    status,
		Details:   fmt.Sprintf("MAC 地址表项总数: %d", total),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// ───────────────────────────────────────────────
// 通用解析器
// ───────────────────────────────────────────────

// parseCustom 自定义命令解析
func parseCustom(output string) ParsedResult {
	return ParsedResult{
		Type:      "cpu",
		Success:   true,
		Value:     truncateOutput(output, 200),
		Status:    "normal",
		Details:   "自定义命令输出",
		RawOutput: truncateOutput(output, 500),
		Timestamp: nowISO(),
	}
}

// ───────────────────────────────────────────────
// 扩展解析器
// ───────────────────────────────────────────────

var txPowerRegex = regexp.MustCompile(`(?i)(?:tx|transmit|发送).*?(-?\d+\.?\d*)\s*(?:dBm|dbm)`)
var rxPowerRegex = regexp.MustCompile(`(?i)(?:rx|receive|接收).*?(-?\d+\.?\d*)\s*(?:dBm|dbm)`)
var opticModuleRegex = regexp.MustCompile(`(?i)(?:Gi|GE|TenGig|XGE|Ethernet|Eth)\S+`)

// parseOptic 光模块收发光功率解析
func parseOptic(output string) ParsedResult {
	txMatches := txPowerRegex.FindAllStringSubmatch(output, -1)
	rxMatches := rxPowerRegex.FindAllStringSubmatch(output, -1)

	var txPowers, rxPowers []float64
	for _, m := range txMatches {
		v, _ := strconv.ParseFloat(m[1], 64)
		txPowers = append(txPowers, v)
	}
	for _, m := range rxMatches {
		v, _ := strconv.ParseFloat(m[1], 64)
		rxPowers = append(rxPowers, v)
	}

	status := "normal"
	for _, p := range rxPowers {
		if p < -25 {
			status = "warning"
			break
		}
	}
	for _, p := range txPowers {
		if p > 5 {
			status = "warning"
			break
		}
	}
	for _, p := range rxPowers {
		if p < -30 {
			status = "critical"
			break
		}
	}
	for _, p := range txPowers {
		if p > 10 {
			status = "critical"
			break
		}
	}

	moduleCount := len(txPowers)
	if len(rxPowers) > moduleCount {
		moduleCount = len(rxPowers)
	}
	if modCount := len(opticModuleRegex.FindAllString(output, -1)); modCount > moduleCount {
		moduleCount = modCount
	}

	details := "未解析到光功率数据"
	if len(txPowers) > 0 || len(rxPowers) > 0 {
		txRange := "N/A"
		if len(txPowers) > 0 {
			min, max := txPowers[0], txPowers[0]
			for _, p := range txPowers {
				if p < min { min = p }
				if p > max { max = p }
			}
			txRange = fmt.Sprintf("%.2f~%.2f dBm", min, max)
		}
		rxRange := "N/A"
		if len(rxPowers) > 0 {
			min, max := rxPowers[0], rxPowers[0]
			for _, p := range rxPowers {
				if p < min { min = p }
				if p > max { max = p }
			}
			rxRange = fmt.Sprintf("%.2f~%.2f dBm", min, max)
		}
		details = fmt.Sprintf("光模块: %d 个, Tx范围: %s, Rx范围: %s", moduleCount, txRange, rxRange)
	}

	return ParsedResult{
		Type:      "optic",
		Success:   strings.TrimSpace(output) != "",
		Value:     moduleCount,
		Status:    status,
		Details:   details,
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

var neighborLineRegex = regexp.MustCompile(`(?i)^[A-Za-z0-9_\-.]+\s+\S+`)
var neighborHeaderRegex = regexp.MustCompile(`(?i)^(Local|DeviceID|Capability|---|Total)`)

// parseNeighbor LLDP/CDP 邻居发现解析
func parseNeighbor(output string) ParsedResult {
	neighborCount := 0
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if neighborLineRegex.MatchString(trimmed) && !neighborHeaderRegex.MatchString(trimmed) {
			neighborCount++
		}
	}

	status := "normal"
	if neighborCount == 0 {
		status = "warning"
	}

	return ParsedResult{
		Type:      "neighbor",
		Success:   strings.TrimSpace(output) != "",
		Value:     neighborCount,
		Status:    status,
		Details:   fmt.Sprintf("LLDP/CDP 邻居数: %d", neighborCount),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// parseSecurityPolicy 防火墙安全策略解析
func parseSecurityPolicy(output string) ParsedResult {
	policyCount := 0
	for _, line := range strings.Split(output, "\n") {
		if regexp.MustCompile(`(?i)policy|rule|策略|规则`).MatchString(line) && strings.TrimSpace(line) != "" &&
			!regexp.MustCompile(`(?i)^(---|#|Policy ID|Total)`).MatchString(line) {
			policyCount++
		}
	}
	activeCount := len(regexp.MustCompile(`(?i)active|enable|启用|permit`).FindAllString(output, -1))
	inactiveCount := len(regexp.MustCompile(`(?i)inactive|disable|禁用|deny`).FindAllString(output, -1))

	totalPolicies := policyCount
	if totalPolicies == 0 {
		totalPolicies = activeCount
	}

	return ParsedResult{
		Type:      "security_policy",
		Success:   strings.TrimSpace(output) != "",
		Value:     totalPolicies,
		Status:    "normal",
		Details:   fmt.Sprintf("安全策略: %d 条 (活跃: %d, 非活跃: %d)", totalPolicies, activeCount, inactiveCount),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// parseNat NAT 策略解析
func parseNat(output string) ParsedResult {
	ruleCount := 0
	natRegex := regexp.MustCompile(`(?i)nat|source|destination|pool|pat|no-nat`)
	skipRegex := regexp.MustCompile(`(?i)^(---|#|Total)`)
	for _, line := range strings.Split(output, "\n") {
		if natRegex.MatchString(line) && strings.TrimSpace(line) != "" && !skipRegex.MatchString(line) {
			ruleCount++
		}
	}

	return ParsedResult{
		Type:      "nat",
		Success:   strings.TrimSpace(output) != "",
		Value:     ruleCount,
		Status:    "normal",
		Details:   fmt.Sprintf("NAT 规则数: %d", ruleCount),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

var sessionTotalRegex = regexp.MustCompile(`(?i)(?:total|current|active)\s*[:：]?\s*(\d+)`)
var sessionActiveRegex = regexp.MustCompile(`(?i)(?:active|established)\s*[:：]?\s*(\d+)`)

// parseSession 防火墙会话统计解析
func parseSession(output string) ParsedResult {
	var total, active int
	if m := sessionTotalRegex.FindStringSubmatch(output); m != nil {
		total, _ = strconv.Atoi(m[1])
	}
	if m := sessionActiveRegex.FindStringSubmatch(output); m != nil {
		active, _ = strconv.Atoi(m[1])
	} else {
		active = total
	}

	status := "normal"
	if total > 100000 {
		status = "critical"
	} else if total > 50000 {
		status = "warning"
	}

	return ParsedResult{
		Type:      "session",
		Success:   strings.TrimSpace(output) != "",
		Value:     total,
		Status:    status,
		Details:   fmt.Sprintf("会话总数: %d, 活跃: %d", total, active),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// parseVpn VPN 隧道状态解析
func parseVpn(output string) ParsedResult {
	upTunnels := len(regexp.MustCompile(`(?i)active|up|established|running`).FindAllString(output, -1))
	downTunnels := len(regexp.MustCompile(`(?i)down|inactive|failed|not\s*connected`).FindAllString(output, -1))
	totalTunnels := upTunnels + downTunnels

	status := "normal"
	if totalTunnels > 0 && downTunnels > 0 {
		if downTunnels > upTunnels {
			status = "critical"
		} else {
			status = "warning"
		}
	}

	return ParsedResult{
		Type:      "vpn",
		Success:   strings.TrimSpace(output) != "",
		Value:     totalTunnels,
		Status:    status,
		Details:   fmt.Sprintf("VPN 隧道: %d 个 (UP: %d, DOWN: %d)", totalTunnels, upTunnels, downTunnels),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// parseWlan 无线客户端/射频解析
func parseWlan(output string) ParsedResult {
	clientMatch := regexp.MustCompile(`(?i)(?:client|station|user|用户)\s*[:：]?\s*(\d+)`).FindStringSubmatch(output)
	ssidCount := len(regexp.MustCompile(`(?i)SSID\s*[:：]?\s*\S+`).FindAllString(output, -1))
	apCount := 0
	if m := regexp.MustCompile(`(?i)(?:AP|access.point|接入点)\s*[:：]?\s*(\d+)`).FindStringSubmatch(output); m != nil {
		apCount, _ = strconv.Atoi(m[1])
	} else {
		apCount = len(regexp.MustCompile(`(?i)\bAP\b`).FindAllString(output, -1))
	}

	clients := 0
	if clientMatch != nil {
		clients, _ = strconv.Atoi(clientMatch[1])
	}

	return ParsedResult{
		Type:      "wlan",
		Success:   strings.TrimSpace(output) != "",
		Value:     clients,
		Status:    "normal",
		Details:   fmt.Sprintf("无线: %d 客户端, %d SSID, %d AP", clients, ssidCount, apCount),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// parsePool DHCP 地址池解析
func parsePool(output string) ParsedResult {
	poolRegex := regexp.MustCompile(`(?i)pool|network|range|mask|gateway|lease`)
	poolLines := 0
	for _, line := range strings.Split(output, "\n") {
		if poolRegex.MatchString(line) && strings.TrimSpace(line) != "" {
			poolLines++
		}
	}

	usedMatch := regexp.MustCompile(`(?i)(?:used|used\s*address|已用)\s*[:：]?\s*(\d+)`).FindStringSubmatch(output)
	totalMatch := regexp.MustCompile(`(?i)(?:total|total\s*address|总数)\s*[:：]?\s*(\d+)`).FindStringSubmatch(output)

	used := 0
	total := poolLines
	if usedMatch != nil {
		used, _ = strconv.Atoi(usedMatch[1])
	}
	if totalMatch != nil {
		total, _ = strconv.Atoi(totalMatch[1])
	}

	utilization := 0
	if total > 0 {
		utilization = used * 100 / total
	}

	status := "normal"
	if utilization > 90 {
		status = "critical"
	} else if utilization > 75 {
		status = "warning"
	}

	return ParsedResult{
		Type:      "pool",
		Success:   strings.TrimSpace(output) != "",
		Value:     utilization,
		Unit:      "%",
		Status:    status,
		Details:   fmt.Sprintf("DHCP 地址池: %d/%d 已用 (%d%%)", used, total, utilization),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// parseDns DNS 配置解析
func parseDns(output string) ParsedResult {
	servers := regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`).FindAllString(output, -1)
	uniqueSet := make(map[string]bool)
	for _, s := range servers {
		uniqueSet[s] = true
	}
	uniqueCount := len(uniqueSet)

	domainMatch := regexp.MustCompile(`(?i)domain\s+([\w.-]+)`).FindStringSubmatch(output)

	details := fmt.Sprintf("DNS 服务器: %d 个", uniqueCount)
	if domainMatch != nil {
		details += fmt.Sprintf(", 域名: %s", domainMatch[1])
	}

	status := "normal"
	if uniqueCount == 0 {
		status = "warning"
	}

	return ParsedResult{
		Type:      "dns",
		Success:   strings.TrimSpace(output) != "",
		Value:     uniqueCount,
		Status:    status,
		Details:   details,
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// parseBgp BGP 邻居解析
func parseBgp(output string) ParsedResult {
	establishedCount := len(regexp.MustCompile(`(?i)Established`).FindAllString(output, -1))
	idleCount := len(regexp.MustCompile(`(?i)Idle`).FindAllString(output, -1))
	activeCount := len(regexp.MustCompile(`(?i)\bActive\b`).FindAllString(output, -1))
	totalCount := establishedCount + idleCount + activeCount

	status := "normal"
	if totalCount > 0 && establishedCount == 0 {
		status = "critical"
	} else if idleCount > 0 || activeCount > 0 {
		status = "warning"
	}

	return ParsedResult{
		Type:      "bgp",
		Success:   strings.TrimSpace(output) != "",
		Value:     establishedCount,
		Status:    status,
		Details:   fmt.Sprintf("BGP 邻居: %d 个 (Established: %d, Idle: %d, Active: %d)", totalCount, establishedCount, idleCount, activeCount),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// parseOspf OSPF 邻居解析
func parseOspf(output string) ParsedResult {
	fullCount := len(regexp.MustCompile(`(?i)Full`).FindAllString(output, -1))
	exStartCount := len(regexp.MustCompile(`(?i)ExStart|Exchange|Loading`).FindAllString(output, -1))
	initCount := len(regexp.MustCompile(`(?i)Init|Down`).FindAllString(output, -1))
	totalCount := fullCount + exStartCount + initCount

	status := "normal"
	if totalCount > 0 && fullCount == 0 {
		status = "critical"
	} else if exStartCount > 0 || initCount > 0 {
		status = "warning"
	}

	return ParsedResult{
		Type:      "ospf",
		Success:   strings.TrimSpace(output) != "",
		Value:     fullCount,
		Status:    status,
		Details:   fmt.Sprintf("OSPF 邻居: %d 个 (Full: %d, ExStart/Exchange: %d, Init/Down: %d)", totalCount, fullCount, exStartCount, initCount),
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// parseNtp NTP 状态解析
func parseNtp(output string) ParsedResult {
	synced := regexp.MustCompile(`(?i)synchronized|synced|已同步|\*`).MatchString(output)
	serverMatch := regexp.MustCompile(`(?i)\*([\d.]+)|synchronized\s+to\s+([\d.]+)`).FindStringSubmatch(output)
	stratumMatch := regexp.MustCompile(`(?i)stratum\s+(\d+)`).FindStringSubmatch(output)

	stratum := 0
	if stratumMatch != nil {
		stratum, _ = strconv.Atoi(stratumMatch[1])
	}

	status := "normal"
	if !synced {
		status = "warning"
	}

	details := "NTP 未同步"
	if synced {
		server := ""
		if serverMatch != nil {
			if serverMatch[1] != "" {
				server = serverMatch[1]
			} else {
				server = serverMatch[2]
			}
		}
		details = "NTP 已同步"
		if server != "" {
			details += fmt.Sprintf(", 服务器: %s", server)
		}
		if stratum > 0 {
			details += fmt.Sprintf(", Stratum: %d", stratum)
		}
	}

	return ParsedResult{
		Type:      "ntp",
		Success:   strings.TrimSpace(output) != "",
		Value:     stratum,
		Status:    status,
		Details:   details,
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// parseLicense License 有效期解析
func parseLicense(output string) ParsedResult {
	activeMatch := regexp.MustCompile(`(?i)active|valid|enabled|激活|有效`).MatchString(output)
	expiredMatch := regexp.MustCompile(`(?i)expired|invalid|disabled|过期|无效`).MatchString(output)
	expiryMatch := regexp.MustCompile(`(?i)(?:expiry|expire|valid\s*until|到期)\s*[:：]?\s*(\d{4}[-/]\d{2}[-/]\d{2})`).FindStringSubmatch(output)

	status := "normal"
	if expiredMatch {
		status = "critical"
	} else if !activeMatch {
		status = "warning"
	}

	value := "unknown"
	if expiryMatch != nil {
		value = expiryMatch[1]
	} else if activeMatch {
		value = "active"
	}

	var details string
	if expiredMatch {
		if expiryMatch != nil {
			details = fmt.Sprintf("License 已过期 (到期日: %s)", expiryMatch[1])
		} else {
			details = "License 已过期"
		}
	} else if activeMatch {
		if expiryMatch != nil {
			details = fmt.Sprintf("License 有效, 到期日: %s", expiryMatch[1])
		} else {
			details = "License 有效"
		}
	} else {
		details = "License 状态未知"
	}

	return ParsedResult{
		Type:      "license",
		Success:   strings.TrimSpace(output) != "",
		Value:     value,
		Status:    status,
		Details:   details,
		RawOutput: truncateOutput(output, 1000),
		Timestamp: nowISO(),
	}
}

// parseConfigChecksum 配置 MD5 快照解析
func parseConfigChecksum(output string) ParsedResult {
	hashMatch := regexp.MustCompile(`[0-9a-fA-F]{32}`).FindString(output)
	hash := ""
	if hashMatch != "" {
		hash = hashMatch
	} else {
		hash = truncateOutput(strings.TrimSpace(output), 32)
	}

	return ParsedResult{
		Type:      "config_checksum",
		Success:   strings.TrimSpace(output) != "",
		Value:     hash,
		Status:    "normal",
		Details:   fmt.Sprintf("配置 MD5: %s...", truncateOutput(hash, 16)),
		RawOutput: truncateOutput(output, 500),
		Timestamp: nowISO(),
	}
}

// ───────────────────────────────────────────────
// 解析器分派（getParser）
// ───────────────────────────────────────────────

// extendedParsers 通用扩展解析器（厂商无关）
var extendedParsers = map[string]ParseFunc{
	"optic":           parseOptic,
	"neighbor":        parseNeighbor,
	"security_policy": parseSecurityPolicy,
	"nat":             parseNat,
	"session":         parseSession,
	"vpn":             parseVpn,
	"wlan":            parseWlan,
	"pool":            parsePool,
	"dns":             parseDns,
	"bgp":             parseBgp,
	"ospf":            parseOspf,
	"ntp":             parseNtp,
	"license":         parseLicense,
	"config_checksum": parseConfigChecksum,
}

// standardParsers 标准解析器（按厂商）
var standardParsers = map[string]map[string]ParseFunc{
	"huawei": {
		"cpu":          parseHuaweiCpu,
		"memory":       parseHuaweiMemory,
		"interface":    parseInterfaceBrief,
		"version":      parseVersion,
		"routes":       parseRoutes,
		"log":          parseLogBuffer,
		"environment":  parseEnvironment,
		"power":        parsePower,
		"fan":          parseFan,
		"stp":          parseStp,
		"vlan":         parseVlan,
		"arp":          parseArp,
		"mac":          parseMac,
	},
	"cisco": {
		"cpu":          parseCiscoCpu,
		"memory":       parseCiscoMemory,
		"interface":    parseInterfaceBrief,
		"version":      parseVersion,
		"routes":       parseRoutes,
		"log":          parseLogBuffer,
		"environment":  parseEnvironment,
		"power":        parsePower,
		"fan":          parseFan,
		"stp":          parseStp,
		"vlan":         parseVlan,
		"arp":          parseArp,
		"mac":          parseMac,
	},
	"h3c": {
		"cpu":          parseHuaweiCpu, // 同华为
		"memory":       parseHuaweiMemory,
		"interface":    parseInterfaceBrief,
		"version":      parseVersion,
		"routes":       parseRoutes,
		"log":          parseLogBuffer,
		"environment":  parseEnvironment,
		"power":        parsePower,
		"fan":          parseFan,
		"stp":          parseStp,
		"vlan":         parseVlan,
		"arp":          parseArp,
		"mac":          parseMac,
	},
	"ruijie": {
		"cpu":          parseRuijieCpu,
		"memory":       parseCiscoMemory, // 同思科
		"interface":    parseInterfaceBrief,
		"version":      parseVersion,
		"routes":       parseRoutes,
		"log":          parseLogBuffer,
		"environment":  parseEnvironment,
		"power":        parsePower,
		"fan":          parseFan,
		"stp":          parseStp,
		"vlan":         parseVlan,
		"arp":          parseArp,
		"mac":          parseMac,
	},
	"zte": {
		"cpu":          parseRuijieCpu, // 同锐捷
		"memory":       parseCiscoMemory,
		"interface":    parseInterfaceBrief,
		"version":      parseVersion,
		"routes":       parseRoutes,
		"log":          parseLogBuffer,
		"environment":  parseEnvironment,
		"power":        parsePower,
		"fan":          parseFan,
		"stp":          parseStp,
		"vlan":         parseVlan,
		"arp":          parseArp,
		"mac":          parseMac,
	},
}

// newVendorCompatParsers 新厂商使用的兼容解析器
func newVendorCompatParsers(vendor string) map[string]ParseFunc {
	cpuParser := parseHuaweiCpu
	memParser := parseHuaweiMemory
	if vendor == "fortinet" || vendor == "paloalto" || vendor == "f5" {
		cpuParser = parseCiscoCpu
		memParser = parseCiscoMemory
	}

	m := map[string]ParseFunc{
		"cpu":         cpuParser,
		"memory":      memParser,
		"interface":   parseInterfaceBrief,
		"version":     parseVersion,
		"routes":      parseRoutes,
		"log":         parseLogBuffer,
		"environment": parseEnvironment,
		"power":       parsePower,
		"fan":         parseFan,
		"stp":         parseStp,
		"vlan":        parseVlan,
		"arp":         parseArp,
		"mac":         parseMac,
	}
	// 合并扩展解析器
	for k, v := range extendedParsers {
		m[k] = v
	}
	return m
}

// newVendorSet 判断是否是新增厂商
var newVendors = map[string]bool{
	"fortinet": true, "paloalto": true, "juniper": true, "arista": true,
	"hpe": true, "mikrotik": true, "ubiquiti": true, "dell": true,
	"tplink": true, "f5": true, "ruijie_eg": true,
}

// GetParser 根据厂商和巡检类型获取解析器
func GetParser(vendor string, inspectionType string) ParseFunc {
	// 新增厂商使用兼容解析器
	if newVendors[vendor] {
		parsers := newVendorCompatParsers(vendor)
		if fn, ok := parsers[inspectionType]; ok {
			return fn
		}
		return parseCustom
	}

	// 已有厂商
	if vendorParsers, ok := standardParsers[vendor]; ok {
		if fn, ok := vendorParsers[inspectionType]; ok {
			return fn
		}
	}

	// 扩展解析器
	if fn, ok := extendedParsers[inspectionType]; ok {
		return fn
	}

	return parseCustom
}
