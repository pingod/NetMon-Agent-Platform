// Package appdb 数据库路由辅助
// 提供按表名自动路由到正确数据库实例的能力
// 指标类表 → MetricsDB()，其余 → DB()
package appdb

import "database/sql"

// 指标类表名集合 — 这些表的数据存放在 metrics.db 中
var metricsTables = map[string]bool{
	"snmp_collect_results":    true,
	"snmp_collect_tasks":      true,
	"snmp_interface_metrics":  true,
	"snmp_custom_oid_results": true,
	"snmp_custom_oid_profiles": true,
	"snmp_trap_events":       true,
	"baseline_metrics":       true,
	"custom_dashboard_metrics": true,
	"custom_dashboard_lists": true,
	"server_metrics":         true,
	"mib_modules":            true,
	"mib_objects":            true,
	"snmp_polling_tasks":     true,
	"snmp_polling_snapshots": true,
	"snmp_metric_definitions": true,
	"snmp_metric_samples":    true,
	"dashboard_panels":       true,
}

// IsMetricsTable 判断表名是否属于指标数据库
func IsMetricsTable(table string) bool {
	return metricsTables[table]
}

// DBForTable 根据表名返回对应的数据库连接
// 指标类表 → MetricsDB()，其余 → DB()
// 如果 MetricsDB 未初始化，回退到 DB()
func DBForTable(table string) *sql.DB {
	if metricsTables[table] && IsMetricsInitialized() {
		return MetricsDB()
	}
	return DB()
}

// RegisterMetricsTable 注册额外的指标类表名（供外部模块使用）
func RegisterMetricsTable(table string) {
	metricsTables[table] = true
}
