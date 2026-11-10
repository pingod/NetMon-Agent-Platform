// Package services - SQLite 到 MySQL 数据迁移与生产配置生成服务
package services

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"itops-backend/internal/appdb"
	"itops-backend/internal/config"
	"itops-backend/internal/logger"
	"itops-backend/internal/migrate"
)

// MigrationReport 迁移结果报告
type MigrationReport struct {
	TotalTables int                `json:"total_tables"`
	TotalRows   int                `json:"total_rows"`
	DurationMs  int64              `json:"duration_ms"`
	Details     []TableMigrateStat `json:"details"`
	MigratedAt  string             `json:"migrated_at"`
}

// TableMigrateStat 单表迁移统计
type TableMigrateStat struct {
	TableName  string `json:"table_name"`
	Rows       int    `json:"rows"`
	Status     string `json:"status"` // "success" | "skipped" | "error"
	FailedRows int    `json:"failed_rows,omitempty"`
	Error      string `json:"error,omitempty"`
}

// DatabaseModeStatus 数据库运行状态
type DatabaseModeStatus struct {
	CurrentEngine    string `json:"current_engine"` // "mysql" | "sqlite"
	MySQLEnabled     bool   `json:"mysql_enabled"`
	MySQLConnected   bool   `json:"mysql_connected"`
	MySQLDSN         string `json:"mysql_dsn"`
	SQLitePath       string `json:"sqlite_path"`
	SQLiteTableCount int    `json:"sqlite_table_count"`
	SQLiteTotalRows  int    `json:"sqlite_total_rows"`
	MySQLTableCount  int    `json:"mysql_table_count"`
	MySQLTotalRows   int    `json:"mysql_total_rows"`
}

// 待迁移的核心业务与配置表列表（顺序按外键依赖优化）
var coreTablesToMigrate = []string{
	"users",
	"network_devices",
	"snmp_credentials",
	"credentials",
	"servers",
	"server_groups",
	"server_commands",
	"ssh_keys",
	"agents",
	"ai_models",
	"databases",
	"database_connections",
	"influxdb_write_settings",
	"dc_rooms",
	"dc_racks",
	"dc_devices",
	"dc_power_panels",
	"dc_power_feeds",
	"dc_cables",
	"device_manufacturers",
	"device_types",
	"network_subnets",
	"network_discovery_tasks",
	"topology_nodes",
	"topology_links",
	"config_backup_tasks",
	"config_backup_history",
	"alerts",
	"alert_rules",
	"alert_groups",
	"alert_providers",
	"alert_mappings",
	"alert_noise_rules",
	"workflows",
	"tasks",
	"scheduled_tasks",
	"approval_requests",
	"ai_remediations",
	"containers",
	"k8s_contexts",
	"virtual_machines",
	"knowledge_base",
	"tool_links",
	"audit_logs",
	"custom_dashboard_metrics",
	"custom_dashboard_lists",
	"dashboard_panels",
	"snmp_metric_definitions",
}

// GetDatabaseModeStatus 获取当前数据库运行模式与统计
func GetDatabaseModeStatus() (*DatabaseModeStatus, error) {
	status := &DatabaseModeStatus{
		CurrentEngine: "sqlite",
		MySQLEnabled:  appdb.IsMySQLMode(),
		SQLitePath:    config.Env.DatabasePath,
	}

	if appdb.IsMySQLMode() {
		status.CurrentEngine = "mysql"
		status.MySQLConnected = true
	}

	if config.Env.MySQLDSN != "" {
		status.MySQLDSN = maskDSN(config.Env.MySQLDSN)
	}

	// 统计 SQLite 表和数据量
	sqliteDB := appdb.SQLiteDB()
	if sqliteDB != nil {
		for _, tbl := range coreTablesToMigrate {
			var cnt int
			err := sqliteDB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tbl)).Scan(&cnt)
			if err == nil {
				status.SQLiteTableCount++
				status.SQLiteTotalRows += cnt
			}
		}
	}

	// 若已连接 MySQL，统计 MySQL 数据量
	mysqlDB := appdb.MySQLDB()
	if mysqlDB != nil {
		ctxErr := mysqlDB.Ping()
		if ctxErr == nil {
			status.MySQLConnected = true
			for _, tbl := range coreTablesToMigrate {
				var cnt int
				err := mysqlDB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tbl)).Scan(&cnt)
				if err == nil {
					status.MySQLTableCount++
					status.MySQLTotalRows += cnt
				}
			}
		}
	}

	return status, nil
}

// GenerateDotEnvConfig 根据 DSN 与当前配置生成生产环境 .env 文件内容
func GenerateDotEnvConfig(mysqlDSN string) string {
	jwtSecret := config.Env.JWTSecret
	if jwtSecret == "" {
		jwtSecret = "generate_your_secure_random_jwt_secret_32_chars_long"
	}

	port := config.Env.Port
	if port == 0 {
		port = 3001
	}

	influxEnabled := "false"
	if config.Env.InfluxdbEnabled {
		influxEnabled = "true"
	}

	var sb strings.Builder
	sb.WriteString("# ==============================================================================\n")
	sb.WriteString("# NetMon ITOps 生产环境配置文件 (.env)\n")
	sb.WriteString(fmt.Sprintf("# 生成时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("# ==============================================================================\n\n")

	sb.WriteString("# ── 基础运行配置 ──\n")
	sb.WriteString("NODE_ENV=production\n")
	sb.WriteString(fmt.Sprintf("PORT=%d\n", port))
	sb.WriteString("LOG_LEVEL=info\n")
	sb.WriteString(fmt.Sprintf("JWT_SECRET=%s\n", jwtSecret))
	sb.WriteString("JWT_EXPIRES_IN=24h\n\n")

	sb.WriteString("# ── 生产环境 MySQL 业务元数据与配置库 ──\n")
	sb.WriteString("MYSQL_ENABLED=true\n")
	sb.WriteString(fmt.Sprintf("MYSQL_DSN=%s\n\n", mysqlDSN))

	sb.WriteString("# ── 生产环境 InfluxDB 时序监控指标库 ──\n")
	sb.WriteString(fmt.Sprintf("INFLUXDB_ENABLED=%s\n", influxEnabled))
	sb.WriteString(fmt.Sprintf("INFLUXDB_VERSION=%s\n", config.Env.InfluxdbVersion))
	sb.WriteString(fmt.Sprintf("INFLUXDB_HOST=%s\n", config.Env.InfluxdbHost))
	sb.WriteString(fmt.Sprintf("INFLUXDB_PORT=%d\n", config.Env.InfluxdbPort))
	if config.Env.InfluxdbToken != "" {
		sb.WriteString(fmt.Sprintf("INFLUXDB_TOKEN=%s\n", config.Env.InfluxdbToken))
	}
	sb.WriteString(fmt.Sprintf("INFLUXDB_ORG=%s\n", config.Env.InfluxdbOrg))
	sb.WriteString(fmt.Sprintf("INFLUXDB_BUCKET=%s\n", config.Env.InfluxdbBucket))
	sb.WriteString(fmt.Sprintf("INFLUXDB_MEASUREMENT_PREFIX=%s\n", config.Env.InfluxdbMeasurementPrefix))
	sb.WriteString(fmt.Sprintf("INFLUXDB_WRITE_MODE=%s\n", config.Env.InfluxdbWriteMode))
	sb.WriteString(fmt.Sprintf("INFLUXDB_BATCH_SIZE=%d\n", config.Env.InfluxdbBatchSize))
	sb.WriteString(fmt.Sprintf("INFLUXDB_FLUSH_INTERVAL_MS=%d\n", config.Env.InfluxdbFlushIntervalMs))

	return sb.String()
}

// MigrateSQLiteToMySQL 一键将当前 SQLite 全部数据迁移/同步至目标 MySQL
func MigrateSQLiteToMySQL(targetDSN string) (*MigrationReport, error) {
	start := time.Now()

	// 1. 连接目标 MySQL
	targetDB, err := sql.Open("mysql", targetDSN)
	if err != nil {
		return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
	}
	defer targetDB.Close()

	if err := targetDB.Ping(); err != nil {
		return nil, fmt.Errorf("Ping MySQL 失败: %w", err)
	}

	// 2. 在 MySQL 中创建/验证全部 60+ 张表结构
	if err := migrate.RunMigrationsMySQL(targetDB); err != nil {
		logger.Warnf("MySQL 初始化表结构提示: %v", err)
	}

	// 3. 获取 SQLite 连接
	sqliteDB := appdb.SQLiteDB()
	if sqliteDB == nil {
		return nil, fmt.Errorf("SQLite 主数据库未就绪")
	}

	report := &MigrationReport{
		Details:    make([]TableMigrateStat, 0),
		MigratedAt: time.Now().Format("2006-01-02 15:04:05"),
	}

	// 4. 逐表迁移数据
	hadFailure := false
	for _, table := range coreTablesToMigrate {
		stat := TableMigrateStat{TableName: table, Status: "success"}

		// 指标类表位于 metrics.db，其余位于 app.db（不依赖 ATTACH）
		tableDB := sqliteDB
		if appdb.IsMetricsTable(table) && appdb.IsMetricsInitialized() {
			tableDB = appdb.MetricsDB()
		}

		// 检查源库中是否存在该表
		var tblExists int
		if err := tableDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&tblExists); err != nil {
			stat.Status = "error"
			stat.Error = err.Error()
			hadFailure = true
			report.Details = append(report.Details, stat)
			continue
		}
		if tblExists == 0 {
			stat.Status = "skipped"
			report.Details = append(report.Details, stat)
			continue
		}

		// 获取 SQLite 表列信息
		colRows, err := tableDB.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
		if err != nil {
			stat.Status = "error"
			stat.Error = err.Error()
			hadFailure = true
			report.Details = append(report.Details, stat)
			continue
		}

		var columns []string
		for colRows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dfltValue sql.NullString
			if err := colRows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
				columns = append(columns, name)
			}
		}
		colRows.Close()

		if len(columns) == 0 {
			stat.Status = "skipped"
			report.Details = append(report.Details, stat)
			continue
		}

		// 查询 SQLite 全量行数据
		colList := "`" + strings.Join(columns, "`, `") + "`"
		querySQL := fmt.Sprintf("SELECT %s FROM `%s`", colList, table)
		rows, err := tableDB.Query(querySQL)
		if err != nil {
			stat.Status = "error"
			stat.Error = err.Error()
			hadFailure = true
			report.Details = append(report.Details, stat)
			continue
		}

		// 构造 MySQL 批量替换写入 (REPLACE INTO 或 INSERT ... ON DUPLICATE KEY UPDATE)
		placeholders := make([]string, len(columns))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		insertSQL := fmt.Sprintf("REPLACE INTO `%s` (%s) VALUES (%s)", table, colList, strings.Join(placeholders, ", "))

		tx, err := targetDB.Begin()
		if err != nil {
			rows.Close()
			stat.Status = "error"
			stat.Error = err.Error()
			report.Details = append(report.Details, stat)
			continue
		}

		stmt, err := tx.Prepare(insertSQL)
		if err != nil {
			rows.Close()
			_ = tx.Rollback()
			stat.Status = "error"
			stat.Error = err.Error()
			report.Details = append(report.Details, stat)
			continue
		}

		rowCount, scanFail, execFail := 0, 0, 0
		for rows.Next() {
			// 扫描每一行
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				scanFail++
				continue
			}

			// 处理类型兼容（[]byte 转为 string）
			execArgs := make([]interface{}, len(columns))
			for i, v := range values {
				if b, ok := v.([]byte); ok {
					execArgs[i] = string(b)
				} else {
					execArgs[i] = v
				}
			}

			if _, err := stmt.Exec(execArgs...); err == nil {
				rowCount++
			} else {
				execFail++
			}
		}
		rows.Close()
		stmt.Close()
		if err := rows.Err(); err != nil {
			scanFail++
		}

		if err := tx.Commit(); err != nil {
			stat.Status = "error"
			stat.Error = "commit failed: " + err.Error()
			hadFailure = true
			report.Details = append(report.Details, stat)
			report.DurationMs = time.Since(start).Milliseconds()
			return report, fmt.Errorf("表 %s 提交失败: %w", table, err)
		}
		stat.Rows = rowCount
		if scanFail > 0 || execFail > 0 {
			stat.FailedRows = scanFail + execFail
			stat.Status = "error"
			stat.Error = fmt.Sprintf("扫描失败 %d 行，写入失败 %d 行", scanFail, execFail)
			hadFailure = true
		}
		report.TotalRows += rowCount
		report.TotalTables++

		report.Details = append(report.Details, stat)
	}

	report.DurationMs = time.Since(start).Milliseconds()
	logger.Infof("SQLite 到 MySQL 数据迁移完成: %d 张表, 共 %d 条记录 (耗时 %dms)",
		report.TotalTables, report.TotalRows, report.DurationMs)

	if hadFailure {
		return report, fmt.Errorf("部分表迁移失败，详见报告 details（失败行数计入 failed_rows）")
	}
	return report, nil
}

// SwitchToMySQLMode 动态开启并切换为 MySQL 生产模式，并将配置持久化写入 .env
func SwitchToMySQLMode(dsn string, persistEnv bool) error {
	// 1. 初始化并连接 MySQL
	if err := appdb.OpenMySQL(dsn); err != nil {
		return fmt.Errorf("无法连接到目标 MySQL: %w", err)
	}

	// 2. 运行表结构初始化
	if err := migrate.RunMigrationsMySQL(appdb.MySQLDB()); err != nil {
		logger.Warnf("MySQL 表结构初始化警告: %v", err)
	}

	// 3. 更新全局环境配置对象
	if config.Env != nil {
		config.Env.MySQLEnabled = true
		config.Env.MySQLDSN = dsn
	}

	// 4. 持久化写入 .env 文件
	if persistEnv {
		if err := updateEnvFile(map[string]string{
			"MYSQL_ENABLED": "true",
			"MYSQL_DSN":     dsn,
		}); err != nil {
			logger.Warnf("持久化写入 .env 失败: %v", err)
		} else {
			logger.Infof(".env 文件已更新: MYSQL_ENABLED=true")
		}
	}

	logger.Infof("系统主数据库已无缝切换为 MySQL 生产模式")
	return nil
}

// SwitchToSQLiteMode 切换回 SQLite 单机模式
func SwitchToSQLiteMode(persistEnv bool) error {
	_ = appdb.CloseMySQL()

	if config.Env != nil {
		config.Env.MySQLEnabled = false
	}

	if persistEnv {
		if err := updateEnvFile(map[string]string{
			"MYSQL_ENABLED": "false",
		}); err != nil {
			logger.Warnf("持久化写入 .env 失败: %v", err)
		}
	}

	logger.Infof("系统主数据库已切换回 SQLite 模式")
	return nil
}

// updateEnvFile 更新本地 .env 文件中的键值对
func updateEnvFile(updates map[string]string) error {
	envPath := ".env"
	content, err := os.ReadFile(envPath)
	if err != nil {
		// 若文件不存在，直接创建
		var sb strings.Builder
		for k, v := range updates {
			sb.WriteString(fmt.Sprintf("%s=%s\n", k, v))
		}
		return os.WriteFile(envPath, []byte(sb.String()), 0644)
	}

	lines := strings.Split(string(content), "\n")
	updatedKeys := make(map[string]bool)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			if newVal, ok := updates[key]; ok {
				lines[i] = fmt.Sprintf("%s=%s", key, newVal)
				updatedKeys[key] = true
			}
		}
	}

	// 写入尚未存在的键
	for k, v := range updates {
		if !updatedKeys[k] {
			lines = append(lines, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0644)
}

// maskDSN 隐藏 DSN 中的密码部分
func maskDSN(dsn string) string {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "******"
	}
	if cfg.Passwd != "" {
		cfg.Passwd = "******"
	}
	return cfg.FormatDSN()
}
