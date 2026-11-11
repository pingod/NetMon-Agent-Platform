// Package appdb 数据库连接与单例
//
// 存储架构：
//   - app.db    (SQLite)  — 设备信息、密码、机房、agent、数据库连接等元数据
//   - metrics.db (SQLite) — 设备指标采集数据（SNMP 采集结果、接口指标等）
//   - MySQL     (可选)    — 当用户配置 MySQL 连接后，app.db 的元数据通过抽象层写入 MySQL
//   - InfluxDB  (可选)    — 当用户配置 InfluxDB 写入后，指标数据写入 InfluxDB 替代 metrics.db
package appdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"

	"itops-backend/internal/logger"
)

var (
	mu          sync.RWMutex
	dbConn      *sql.DB // app.db（SQLite）或 MySQL 连接
	metricsConn *sql.DB // metrics.db（SQLite）
	oidConn     *sql.DB // oid.db（SQLite）
	inited      bool
	metricsInit bool
	oidInit     bool
)

// MySQL 连接状态
var (
	mysqlMu     sync.RWMutex
	mysqlConn   *sql.DB // MySQL 连接（可选）
	mysqlInited bool
)

// pragmas 针对高并发读写优化的 SQLite WAL 配置
var pragmas = []string{
	`PRAGMA journal_mode = WAL`,
	`PRAGMA wal_autocheckpoint = 4000`,
	`PRAGMA journal_size_limit = 268435456`,
	`PRAGMA busy_timeout = 10000`,
	`PRAGMA locking_mode = NORMAL`,
	`PRAGMA foreign_keys = ON`,
	`PRAGMA synchronous = NORMAL`,
	`PRAGMA recursive_triggers = ON`,
	`PRAGMA temp_store = MEMORY`,
	`PRAGMA mmap_size = 2147483648`,
	`PRAGMA cache_size = -128000`,
	`PRAGMA cache_spill = ON`,
	`PRAGMA automatic_index = ON`,
}

// sqlitePragmas 写入 DSN，确保连接池中每条新连接都带上关键 PRAGMA
// （原先仅对单条连接执行 Exec PRAGMA，其余连接 busy_timeout=0 会立即失败）
const sqlitePragmas = "_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=temp_store(memory)"

// sqliteDSN 为 SQLite 路径拼接连接参数
func sqliteDSN(path string) string {
	if strings.Contains(path, "?") {
		return path + "&" + sqlitePragmas
	}
	return path + "?" + sqlitePragmas
}

// Open 初始化主数据库连接（app.db SQLite 或 MySQL）（幂等）
func Open(dbPath string) error {
	mu.Lock()
	defer mu.Unlock()
	if inited && dbConn != nil {
		logger.Infof("Database already initialized, skipping")
		return nil
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)

	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			logger.Warnf("pragma %q failed: %v", p, err)
		}
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping sqlite: %w", err)
	}

	dbConn = db
	inited = true
	initAlertRulesTable(db)
	InitServersTableSchema(db)
	logger.Infof("SQLite connected (app): %s", dbPath)
	return nil
}

// InitServersTableSchema 确保 servers 及 server_metrics 表包含完整 agent 及指标字段
func InitServersTableSchema(db *sql.DB) {
	if TableExists(db, "servers") {
		cols := []struct {
			name string
			dflt string
		}{
			{"vendor", "TEXT DEFAULT 'other'"},
			{"model", "TEXT DEFAULT ''"},
			{"is_virtual_machine", "INTEGER DEFAULT 0"},
			{"agent_installed", "INTEGER DEFAULT 0"},
			{"agent_version", "TEXT DEFAULT ''"},
			{"agent_status", "TEXT DEFAULT 'uninstalled'"},
			{"agent_last_heartbeat", "DATETIME"},
			{"agent_cpu_usage", "REAL DEFAULT 0"},
			{"agent_mem_mb", "REAL DEFAULT 0"},
			{"agent_uptime_seconds", "INTEGER DEFAULT 0"},
			{"agent_token", "TEXT DEFAULT ''"},
		}
		for _, col := range cols {
			if !ColumnExists(db, "servers", col.name) {
				_, err := db.Exec(fmt.Sprintf(`ALTER TABLE servers ADD COLUMN %s %s`, col.name, col.dflt))
				if err != nil {
					logger.Warnf("Failed to add column %s to servers: %v", col.name, err)
				} else {
					logger.Infof("Added %s column to servers table", col.name)
				}
			}
		}
	}

	// 确保 server_metrics 包含磁盘 IO、带宽、各核心 CPU 与负载扩展字段
	if TableExists(db, "server_metrics") {
		metricCols := []struct {
			name string
			dflt string
		}{
			{"disk_read_kbps", "REAL DEFAULT 0"},
			{"disk_write_kbps", "REAL DEFAULT 0"},
			{"net_rx_kbps", "REAL DEFAULT 0"},
			{"net_tx_kbps", "REAL DEFAULT 0"},
			{"cpu_user", "REAL DEFAULT 0"},
			{"cpu_system", "REAL DEFAULT 0"},
			{"cpu_iowait", "REAL DEFAULT 0"},
			{"memory_cached_gb", "REAL DEFAULT 0"},
			{"memory_buffers_gb", "REAL DEFAULT 0"},
			{"swap_used_gb", "REAL DEFAULT 0"},
			{"raw_metrics", "TEXT DEFAULT '{}'"},
		}
		for _, col := range metricCols {
			if !ColumnExists(db, "server_metrics", col.name) {
				_, err := db.Exec(fmt.Sprintf(`ALTER TABLE server_metrics ADD COLUMN %s %s`, col.name, col.dflt))
				if err != nil {
					logger.Warnf("Failed to add column %s to server_metrics: %v", col.name, err)
				} else {
					logger.Infof("Added %s column to server_metrics table", col.name)
				}
			}
		}
	}
}

// OpenMetrics 初始化指标数据库连接（metrics.db SQLite）（幂等）
// 同时将 metrics.db ATTACH 到主数据库连接上，使得 appdb.DB() 可以透明访问指标表
func OpenMetrics(dbPath string) error {
	mu.Lock()
	defer mu.Unlock()
	if metricsInit && metricsConn != nil {
		logger.Infof("Metrics database already initialized, skipping")
		return nil
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create metrics db dir: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return fmt.Errorf("open metrics sqlite: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)

	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			logger.Warnf("metrics pragma %q failed: %v", p, err)
		}
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping metrics sqlite: %w", err)
	}

	metricsConn = db
	metricsInit = true
	logger.Infof("SQLite connected (metrics): %s", dbPath)

	// 将 metrics.db ATTACH 到主数据库连接上
	// 这样所有通过 appdb.DB() 的 SQL 查询可以直接访问 metrics 表，
	// 无需修改任何 repo 层代码
	if dbConn != nil {
		// 先 DETACH 以防重复挂载
		_, _ = dbConn.Exec(`DETACH DATABASE IF EXISTS metrics_db`)
		_, err := dbConn.Exec(`ATTACH DATABASE ? AS metrics_db`, dbPath)
		if err != nil {
			logger.Warnf("Failed to ATTACH metrics.db to main connection: %v", err)
		} else {
			logger.Infof("Metrics database ATTACHED to main connection as 'metrics_db'")
		}
	}

	return nil
}

// OpenMySQL 初始化 MySQL 连接（可选后端）
// dsn 格式: user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
func OpenMySQL(dsn string) error {
	mysqlMu.Lock()
	defer mysqlMu.Unlock()

	if mysqlInited && mysqlConn != nil {
		// 先关闭旧连接
		_ = mysqlConn.Close()
		mysqlConn = nil
		mysqlInited = false
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(20)
	db.SetConnMaxLifetime(10 * time.Minute)

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping mysql: %w", err)
	}

	mysqlConn = db
	mysqlInited = true
	logger.Infof("MySQL connected")
	return nil
}

// CloseMySQL 关闭 MySQL 连接
func CloseMySQL() error {
	mysqlMu.Lock()
	defer mysqlMu.Unlock()
	if mysqlConn != nil {
		err := mysqlConn.Close()
		mysqlConn = nil
		mysqlInited = false
		return err
	}
	return nil
}

// MySQLDB 返回 MySQL 数据库单例（如果未初始化则返回 nil）
func MySQLDB() *sql.DB {
	mysqlMu.RLock()
	defer mysqlMu.RUnlock()
	return mysqlConn
}

// IsMySQLMode 是否当前处于 MySQL 模式
func IsMySQLMode() bool {
	mysqlMu.RLock()
	defer mysqlMu.RUnlock()
	return mysqlInited && mysqlConn != nil
}

// NowExpr 返回当前时间的 SQL 表达式（按当前后端方言：SQLite / MySQL）
func NowExpr() string {
	if IsMySQLMode() {
		return "NOW()"
	}
	return "datetime('now','localtime')"
}

// NowExprFor 返回适用于指定连接的当前时间 SQL 表达式。
// 指标库（metrics.db）等固定使用 SQLite 方言的连接即使在 MySQL 模式下也返回 SQLite 表达式。
func NowExprFor(db *sql.DB) string {
	if db != nil && IsMySQLMode() && db == MySQLDB() {
		return "NOW()"
	}
	return "datetime('now','localtime')"
}

// SQLiteDB 返回底层的 SQLite 数据库连接（可供数据迁移和底层维护使用）
func SQLiteDB() *sql.DB {
	mu.RLock()
	defer mu.RUnlock()
	return dbConn
}

// DB 返回主数据库单例（启用 MySQL 时返回 MySQL 连接，否则返回 app.db SQLite 连接）
func DB() *sql.DB {
	mysqlMu.RLock()
	if mysqlInited && mysqlConn != nil {
		db := mysqlConn
		mysqlMu.RUnlock()
		return db
	}
	mysqlMu.RUnlock()

	mu.RLock()
	defer mu.RUnlock()
	if dbConn == nil {
		panic("Database not initialized. Call appdb.Open() first.")
	}
	return dbConn
}

// MetricsDB 返回指标数据库单例（metrics.db SQLite）
func MetricsDB() *sql.DB {
	mu.RLock()
	defer mu.RUnlock()
	if metricsConn == nil {
		panic("Metrics database not initialized. Call appdb.OpenMetrics() first.")
	}
	return metricsConn
}

// OpenOid 初始化独立 OID 数据库连接（oid.db SQLite）（幂等）
// 同时创建 mib_modules 和 mib_objects 表结构与性能索引，并 ATTACH 到主数据库连接
func OpenOid(dbPath string) error {
	mu.Lock()
	defer mu.Unlock()
	if oidInit && oidConn != nil {
		logger.Infof("OID database already initialized, skipping")
		return nil
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create oid db dir: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return fmt.Errorf("open oid sqlite: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)

	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			logger.Warnf("oid pragma %q failed: %v", p, err)
		}
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping oid sqlite: %w", err)
	}

	initOidTables(db)

	oidConn = db
	oidInit = true
	logger.Infof("SQLite connected (oid): %s", dbPath)

	if dbConn != nil {
		_, _ = dbConn.Exec(`DETACH DATABASE IF EXISTS oid_db`)
		_, err := dbConn.Exec(`ATTACH DATABASE ? AS oid_db`, dbPath)
		if err != nil {
			logger.Warnf("Failed to ATTACH oid.db to main connection: %v", err)
		} else {
			logger.Infof("OID database ATTACHED to main connection as 'oid_db'")
		}
	}

	return nil
}

func initOidTables(db *sql.DB) {
	schema := `
	CREATE TABLE IF NOT EXISTS mib_modules (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		file_name TEXT,
		file_content TEXT,
		description TEXT,
		organization TEXT,
		contact_info TEXT,
		last_updated TEXT,
		oid_tree_json TEXT,
		object_count INTEGER NOT NULL DEFAULT 0,
		uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS mib_objects (
		id TEXT PRIMARY KEY,
		module_id TEXT NOT NULL,
		module_name TEXT NOT NULL,
		name TEXT NOT NULL,
		oid TEXT NOT NULL,
		syntax TEXT,
		access TEXT,
		status TEXT,
		description TEXT,
		parent_oid TEXT,
		table_name TEXT,
		index_columns TEXT,
		is_table INTEGER NOT NULL DEFAULT 0,
		is_scalar INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (module_id) REFERENCES mib_modules(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_mib_objects_module_id ON mib_objects(module_id);
	CREATE INDEX IF NOT EXISTS idx_mib_objects_oid ON mib_objects(oid);
	CREATE INDEX IF NOT EXISTS idx_mib_objects_name ON mib_objects(name);
	CREATE INDEX IF NOT EXISTS idx_mib_objects_is_table ON mib_objects(is_table);
	CREATE INDEX IF NOT EXISTS idx_mib_objects_search ON mib_objects(name, oid);
	`
	if _, err := db.Exec(schema); err != nil {
		logger.Warnf("Failed to create tables/indexes in oid.db: %v", err)
	}
}

func initAlertRulesTable(db *sql.DB) {
	schema := `
	CREATE TABLE IF NOT EXISTS alert_rules (
		id VARCHAR(64) PRIMARY KEY,
		name VARCHAR(128) NOT NULL,
		description TEXT,
		severity VARCHAR(32) NOT NULL DEFAULT 'warning',
		metric_name VARCHAR(128) NOT NULL,
		promql TEXT,
		tags_filter_json TEXT,
		operator VARCHAR(16) NOT NULL DEFAULT '>',
		threshold DOUBLE NOT NULL DEFAULT 0,
		for_duration_sec INT NOT NULL DEFAULT 0,
		group_by_tags VARCHAR(256),
		enabled INT NOT NULL DEFAULT 1,
		cooldown_ms INT NOT NULL DEFAULT 600000,
		last_triggered_at BIGINT NOT NULL DEFAULT 0,
		template TEXT,
		created_at VARCHAR(64),
		updated_at VARCHAR(64)
	);
	CREATE INDEX IF NOT EXISTS idx_alert_rules_metric_name ON alert_rules(metric_name);
	CREATE INDEX IF NOT EXISTS idx_alert_rules_enabled ON alert_rules(enabled);
	`
	if _, err := db.Exec(schema); err != nil {
		logger.Warnf("Failed to create alert_rules table: %v", err)
	}
}

// OidDB 返回 OID 数据库单例（oid.db SQLite）
func OidDB() *sql.DB {
	mu.RLock()
	conn := oidConn
	mu.RUnlock()
	if conn == nil {
		return DB()
	}
	return conn
}

// IsInitialized 主数据库是否已初始化
func IsInitialized() bool {
	mu.RLock()
	defer mu.RUnlock()
	return inited && dbConn != nil
}

// IsMetricsInitialized 指标数据库是否已初始化
func IsMetricsInitialized() bool {
	mu.RLock()
	defer mu.RUnlock()
	return metricsInit && metricsConn != nil
}

// Close 关闭所有连接
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	var errs []error
	if dbConn != nil {
		if err := dbConn.Close(); err != nil {
			errs = append(errs, err)
		}
		dbConn = nil
		inited = false
	}
	if metricsConn != nil {
		if err := metricsConn.Close(); err != nil {
			errs = append(errs, err)
		}
		metricsConn = nil
		metricsInit = false
	}
	if oidConn != nil {
		if err := oidConn.Close(); err != nil {
			errs = append(errs, err)
		}
		oidConn = nil
		oidInit = false
	}

	mysqlMu.Lock()
	if mysqlConn != nil {
		if err := mysqlConn.Close(); err != nil {
			errs = append(errs, err)
		}
		mysqlConn = nil
		mysqlInited = false
	}
	mysqlMu.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// Helper: 判断列是否存在（迁移中常用）
func ColumnExists(db *sql.DB, table, column string) bool {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		if name == column {
			return true
		}
	}
	return false
}

// TableExists 判断表是否存在
func TableExists(db *sql.DB, table string) bool {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
	return err == nil && n > 0
}

// ParseDBTime 解析从数据库读出的时间字符串。
// modernc.org/sqlite 会把 DATETIME 声明列解析为 time.Time，扫描进 string 后是 RFC3339；
// TEXT 列则原样返回 "2006-01-02 15:04:05"。两种形态都必须接受，否则时间差计算静默归零。
func ParseDBTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
}
