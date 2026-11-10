package migrate

import (
	"os"
	"path/filepath"
	"testing"

	"itops-backend/internal/appdb"
	"itops-backend/internal/repos"
)

// TestFreshInstallSchema 验证合并 schema 可在全新数据库一次性建成全部表
func TestFreshInstallSchema(t *testing.T) {
	tmp := filepath.Join(os.TempDir(), "itops_migration_test.db")
	_ = os.Remove(tmp)
	defer os.Remove(tmp)

	if err := appdb.Open(tmp); err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer appdb.Close()

	db := appdb.DB()
	if err := RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	// 校验版本达到最新迁移版本
	m := NewAppMigrationManager(db)
	expectedVer := migrationVersions[len(migrationVersions)-1]
	if got := m.CurrentVersion(); got != expectedVer {
		t.Fatalf("expected current version %d, got %d", expectedVer, got)
	}

	// 校验表数量（114 业务表 + schema_migrations = 115）
	var tableCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&tableCount); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount < 115 {
		t.Fatalf("expected at least 115 tables, got %d", tableCount)
	}

	// 抽查若干关键表存在
	for _, tbl := range []string{"users", "servers", "alerts", "agents", "workflows", "network_devices", "settings", "token_blacklist", "config_backup_tasks", "snmp_collect_tasks"} {
		if !appdb.TableExists(db, tbl) {
			t.Errorf("expected table %s to exist", tbl)
		}
	}

	t.Logf("fresh install OK: %d tables, version %d", tableCount, m.CurrentVersion())
}

// TestExistingDbNoop 验证对已迁移库不重复执行（幂等）
func TestExistingDbNoop(t *testing.T) {
	tmp := filepath.Join(os.TempDir(), "itops_migration_noop.db")
	_ = os.Remove(tmp)
	defer os.Remove(tmp)

	if err := appdb.Open(tmp); err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer appdb.Close()
	db := appdb.DB()

	// 第一次运行：建库
	if err := RunMigrations(db); err != nil {
		t.Fatalf("first run: %v", err)
	}
	m := NewAppMigrationManager(db)
	res := m.Migrate()
	if !res.Success {
		t.Fatalf("second run failed: %s", res.ErrorMessage)
	}
	if res.ExecutedMigrations != 0 {
		t.Fatalf("expected 0 migrations on second run, got %d", res.ExecutedMigrations)
	}
}

func TestListTables(t *testing.T) {
	appPath := "../../../backend/data/app.db"
	if _, err := os.Stat(appPath); err != nil {
		appPath = "../../backend/data/app.db"
	}
	if err := appdb.Open(appPath); err != nil {
		t.Fatalf("open: %v", err)
	}
	defer appdb.Close()
	_ = repos.Users
	tables := []string{"workflows", "tasks", "scheduled_tasks", "reports", "containers", "container_images", "storage_volumes", "compose_projects", "k8s_contexts", "virtual_machines", "vm_platforms", "knowledge_base", "audit_logs", "tool_links", "approval_requests", "remediation_policies", "remediation_executions", "ai_remediations"}
	for _, tbl := range tables {
		var sqlStr string
		err := appdb.DB().QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&sqlStr)
		if err != nil {
			t.Logf("Table %s not found: %v", tbl, err)
		} else {
			t.Logf("Table %s: %s", tbl, sqlStr)
		}
	}
}
