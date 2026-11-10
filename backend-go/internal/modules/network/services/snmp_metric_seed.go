// SNMP 指标定义自动派生 + 标准 IF-MIB 种子
//
// 放在 services 包中（可同时访问 oid_registry 与 repos），避免 repos→services 的循环依赖。
//
// 目标：让 snmp_metric_definitions 在缺乏 MIB 上传功能时也能开箱即用：
//   1) 从 app.db.mib_objects（用户上传/迁移的 MIB）自动派生命名指标定义
//      - 标量 OBJECT-TYPE → is_table=0
//      - 表列 OBJECT-TYPE → is_table=1，并从其所属表抽取索引/描述列作为标签规则
//   2) 若标准 IF-MIB 定义尚未存在，注入一套权威的内置定义（采用 oid_registry.go 的 OID）
//
// 该逻辑在 metrics 迁移之后调用一次，保证 snmp_metric_samples 写入时有可用的命名体系。
package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"itops-backend/internal/appdb"
	"itops-backend/internal/repos"
)

// SnmpMetricSeedLog 种子阶段日志回调（可选注入）
var SnmpMetricSeedLog = func(format string, args ...interface{}) {}

// SeedSnmpMetricDefinitions 执行指标定义派生：从用户解析上传的 mib_objects 动态生成命名指标库。
// 幂等：依赖 metric_name 唯一约束，重复调用不会插入重复行。
func SeedSnmpMetricDefinitions() error {
	// 仅从 oid.db.mib_objects 动态派生（完全基于用户解析上传的 MIB Excel/文件）
	if err := deriveFromMibObjects(); err != nil {
		SnmpMetricSeedLog("derive from mib_objects failed: %v", err)
	}
	return nil
}

// ───────────────────────────────────────────────
// 内置标准 IF-MIB 定义
// ───────────────────────────────────────────────

type builtinMetricDef struct {
	MetricName  string
	OID         string
	Description string
	MetricType  string
	Unit        string
	IsTable     int
	IndexTag    string
}

// builtinIfMibDefs 标准 IF-MIB 指标（snmp_exporter 风格命名）
var builtinIfMibDefs = []builtinMetricDef{
	{"sysDescr", SystemOIDs.SysDescr, "系统描述", "gauge", "", 0, ""},
	{"sysUptime", SystemOIDs.SysUptime, "系统运行时间（百分之一秒）", "counter", "s", 0, ""},
	{"sysName", SystemOIDs.SysName, "系统名称", "gauge", "", 0, ""},
	{"sysLocation", SystemOIDs.SysLocation, "系统位置", "gauge", "", 0, ""},
	{"ifDescr", IfMibOIDs.IfDescr, "接口描述", "gauge", "", 1, "ifIndex"},
	{"ifType", IfMibOIDs.IfType, "接口类型", "gauge", "", 1, "ifIndex"},
	{"ifMtu", IfMibOIDs.IfMtu, "接口 MTU", "gauge", "bytes", 1, "ifIndex"},
	{"ifSpeed", IfMibOIDs.IfSpeed, "接口速率", "gauge", "bps", 1, "ifIndex"},
	{"ifAdminStatus", IfMibOIDs.IfAdminStatus, "接口管理状态", "gauge", "", 1, "ifIndex"},
	{"ifOperStatus", IfMibOIDs.IfOperStatus, "接口运营状态", "gauge", "", 1, "ifIndex"},
	{"ifInOctets", IfMibOIDs.IfInOctets, "接口入向字节数", "counter", "bytes", 1, "ifIndex"},
	{"ifInUcastPkts", IfMibOIDs.IfInUcastPkts, "接口入向单播包数", "counter", "pkts", 1, "ifIndex"},
	{"ifInErrors", IfMibOIDs.IfInErrors, "接口入向错误数", "counter", "pkts", 1, "ifIndex"},
	{"ifOutOctets", IfMibOIDs.IfOutOctets, "接口出向字节数", "counter", "bytes", 1, "ifIndex"},
	{"ifOutUcastPkts", IfMibOIDs.IfOutUcastPkts, "接口出向单播包数", "counter", "pkts", 1, "ifIndex"},
	{"ifOutErrors", IfMibOIDs.IfOutErrors, "接口出向错误数", "counter", "pkts", 1, "ifIndex"},
	{"ifHighSpeed", IfMibOIDs.IfHighSpeed, "接口高速率", "gauge", "Mbps", 1, "ifIndex"},
	{"ifHCInOctets", IfMibOIDs.IfHCInOctets, "接口入向字节数(HC)", "counter", "bytes", 1, "ifIndex"},
	{"ifHCOutOctets", IfMibOIDs.IfHCOutOctets, "接口出向字节数(HC)", "counter", "bytes", 1, "ifIndex"},
	{"ifName", IfMibOIDs.IfName, "接口名称", "gauge", "", 1, "ifIndex"},
	{"ifAlias", IfMibOIDs.IfAlias, "接口别名", "gauge", "", 1, "ifIndex"},
	{"ifIndex", IfMibOIDs.IfIndex, "接口索引列", "gauge", "", 1, "ifIndex"},
}

// seedBuiltinIfMib 注入内置标准 IF-MIB 定义（source=mib, module=IF-MIB）
func seedBuiltinIfMib() error {
	count := 0
	for _, d := range builtinIfMibDefs {
		def := &repos.SnmpMetricDefinition{
			MetricName:  d.MetricName,
			ModuleName:  "IF-MIB",
			OID:         d.OID,
			Description: d.Description,
			MetricType:  d.MetricType,
			IsTable:     d.IsTable,
			IndexTag:    d.IndexTag,
			Source:      "mib",
		}
		if d.IsTable == 1 {
			// 表列：附加上 ifDescr 作为可读标签
			def.LabelOIDs = mustJSONStr([]string{IfMibOIDs.IfDescr})
			def.LabelNames = mustJSONStr([]string{"ifDescr"})
		}
		if d.Unit != "" {
			def.Unit = d.Unit
		}
		if err := repos.SnmpMetricDefs.Upsert(def); err != nil {
			return fmt.Errorf("upsert builtin %s: %w", d.MetricName, err)
		}
		count++
	}
	SnmpMetricSeedLog("seeded %d builtin IF-MIB metric definitions", count)
	return nil
}

// ───────────────────────────────────────────────
// 从 app.db.mib_objects 自动派生
// ───────────────────────────────────────────────

// deriveFromMibObjects 遍历 oid.db mib_objects，对 OBJECT-TYPE 生成命名指标定义。
// 依赖 oid.db 已初始化（mib_objects 表存在）。
func deriveFromMibObjects() error {
	oidDB := appdb.OidDB()
	if oidDB == nil {
		return fmt.Errorf("oid database not initialized")
	}
	rows, err := oidDB.Query(`SELECT id, module_id, module_name, oid, name, syntax, access, status, description, COALESCE(table_name, ''), is_table, is_scalar
		FROM mib_objects`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type mibObj struct {
		id          string
		moduleID    string
		moduleName  string
		oid         string
		name        string
		syntax      string
		access      string
		status      string
		description string
		tableName   string
		isTable     int
		isScalar    int
	}

	objectsByTable := map[string][]mibObj{}
	allObjects := []mibObj{}

	for rows.Next() {
		var o mibObj
		var id, moduleID, moduleName, oid, name, syntax, access, status, desc, tableName sqlStr
		var isTable, isScalar sqlInt
		if err := rows.Scan(&id.s, &moduleID.s, &moduleName.s, &oid.s, &name.s,
			&syntax.s, &access.s, &status.s, &desc.s, &tableName.s, &isTable.i, &isScalar.i); err != nil {
			continue
		}
		o.id = id.s
		o.moduleID = moduleID.s
		o.moduleName = moduleName.s
		o.oid = oid.s
		o.name = name.s
		o.syntax = syntax.s
		o.access = access.s
		o.status = status.s
		o.description = desc.s
		o.tableName = tableName.s
		o.isTable = int(isTable.i)
		o.isScalar = int(isScalar.i)
		allObjects = append(allObjects, o)
		if o.tableName != "" {
			objectsByTable[o.tableName] = append(objectsByTable[o.tableName], o)
		}
	}

	// 找出每个表的索引列与描述列
	tableMeta := map[string]struct {
		indexOID  string
		indexName string
		labelOIDs []string
		labelNams []string
	}{}

	for tbl, cols := range objectsByTable {
		meta := struct {
			indexOID  string
			indexName string
			labelOIDs []string
			labelNams []string
		}{}
		for _, c := range cols {
			n := strings.ToLower(c.name)
			if c.access == "index" || strings.HasSuffix(c.name, "Index") || strings.EqualFold(c.name, "ifIndex") {
				if meta.indexOID == "" {
					meta.indexOID = c.oid
					meta.indexName = c.name
				}
			}
			if strings.Contains(n, "descr") || strings.Contains(n, "name") || strings.Contains(n, "alias") {
				if !containsStr(meta.labelOIDs, c.oid) {
					meta.labelOIDs = append(meta.labelOIDs, c.oid)
					meta.labelNams = append(meta.labelNams, c.name)
				}
			}
		}
		_ = tbl
		tableMeta[tbl] = meta
	}

	derived := 0
	for _, o := range allObjects {
		def := &repos.SnmpMetricDefinition{
			MetricName:  o.name,
			ModuleName:  o.moduleName,
			OID:         o.oid,
			Description: o.description,
			MetricType:  metricTypeFromSyntax(o.syntax),
			Source:      "mib",
		}
		if o.isTable == 1 && o.tableName != "" {
			def.IsTable = 1
			if m, ok := tableMeta[o.tableName]; ok {
				def.IndexOID = m.indexOID
				def.IndexTag = m.indexName
				def.LabelOIDs = mustJSONStr(m.labelOIDs)
				def.LabelNames = mustJSONStr(m.labelNams)
			}
		} else {
			def.IsTable = 0
		}
		if err := repos.SnmpMetricDefs.Upsert(def); err != nil {
			SnmpMetricSeedLog("upsert derived %s failed: %v", o.name, err)
			continue
		}
		derived++
	}
	SnmpMetricSeedLog("derived %d metric definitions from mib_objects", derived)
	return nil
}

// metricTypeFromSyntax 根据 MIB SYNTAX 推断 Prometheus 风格类型
func metricTypeFromSyntax(syntax string) string {
	s := strings.ToLower(syntax)
	if strings.Contains(s, "counter") || strings.Contains(s, "counter32") || strings.Contains(s, "counter64") {
		return "counter"
	}
	return "gauge"
}

// ───────────────────────────────────────────────
// 辅助
// ───────────────────────────────────────────────

func mustJSONStr(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// 轻量 Null 类型（保持本文件自洽，避免依赖 repos 内部辅助）
type sqlStr struct{ s string }
type sqlInt struct{ i int64 }

// ───────────────────────────────────────────────
// 内置公有及多厂商 MIB 预置
// ───────────────────────────────────────────────

type builtinMibObject struct {
	Name        string
	OID         string
	Syntax      string
	Description string
	IsTable     int
	IsScalar    int
}

type builtinMibModule struct {
	ID          string
	Name        string
	FileName    string
	Description string
	Objects     []builtinMibObject
}

var builtinMibModules = []builtinMibModule{
	{
		ID:          "builtin-if-mib",
		Name:        "IF-MIB",
		FileName:    "IF-MIB (RFC 2863)",
		Description: "标准接口 MIB 模块，包含网络接口状态、进出流量、错包与速率信息",
		Objects: []builtinMibObject{
			{"ifIndex", "1.3.6.1.2.1.2.2.1.1", "INTEGER", "接口索引列", 1, 0},
			{"ifDescr", "1.3.6.1.2.1.2.2.1.2", "DisplayString", "接口描述字符串", 1, 0},
			{"ifType", "1.3.6.1.2.1.2.2.1.3", "INTEGER", "接口物理/逻辑类型", 1, 0},
			{"ifMtu", "1.3.6.1.2.1.2.2.1.4", "INTEGER", "接口最大传输单元 MTU", 1, 0},
			{"ifSpeed", "1.3.6.1.2.1.2.2.1.5", "Gauge32", "接口标准传输速率 (bps)", 1, 0},
			{"ifPhysAddress", "1.3.6.1.2.1.2.2.1.6", "PhysAddress", "接口物理 MAC 地址", 1, 0},
			{"ifAdminStatus", "1.3.6.1.2.1.2.2.1.7", "INTEGER", "接口管理状态 (1:UP, 2:DOWN)", 1, 0},
			{"ifOperStatus", "1.3.6.1.2.1.2.2.1.8", "INTEGER", "接口运行状态 (1:UP, 2:DOWN)", 1, 0},
			{"ifInOctets", "1.3.6.1.2.1.2.2.1.10", "Counter32", "接口入向累计字节数 (In Octets)", 1, 0},
			{"ifInUcastPkts", "1.3.6.1.2.1.2.2.1.11", "Counter32", "接口入向单播数据包数", 1, 0},
			{"ifInErrors", "1.3.6.1.2.1.2.2.1.14", "Counter32", "接口入向错误包总数", 1, 0},
			{"ifOutOctets", "1.3.6.1.2.1.2.2.1.16", "Counter32", "接口出向累计字节数 (Out Octets)", 1, 0},
			{"ifOutUcastPkts", "1.3.6.1.2.1.2.2.1.17", "Counter32", "接口出向单播数据包数", 1, 0},
			{"ifOutErrors", "1.3.6.1.2.1.2.2.1.20", "Counter32", "接口出向错误包总数", 1, 0},
			{"ifHighSpeed", "1.3.6.1.2.1.31.1.1.1.15", "Gauge32", "千兆/万兆接口高速率 (Mbps)", 1, 0},
			{"ifHCInOctets", "1.3.6.1.2.1.31.1.1.1.6", "Counter64", "64位高容量接口入向字节数", 1, 0},
			{"ifHCOutOctets", "1.3.6.1.2.1.31.1.1.1.10", "Counter64", "64位高容量接口出向字节数", 1, 0},
			{"ifName", "1.3.6.1.2.1.31.1.1.1.1", "DisplayString", "接口简称/名称 (GE0/0/1)", 1, 0},
			{"ifAlias", "1.3.6.1.2.1.31.1.1.1.18", "DisplayString", "接口别名/注释说明", 1, 0},
		},
	},
	{
		ID:          "builtin-snmpv2-mib",
		Name:        "SNMPv2-MIB",
		FileName:    "SNMPv2-MIB (RFC 3418)",
		Description: "标准系统 MIB 模块，包含设备名称、描述、运行时间及位置",
		Objects: []builtinMibObject{
			{"sysDescr", "1.3.6.1.2.1.1.1.0", "DisplayString", "系统硬件与软件描述", 0, 1},
			{"sysObjectID", "1.3.6.1.2.1.1.2.0", "OBJECT IDENTIFIER", "系统厂商唯一 OID 标识", 0, 1},
			{"sysUpTime", "1.3.6.1.2.1.1.3.0", "TimeTicks", "系统启动运行时间 (厘秒)", 0, 1},
			{"sysContact", "1.3.6.1.2.1.1.4.0", "DisplayString", "系统管理员联系方式", 0, 1},
			{"sysName", "1.3.6.1.2.1.1.5.0", "DisplayString", "系统的主机名/网络标识名", 0, 1},
			{"sysLocation", "1.3.6.1.2.1.1.6.0", "DisplayString", "设备物理安装位置", 0, 1},
		},
	},
	{
		ID:          "builtin-huawei-mib",
		Name:        "HUAWEI-MIB",
		FileName:    "HUAWEI-ENTITY-EXT-MIB",
		Description: "华为交换机与路由器私有硬件监控 MIB (CPU/内存/风扇/电源/温度/光模块)",
		Objects: []builtinMibObject{
			{"hwCpuDevUsage", "1.3.6.1.4.1.2011.6.3.4.1.2.0", "INTEGER", "华为设备 CPU 平均利用率 (%)", 0, 1},
			{"hwMemoryDevUsage", "1.3.6.1.4.1.2011.6.3.5.1.1.2.0", "INTEGER", "华为设备内存利用率 (%)", 0, 1},
			{"hwEntityTemperature", "1.3.6.1.4.1.2011.6.3.4.1.1.2", "INTEGER", "华为主板/槽位温度 (摄氏度)", 1, 0},
			{"hwEntityPowerStatus", "1.3.6.1.4.1.2011.6.3.4.1.3", "INTEGER", "华为电源模块状态 (1:normal, 2:abnormal)", 1, 0},
			{"hwEntityFanStatus", "1.3.6.1.4.1.2011.6.3.4.1.1.3", "INTEGER", "华为风扇模块状态 (1:normal, 2:abnormal)", 1, 0},
			{"hwOpticalModuleInfoRxPower", "1.3.6.1.4.1.2011.10.2.6.1.1.1.1.5", "INTEGER", "华为光模块接收光功率 (dBm)", 1, 0},
			{"hwOpticalModuleInfoTxPower", "1.3.6.1.4.1.2011.10.2.6.1.1.1.1.6", "INTEGER", "华为光模块发送光功率 (dBm)", 1, 0},
			{"hwOpticalModuleInfoTemperature", "1.3.6.1.4.1.2011.10.2.6.1.1.1.1.4", "INTEGER", "华为光模块工作温度 (摄氏度)", 1, 0},
		},
	},
	{
		ID:          "builtin-h3c-mib",
		Name:        "HH3C-ENTITY-EXT-MIB",
		FileName:    "HH3C-ENTITY-EXT-MIB",
		Description: "华三 H3C 网络设备私有硬件监控 MIB (CPU/内存/风扇/电源/温度/光模块)",
		Objects: []builtinMibObject{
			{"hh3cEntityExtCpuUsage", "1.3.6.1.4.1.25506.2.6.1.1.1.1.6.1", "INTEGER", "华三设备 CPU 平均利用率 (%)", 0, 1},
			{"hh3cEntityExtMemUsage", "1.3.6.1.4.1.25506.2.6.1.1.1.1.8.1", "INTEGER", "华三设备内存利用率 (%)", 0, 1},
			{"hh3cEntityExtTemperature", "1.3.6.1.4.1.25506.2.6.1.1.1.1.12", "INTEGER", "华三板卡/设备环境温度 (摄氏度)", 1, 0},
			{"hh3cEntityExtPowerStatus", "1.3.6.1.4.1.25506.2.6.1.1.1.1.10", "INTEGER", "华三电源模块运行状态", 1, 0},
			{"hh3cEntityExtFanStatus", "1.3.6.1.4.1.25506.2.6.1.1.1.1.11", "INTEGER", "华三风扇组运行状态", 1, 0},
			{"hh3cTransceiverRxPower", "1.3.6.1.4.1.25506.2.70.1.1.1.6", "INTEGER", "华三光模块接收光功率", 1, 0},
			{"hh3cTransceiverTxPower", "1.3.6.1.4.1.25506.2.70.1.1.1.5", "INTEGER", "华三光模块发送光功率", 1, 0},
			{"hh3cTransceiverTemperature", "1.3.6.1.4.1.25506.2.70.1.1.1.4", "INTEGER", "华三光模块当前温度", 1, 0},
		},
	},
	{
		ID:          "builtin-cisco-mib",
		Name:        "CISCO-PROCESS-MIB",
		FileName:    "CISCO-PROCESS-MIB",
		Description: "思科 Cisco 设备硬件监控 MIB (CPU 5秒/1分/5分, 内存池, 环境监控)",
		Objects: []builtinMibObject{
			{"cpmCPUTotal5sec", "1.3.6.1.4.1.9.9.109.1.1.1.1.7.1", "Gauge32", "思科 5秒内 CPU 占用率 (%)", 0, 1},
			{"cpmCPUTotal1min", "1.3.6.1.4.1.9.9.109.1.1.1.1.8.1", "Gauge32", "思科 1分钟内 CPU 平均占用率 (%)", 0, 1},
			{"cpmCPUTotal5min", "1.3.6.1.4.1.9.9.109.1.1.1.1.6.1", "Gauge32", "思科 5分钟内 CPU 平均占用率 (%)", 0, 1},
			{"ciscoMemoryPoolFree", "1.3.6.1.4.1.9.9.48.1.1.1.6.1", "Gauge32", "思科内存池剩余字节数", 0, 1},
			{"ciscoMemoryPoolUsed", "1.3.6.1.4.1.9.9.48.1.1.1.5.1", "Gauge32", "思科内存池已用字节数", 0, 1},
			{"ciscoEnvMonTemperatureStatusValue", "1.3.6.1.4.1.9.9.13.1.3.1.3", "Gauge32", "思科系统环境温度", 1, 0},
			{"ciscoEnvMonSupplyState", "1.3.6.1.4.1.9.9.117.1.1.2.1.2", "INTEGER", "思科电源状态", 1, 0},
			{"ciscoEnvMonFanState", "1.3.6.1.4.1.9.9.117.1.1.3.1.2", "INTEGER", "思科风扇运行状态", 1, 0},
		},
	},
	{
		ID:          "builtin-ruijie-mib",
		Name:        "RUIJIE-SYSTEM-MIB",
		FileName:    "RUIJIE-SYSTEM-MIB",
		Description: "锐捷 Ruijie 设备私有硬件监控 MIB (CPU/内存/风扇/电源/温度)",
		Objects: []builtinMibObject{
			{"ruijieCpuUsage", "1.3.6.1.4.1.4881.1.1.10.2.35.1.1.1.0", "INTEGER", "锐捷设备 CPU 利用率 (%)", 0, 1},
			{"ruijieMemoryUsage", "1.3.6.1.4.1.4881.1.1.10.2.35.1.1.2.0", "INTEGER", "锐捷设备内存利用率 (%)", 0, 1},
			{"ruijieTemperature", "1.3.6.1.4.1.4881.1.1.10.2.35.1.1.3.0", "INTEGER", "锐捷设备环境温度", 0, 1},
			{"ruijiePowerStatus", "1.3.6.1.4.1.4881.1.1.10.2.35.1.1.4.0", "INTEGER", "锐捷设备电源模块状态", 0, 1},
			{"ruijieFanStatus", "1.3.6.1.4.1.4881.1.1.10.2.35.1.1.5.0", "INTEGER", "锐捷设备风扇模块状态", 0, 1},
		},
	},
	{
		ID:          "builtin-maipu-mib",
		Name:        "MAIPU-SYSTEM-MIB",
		FileName:    "MAIPU-SYSTEM-MIB",
		Description: "迈普 Maipu 设备私有硬件监控 MIB (CPU/内存/风扇/电源/温度)",
		Objects: []builtinMibObject{
			{"mpCpuUsage", "1.3.6.1.4.1.5651.1.2.1.1.1.0", "INTEGER", "迈普设备 CPU 利用率 (%)", 0, 1},
			{"mpMemoryUsage", "1.3.6.1.4.1.5651.1.2.1.1.2.0", "INTEGER", "迈普设备内存利用率 (%)", 0, 1},
			{"mpTemperature", "1.3.6.1.4.1.5651.1.2.1.1.3.0", "INTEGER", "迈普设备环境温度", 0, 1},
			{"mpPowerStatus", "1.3.6.1.4.1.5651.1.2.1.1.4.0", "INTEGER", "迈普电源模块状态", 0, 1},
			{"mpFanStatus", "1.3.6.1.4.1.5651.1.2.1.1.5.0", "INTEGER", "迈普风扇模块状态", 0, 1},
		},
	},
}

// SeedBuiltinMibModules 注入内置标准公有及主流厂商（华为、华三、思科、锐捷、迈普）MIB 模块与 OID 到 oid.db
func SeedBuiltinMibModules() error {
	db := appdb.OidDB()

	for _, mod := range builtinMibModules {
		// 1) 插入 MIB 模块
		_, err := db.Exec(`
			INSERT INTO mib_modules (id, name, file_name, description, object_count, uploaded_at)
			VALUES (?, ?, ?, ?, ?, datetime('now','localtime'))
			ON CONFLICT(id) DO UPDATE SET 
				name = excluded.name,
				file_name = excluded.file_name,
				description = excluded.description,
				object_count = excluded.object_count`,
			mod.ID, mod.Name, mod.FileName, mod.Description, len(mod.Objects))
		if err != nil {
			SnmpMetricSeedLog("insert builtin mib_module %s failed: %v", mod.Name, err)
			continue
		}

		// 2) 插入 MIB OID 对象
		for _, obj := range mod.Objects {
			objID := fmt.Sprintf("%s-%s", mod.ID, obj.Name)
			_, err := db.Exec(`
				INSERT INTO mib_objects (id, module_id, module_name, name, oid, syntax, description, is_table, is_scalar)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO UPDATE SET
					name = excluded.name,
					oid = excluded.oid,
					syntax = excluded.syntax,
					description = excluded.description,
					is_table = excluded.is_table,
					is_scalar = excluded.is_scalar`,
				objID, mod.ID, mod.Name, obj.Name, obj.OID, obj.Syntax, obj.Description, obj.IsTable, obj.IsScalar)
			if err != nil {
				SnmpMetricSeedLog("insert builtin mib_object %s failed: %v", obj.Name, err)
			}
		}
	}
	SnmpMetricSeedLog("seeded %d builtin MIB modules (IF-MIB, HUAWEI, H3C, CISCO, RUIJIE, MAIPU)", len(builtinMibModules))
	return nil
}
