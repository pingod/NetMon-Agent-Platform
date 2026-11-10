// Package migrate 数据库迁移框架，等价于 TS models/migrations/migrationFramework.ts
package migrate

import (
	"database/sql"
	"sort"
	"sync"

	"github.com/google/uuid"

	"itops-backend/internal/logger"
)

// Migration 单个迁移定义
type Migration struct {
	ID          string
	Version     int
	Name        string
	Description string
	Up          func(tx *sql.Tx) error
	Down        func(tx *sql.Tx) error
}

// Record schema_migrations 表记录
type Record struct {
	ID           string
	Version      int
	Name         string
	AppliedAt    string
	Success      bool
	ErrorMessage string
}

// Result 迁移结果
type Result struct {
	Success           bool
	ExecutedMigrations int
	CurrentVersion    int
	FailedVersion     int
	ErrorMessage      string
}

// Manager 迁移管理器
type Manager struct {
	mu         sync.Mutex
	db         *sql.DB
	migrations []Migration
}

// NewManager 创建管理器并初始化迁移表
func NewManager(db *sql.DB) *Manager {
	m := &Manager{db: db}
	m.initTable()
	return m
}

func (m *Manager) initTable() {
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id TEXT PRIMARY KEY,
			version INTEGER NOT NULL UNIQUE,
			name TEXT NOT NULL,
			applied_at DATETIME DEFAULT (datetime('now','localtime')),
			success INTEGER NOT NULL DEFAULT 0,
			error_message TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_schema_migrations_version ON schema_migrations(version);
		CREATE INDEX IF NOT EXISTS idx_schema_migrations_success ON schema_migrations(success);
	`)
	if err != nil {
		logger.Errorf("❌ Failed to initialize migration table: %v", err)
		panic(err)
	}
	logger.Infof("✅ schema_migrations table initialized")
}

// Register 注册单个迁移
func (m *Manager) Register(mig Migration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.migrations {
		if e.Version == mig.Version {
			panic("Migration version already exists: " + mig.Name)
		}
		if e.Name == mig.Name {
			panic("Migration name conflicts: " + mig.Name)
		}
	}
	m.migrations = append(m.migrations, mig)
	sort.Slice(m.migrations, func(i, j int) bool { return m.migrations[i].Version < m.migrations[j].Version })
}

// RegisterBatch 批量注册
func (m *Manager) RegisterBatch(migs []Migration) {
	for _, mig := range migs {
		m.Register(mig)
	}
}

func (m *Manager) appliedRecords() []Record {
	rows, err := m.db.Query(`SELECT id, version, name, applied_at, success, COALESCE(error_message,'') FROM schema_migrations ORDER BY version ASC`)
	if err != nil {
		logger.Errorf("❌ Failed to get applied migrations: %v", err)
		return nil
	}
	defer rows.Close()
	var recs []Record
	for rows.Next() {
		var r Record
		var success int
		if err := rows.Scan(&r.ID, &r.Version, &r.Name, &r.AppliedAt, &success, &r.ErrorMessage); err != nil {
			continue
		}
		r.Success = success == 1
		recs = append(recs, r)
	}
	return recs
}

// CurrentVersion 当前成功应用的最大版本
func (m *Manager) CurrentVersion() int {
	var max *int
	err := m.db.QueryRow(`SELECT MAX(version) FROM schema_migrations WHERE success = 1`).Scan(&max)
	if err != nil || max == nil {
		return 0
	}
	return *max
}

func (m *Manager) pending() []Migration {
	applied := map[int]bool{}
	for _, r := range m.appliedRecords() {
		if r.Success {
			applied[r.Version] = true
		}
	}
	var out []Migration
	for _, mig := range m.migrations {
		if !applied[mig.Version] {
			out = append(out, mig)
		}
	}
	return out
}

// MigrateTo 迁移到指定版本
func (m *Manager) MigrateTo(target int) Result {
	current := m.CurrentVersion()
	pendingList := m.pending()
	var filtered []Migration
	for _, mig := range pendingList {
		if mig.Version <= target {
			filtered = append(filtered, mig)
		}
	}

	if len(filtered) == 0 {
		logger.Infof("✅ No pending migrations to apply. Current version: v%d", current)
		return Result{Success: true, ExecutedMigrations: 0, CurrentVersion: current}
	}

	logger.Infof("🔄 Starting migration from v%d to v%d", current, target)
	logger.Infof("📋 %d pending migration(s) to apply", len(filtered))

	executed := 0
	for _, mig := range filtered {
		logger.Infof("⬆️ Applying migration v%d: %s", mig.Version, mig.Name)
		tx, err := m.db.Begin()
		if err != nil {
			return Result{Success: false, ExecutedMigrations: executed, CurrentVersion: m.CurrentVersion(), FailedVersion: mig.Version, ErrorMessage: err.Error()}
		}
		if err := mig.Up(tx); err != nil {
			_ = tx.Rollback()
			logger.Errorf("❌ Migration v%d failed, rolling back: %v", mig.Version, err)
			m.deleteRecord(mig.Version)
			m.insertRecord(mig.Version, mig.Name, false, err.Error())
			return Result{Success: false, ExecutedMigrations: executed, CurrentVersion: m.CurrentVersion(), FailedVersion: mig.Version, ErrorMessage: err.Error()}
		}
		m.deleteRecordTx(tx, mig.Version)
		m.insertRecordTx(tx, mig.Version, mig.Name, true, "")
		if err := tx.Commit(); err != nil {
			logger.Errorf("❌ Migration v%d commit failed: %v", mig.Version, err)
		}
		executed++
		logger.Infof("✅ Migration v%d applied successfully", mig.Version)
	}

	final := m.CurrentVersion()
	logger.Infof("✅ Migration complete. Now at version v%d", final)
	return Result{Success: true, ExecutedMigrations: executed, CurrentVersion: final}
}

// Migrate 迁移到最新版本
func (m *Manager) Migrate() Result {
	latest := 0
	for _, mig := range m.migrations {
		if mig.Version > latest {
			latest = mig.Version
		}
	}
	return m.MigrateTo(latest)
}

func (m *Manager) deleteRecord(version int) {
	_, _ = m.db.Exec(`DELETE FROM schema_migrations WHERE version = ?`, version)
}

func (m *Manager) insertRecord(version int, name string, success bool, errMsg string) {
	s := 0
	if success {
		s = 1
	}
	_, _ = m.db.Exec(`INSERT INTO schema_migrations (id, version, name, success, error_message) VALUES (?, ?, ?, ?, ?)`,
		uuid.NewString(), version, name, s, errMsg)
}

// deleteRecordTx 在事务内删除记录（避免与迁移事务争用唯一连接）
func (m *Manager) deleteRecordTx(tx *sql.Tx, version int) {
	_, _ = tx.Exec(`DELETE FROM schema_migrations WHERE version = ?`, version)
}

// insertRecordTx 在事务内写入记录
func (m *Manager) insertRecordTx(tx *sql.Tx, version int, name string, success bool, errMsg string) {
	s := 0
	if success {
		s = 1
	}
	_, _ = tx.Exec(`INSERT INTO schema_migrations (id, version, name, success, error_message) VALUES (?, ?, ?, ?, ?)`,
		uuid.NewString(), version, name, s, errMsg)
}
