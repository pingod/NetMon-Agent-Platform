// Package migrate 迁移注册中心，等价于 TS models/migrations/index.ts
//
// 策略说明：
//   - Go 后端与 TS 后端共享同一 SQLite 数据库(backend/data/app.db)。
//   - v001 负责在全新库上一次性建成与 TS v001-v065 最终状态完全一致的 schema。
//   - v002-v065 注册为与 TS 同名同版本的空操作迁移，仅用于对齐 schema_migrations 版本跟踪，
//     使 Go 与 TS 两套迁移框架可在同一数据库上无缝互换。
//   - 对已完成迁移的现有库，所有版本均已标记 applied，启动时零操作、数据完全保留。
package migrate

import (
	"database/sql"
	"fmt"
	"strings"

	"itops-backend/internal/logger"
	"itops-backend/internal/migrate/schema"
)

// isDuplicateColumnErr 判断 ALTER TABLE ADD COLUMN 是否仅因列已存在失败。
// 只有这类错误可以安全忽略，其他错误（权限、锁、语法、磁盘）必须向上返回。
func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "duplicate column name")
}

// addColumnIfMissing 逐条执行 ALTER，列已存在时忽略
func addColumnIfMissing(tx *sql.Tx, stmt string) error {
	if _, err := tx.Exec(stmt); err != nil && !isDuplicateColumnErr(err) {
		return fmt.Errorf("%s: %w", stmt, err)
	}
	return nil
}

// migrationName 与 TS schema_migrations 中记录的 name 完全一致（权威来源：数据库导出）
var migrationNames = map[int]string{
	1:  "initial_schema",
	2:  "add_api_provider_to_agents",
	3:  "add_ai_models_table",
	4:  "add_agent_model_fields",
	5:  "ssh_key_password_support",
	6:  "network_device_credentials",
	7:  "credentials_table",
	8:  "fix_users_id_type",
	9:  "network_complete_coverage",
	10: "snmp_channel",
	12: "Timezone migration to UTC+8",
	13: "Network device discovery (IP scan)",
	14: "Alert correlation/aggregation",
	15: "Add missing notification columns",
	16: "Create databases table for external DB connections",
	17: "approval_requests",
	18: "alert_auto_response",
	19: "workflow_engine_enhancement",
	20: "databases_table",
	22: "config_templates",
	23: "virtual_machines",
	24: "containers",
	25: "container_images",
	26: "volumes",
	27: "ai_remediations",
	28: "dc_infrastructure",
	29: "dc_lifecycle",
	30: "dc_room_monitor",
	31: "network_subnets",
	32: "device_manufacturers",
	33: "device_types",
	34: "device_type_slot_definitions",
	35: "dc_power_panels",
	36: "dc_power_feeds",
	37: "dc_cables",
	38: "tool_links",
	39: "tool_link_image",
	40: "alert_provider_configs",
	41: "dc_room_energy",
	42: "baseline_metrics",
	43: "alert_processing_records",
	44: "compose_projects",
	45: "image_registries",
	46: "docker_endpoints",
	47: "k8s_contexts",
	48: "auto_scale_tables",
	49: "vm_migrations",
	50: "vm_snapshot_policies",
	51: "ai_remediations_columns",
	52: "knowledge_base",
	53: "escalation_history",
	54: "alert_auto_analysis",
	55: "vm_management_tables",
	56: "custom_oid_profiles",
	57: "fix_snmp_interface_metrics_schema",
	58: "fix_snmp_inspection_status",
	59: "config_backup_tables",
	60: "mib_and_snmp_collect",
	61: "interface_description_and_influxdb",
	62: "custom_dashboard_metrics",
	63: "value_mappings",
	64: "custom_metrics_is_table",
	65: "custom_dashboard_lists",
	66: "poll_intervals",
	67: "inspection_templates",
	68: "servers_vendor_model",
	69: "agent_token_and_token_invalidation",
	70: "scripts_center_columns",
	71: "remediation_verification_task",
	72: "rca_confidence_and_platform_backups",
}

// 迁移版本号升序序列（v11、v21 在 TS 中即为空洞）
var migrationVersions = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	12, 13, 14, 15, 16, 17, 18, 19, 20,
	22, 23, 24, 25, 26, 27, 28, 29, 30, 31,
	32, 33, 34, 35, 36, 37, 38, 39, 40, 41,
	42, 43, 44, 45, 46, 47, 48, 49, 50,
	51, 52, 53, 54, 55, 56, 57, 58, 59, 60,
	61, 62, 63, 64, 65, 66, 67, 68, 69, 70,
	71, 72,
}

// execScript 在事务上执行多语句 SQL 脚本。
// modernc.org/sqlite 的 Exec 会依次编译并执行脚本内全部语句。
func execScript(tx *sql.Tx, script string) error {
	_, err := tx.Exec(script)
	return err
}

// buildAllMigrations 构造全部迁移定义
func buildAllMigrations() []Migration {
	noop := func(tx *sql.Tx) error { return nil }

	var migs []Migration
	for _, v := range migrationVersions {
		name := migrationNames[v]
		m := Migration{
			Version: v,
			Name:    name,
			Up:      noop,
			Down:    noop,
		}
		if v == 1 {
			m.Description = "Initial consolidated schema (equivalent to TS v001-v065 final state)"
			m.Up = func(tx *sql.Tx) error {
				logger.Infof("🔄 Creating consolidated database schema (fresh install)...")
				if err := execScript(tx, schema.ConsolidatedSQL); err != nil {
					return err
				}
				logger.Infof("✅ Consolidated schema created successfully")
				return nil
			}
		} else if v == 66 {
			m.Description = "Add system_poll_interval and interface_poll_interval to network_devices"
			m.Up = func(tx *sql.Tx) error {
				if err := addColumnIfMissing(tx, "ALTER TABLE network_devices ADD COLUMN system_poll_interval INTEGER NOT NULL DEFAULT 300"); err != nil {
					return err
				}
				return addColumnIfMissing(tx, "ALTER TABLE network_devices ADD COLUMN interface_poll_interval INTEGER NOT NULL DEFAULT 60")
			}
		} else if v == 67 {
			m.Description = "Create inspection_templates table for inspection command templates"
			m.Up = func(tx *sql.Tx) error {
				_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS inspection_templates (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	vendor TEXT DEFAULT 'other',
	category TEXT DEFAULT 'network',
	device_type TEXT DEFAULT 'unknown',
	os_type TEXT DEFAULT 'linux',
	description TEXT DEFAULT '',
	commands TEXT DEFAULT '[]',
	variables TEXT DEFAULT '[]',
	tags TEXT DEFAULT '[]',
	is_preset INTEGER DEFAULT 0,
	enabled INTEGER DEFAULT 1,
	usage_count INTEGER DEFAULT 0,
	created_by TEXT DEFAULT '',
	created_at TEXT DEFAULT (datetime('now','localtime')),
	updated_at TEXT DEFAULT (datetime('now','localtime'))
)`)
				if err != nil {
					return err
				}
				_, err = tx.Exec("CREATE INDEX IF NOT EXISTS idx_inspection_templates_category ON inspection_templates(category)")
				if err != nil {
					return err
				}
				_, err = tx.Exec("CREATE INDEX IF NOT EXISTS idx_inspection_templates_vendor ON inspection_templates(vendor)")
				if err != nil {
					return err
				}
				_, err = tx.Exec("CREATE INDEX IF NOT EXISTS idx_inspection_templates_device_type ON inspection_templates(device_type)")
				if err != nil {
					return err
				}
				_, err = tx.Exec("CREATE INDEX IF NOT EXISTS idx_inspection_templates_os_type ON inspection_templates(os_type)")
				return err
			}
		} else if v == 68 {
			m.Description = "Add vendor and model columns to servers table"
			m.Up = func(tx *sql.Tx) error {
				if err := addColumnIfMissing(tx, "ALTER TABLE servers ADD COLUMN vendor TEXT DEFAULT 'other'"); err != nil {
					return err
				}
				return addColumnIfMissing(tx, "ALTER TABLE servers ADD COLUMN model TEXT DEFAULT ''")
			}
		} else if v == 69 {
			m.Description = "Add agent_token, users.token_invalid_before and alerts.device_id"
			m.Up = func(tx *sql.Tx) error {
				alters := []string{
					"ALTER TABLE users ADD COLUMN token_invalid_before INTEGER NOT NULL DEFAULT 0",
					"ALTER TABLE alerts ADD COLUMN device_id TEXT DEFAULT ''",
					"ALTER TABLE servers ADD COLUMN agent_token TEXT DEFAULT ''",
				}
				for _, stmt := range alters {
					if err := addColumnIfMissing(tx, stmt); err != nil {
						return err
					}
				}
				return nil
			}
		} else if v == 70 {
			m.Description = "Add type/parameters/version columns to scripts for script center"
			m.Up = func(tx *sql.Tx) error {
				alters := []string{
					"ALTER TABLE scripts ADD COLUMN type TEXT NOT NULL DEFAULT 'shell'",
					"ALTER TABLE scripts ADD COLUMN parameters TEXT NOT NULL DEFAULT '[]'",
					"ALTER TABLE scripts ADD COLUMN version INTEGER NOT NULL DEFAULT 1",
				}
				for _, stmt := range alters {
					if err := addColumnIfMissing(tx, stmt); err != nil {
						return err
					}
				}
				// 旧 language 列的数据映射到新 type 列
				if _, err := tx.Exec(`UPDATE scripts SET type = language WHERE language IN ('shell','python','powershell')`); err != nil {
					return err
				}
				_, err := tx.Exec(`UPDATE scripts SET type = 'shell' WHERE language = 'bash'`)
				return err
			}
		} else if v == 71 {
			m.Description = "Add verification_task_id to remediation_executions for verification workflow linkage"
			m.Up = func(tx *sql.Tx) error {
				return addColumnIfMissing(tx, "ALTER TABLE remediation_executions ADD COLUMN verification_task_id TEXT")
			}
		} else if v == 72 {
			m.Description = "Add confidence to root_cause_analyses; create platform_backups table"
			m.Up = func(tx *sql.Tx) error {
				if err := addColumnIfMissing(tx, "ALTER TABLE root_cause_analyses ADD COLUMN confidence REAL DEFAULT 0"); err != nil {
					return err
				}
				_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS platform_backups (
	id TEXT PRIMARY KEY,
	filename TEXT NOT NULL,
	size_bytes INTEGER DEFAULT 0,
	source TEXT DEFAULT 'manual',
	created_by TEXT DEFAULT '',
	created_at DATETIME DEFAULT (datetime('now','localtime'))
)`)
				return err
			}
		}
		migs = append(migs, m)
	}
	return migs
}

// NewAppMigrationManager 创建并注册全部迁移的管理器
func NewAppMigrationManager(db *sql.DB) *Manager {
	m := NewManager(db)
	m.RegisterBatch(buildAllMigrations())
	return m
}

// RunMigrations 等价于 TS runMigrations(db)
func RunMigrations(db *sql.DB) error {
	m := NewAppMigrationManager(db)
	res := m.Migrate()
	if !res.Success {
		logger.Errorf("❌ Migration failed at v%d: %s", res.FailedVersion, res.ErrorMessage)
		return &MigrationError{Version: res.FailedVersion, Msg: res.ErrorMessage}
	}
	// 确保 custom_dashboard_metrics 包含 chart_type, metric_name 与 tags 列
	// 仅忽略“列已存在”，其余错误必须让启动失败（避免 schema 漂移后静默运行）
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("post-migration schema fix: begin: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE custom_dashboard_metrics ADD COLUMN chart_type TEXT DEFAULT 'auto'`,
		`ALTER TABLE custom_dashboard_metrics ADD COLUMN metric_name TEXT DEFAULT ''`,
		`ALTER TABLE custom_dashboard_metrics ADD COLUMN tags TEXT DEFAULT ''`,
	} {
		if err := addColumnIfMissing(tx, stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("post-migration schema fix: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("post-migration schema fix: commit: %w", err)
	}
	return nil
}

// RunMetricsMigrations 指标数据库迁移
func RunMetricsMigrations(db *sql.DB) error {
	logger.Infof("Running metrics database migrations...")
	_, err := db.Exec(schema.MetricsSQL)
	if err != nil {
		logger.Errorf("Failed to create metrics schema: %v", err)
		return err
	}
	// 旧 metrics.db 的 server_metrics 缺少扩展列：逐条补列，仅忽略“列已存在”
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("metrics schema upgrade: begin: %w", err)
	}
	metricAlters := []string{
		`ALTER TABLE server_metrics ADD COLUMN disk_read_kbps REAL DEFAULT 0`,
		`ALTER TABLE server_metrics ADD COLUMN disk_write_kbps REAL DEFAULT 0`,
		`ALTER TABLE server_metrics ADD COLUMN net_rx_kbps REAL DEFAULT 0`,
		`ALTER TABLE server_metrics ADD COLUMN net_tx_kbps REAL DEFAULT 0`,
		`ALTER TABLE server_metrics ADD COLUMN cpu_user REAL DEFAULT 0`,
		`ALTER TABLE server_metrics ADD COLUMN cpu_system REAL DEFAULT 0`,
		`ALTER TABLE server_metrics ADD COLUMN cpu_iowait REAL DEFAULT 0`,
		`ALTER TABLE server_metrics ADD COLUMN memory_cached_gb REAL DEFAULT 0`,
		`ALTER TABLE server_metrics ADD COLUMN memory_buffers_gb REAL DEFAULT 0`,
		`ALTER TABLE server_metrics ADD COLUMN swap_used_gb REAL DEFAULT 0`,
		`ALTER TABLE server_metrics ADD COLUMN raw_metrics TEXT DEFAULT '{}'`,
	}
	for _, stmt := range metricAlters {
		if err := addColumnIfMissing(tx, stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("metrics schema upgrade: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("metrics schema upgrade: commit: %w", err)
	}
	logger.Infof("Metrics database schema created/verified")
	return nil
}

// MigrationError 迁移失败错误
type MigrationError struct {
	Version int
	Msg     string
}

func (e *MigrationError) Error() string {
	return "migration failed at v" + itoa(e.Version) + ": " + e.Msg
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
