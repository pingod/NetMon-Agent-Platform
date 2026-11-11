// 网管系统 Go 后端入口
package main

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"itops-backend/internal/appdb"
	"itops-backend/internal/config"
	appcrypto "itops-backend/internal/crypto"
	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
	"itops-backend/internal/migrate"
	"itops-backend/internal/modules"
	"itops-backend/internal/modules/inspection"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/modules/planning"
	"itops-backend/internal/repos"
	"itops-backend/internal/services/tokenblacklist"
)

const initialAdminPasswordCharset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

// generateInitialPassword 生成随机初始管理员口令
func generateInitialPassword(length int) (string, error) {
	raw := make([]byte, length)
	if _, err := crand.Read(raw); err != nil {
		return "", err
	}
	out := make([]byte, length)
	for i, b := range raw {
		out[i] = initialAdminPasswordCharset[int(b)%len(initialAdminPasswordCharset)]
	}
	return string(out), nil
}

// seedDefaultAdmin 在 users 表为空时创建随机口令的 admin，并将口令写入受限文件（仅日志输出路径）
func seedDefaultAdmin(databasePath string) {
	if repos.Users.CountAll() != 0 {
		return
	}
	logger.Infof("No users found, seeding default admin account...")

	password, err := generateInitialPassword(24)
	if err != nil {
		logger.Errorf("Failed to generate initial admin password: %v", err)
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		logger.Errorf("Failed to hash initial admin password: %v", err)
		return
	}

	if err := repos.Users.CreateDefaultAdmin(
		"1", "admin", string(hashed), "admin@netmon.local", "admin", 1, 1,
	); err != nil {
		logger.Errorf("Failed to seed admin user: %v", err)
		return
	}

	passwordFile := filepath.Join(filepath.Dir(databasePath), "initial-admin-password.txt")
	content := fmt.Sprintf("# NetMon 初始管理员账户，请登录后立即修改密码并删除本文件\nusername: admin\npassword: %s\n", password)
	if err := os.WriteFile(passwordFile, []byte(content), 0o600); err != nil {
		logger.Errorf("Default admin created but failed to write initial password file: %v", err)
		return
	}
	logger.Infof("Default admin user created (username: admin); initial password written to %s", passwordFile)
}

func main() {
	// ── 加载 .env（从当前目录向上遍历查找项目根） ──
	if _, err := os.Stat(".env"); err == nil {
		config.LoadDotEnv()
	} else {
		// 当前目录无 .env，向上遍历最多5级父目录查找
		if wd, e := os.Getwd(); e == nil {
			dir := wd
			for i := 0; i < 5; i++ {
				parent := filepath.Dir(dir)
				if parent == dir {
					break // 已到文件系统根
				}
				parentEnv := filepath.Join(parent, ".env")
				if _, e2 := os.Stat(parentEnv); e2 == nil {
					config.LoadDotEnv(parentEnv)
					break
				}
				dir = parent
			}
		}
	}

	// ── 加载配置 ──
	cfg, err := config.Load()
	if err != nil {
		logger.Errorf("Config error: %v", err)
		os.Exit(1)
	}
	config.Env = cfg

	// ── 初始化日志（配置就绪后才能确定级别与日志目录） ──
	logger.Init(cfg)
	logger.Infof("Starting itops-backend (Go)...")

	// ── 初始化加密密钥（ENCRYPTION_KEY 优先，回退 JWT_SECRET） ──
	if err := appcrypto.Init(); err != nil {
		logger.Errorf("Encryption init failed: %v", err)
		os.Exit(1)
	}

	// ── 数据库初始化 ──
	// app.db — 设备信息、密码、机房、agent、数据库连接等元数据
	if err := appdb.Open(cfg.DatabasePath); err != nil {
		logger.Errorf("Failed to initialize database: %v", err)
		os.Exit(1)
	}
	db := appdb.DB()
	logger.Infof("Database initialized (app): %s", cfg.DatabasePath)

	// metrics.db — 设备指标采集数据
	metricsPath := cfg.MetricsDatabasePath
	if err := appdb.OpenMetrics(metricsPath); err != nil {
		logger.Errorf("Failed to initialize metrics database: %v", err)
		os.Exit(1)
	}
	logger.Infof("Database initialized (metrics): %s", metricsPath)

	// oid.db — 独立 OID / MIB 数据库
	oidPath := filepath.Join(filepath.Dir(cfg.DatabasePath), "oid.db")
	if err := appdb.OpenOid(oidPath); err != nil {
		logger.Errorf("Failed to initialize OID database: %v", err)
		os.Exit(1)
	}
	logger.Infof("Database initialized (oid): %s", oidPath)

	// ── 运行迁移 ──
	if err := migrate.RunMigrations(db); err != nil {
		logger.Errorf("Migration failed: %v", err)
		os.Exit(1)
	}
	logger.Infof("Database migrations applied (app)")
	appdb.InitServersTableSchema(db)

	// ── 种子：预置巡检模版（网络设备 + 服务器分层巡检） ──
	if err := inspection.EnsurePresets(); err != nil {
		logger.Warnf("Inspection template seed failed (non-fatal): %v", err)
	} else {
		logger.Infof("Inspection templates seeded")
	}

	// ── 种子：预置网络设备配置备份模版（华为/华三/思科/中兴/锐捷等） ──
	if err := services.EnsureBackupPresets(); err != nil {
		logger.Warnf("Config backup template seed failed (non-fatal): %v", err)
	} else {
		logger.Infof("Config backup templates seeded")
	}

	// ── 种子：确保 admin 用户存在（随机初始口令写入受限文件，日志不打印口令） ──
	seedDefaultAdmin(cfg.DatabasePath)

	// 指标数据库迁移
	if err := migrate.RunMetricsMigrations(appdb.MetricsDB()); err != nil {
		logger.Errorf("Metrics migration failed: %v", err)
		os.Exit(1)
	}
	logger.Infof("Database migrations applied (metrics)")

	// 修正 mib_objects 中 COLUMNAR 语法的表列标识（mib_objects 位于 oid.db）
	if _, err := appdb.OidDB().Exec(`UPDATE mib_objects SET is_table = 1, is_scalar = 0 WHERE (LOWER(syntax) LIKE '%column%' OR LOWER(syntax) LIKE '%table%' OR LOWER(name) LIKE '%table%') AND is_table = 0`); err != nil {
		logger.Warnf("Failed to sync COLUMNAR is_table flags: %v", err)
	}

	// 种子 SNMP 指标命名体系（内置 IF-MIB + 从 mib_objects 派生），幂等
	services.SnmpMetricSeedLog = logger.Infof
	if err := services.SeedSnmpMetricDefinitions(); err != nil {
		logger.Warnf("SNMP metric definition seed failed (non-fatal): %v", err)
	} else {
		logger.Infof("SNMP metric definitions seeded")
	}

	// ── 可选 MySQL 后端初始化 ──
	if cfg.MySQLEnabled {
		dsn := cfg.MySQLDSN
		if dsn != "" {
			if err := appdb.OpenMySQL(dsn); err != nil {
				logger.Warnf("MySQL connection failed (falling back to SQLite): %v", err)
			} else {
				logger.Infof("MySQL backend enabled and connected")
				// 在 MySQL 中创建相同的元数据表结构
				if err := migrate.RunMigrationsMySQL(appdb.MySQLDB()); err != nil {
					logger.Warnf("MySQL migration failed: %v", err)
				}
			}
		}
	}

	// ── 初始化服务 ──
	tokenblacklist.Init()
	middleware.StartRateLimiterCleanup()
	middleware.StartUserCacheCleanup()

	services.ConfigBackupServiceInstance.Init()
	services.DeviceStatusProberInstance.Start()
	services.SnmpPollerInstance.Start()
	services.StartAlertRuleEvaluator()
	logger.Infof("Network services initialized")

	// ── 规划引擎（IDC 上架布线）──
	// 异步启动：引擎不可用不应阻塞主服务；代理返回 503 与原因。
	planning.Start()

	// ── 构建路由 ──
	router := modules.NewRouter()

	// ── 启动 HTTP 服务 ──
	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}

	// 绑定地址：通过 BIND 环境变量控制 IPv4/IPv6
	//   BIND=::        → 仅 IPv6（纯 IPv6 网络）
	//   BIND=127.0.0.1 → 仅本机 IPv4
	//   不设置/其他    → 0.0.0.0（默认 IPv4，兼容性最好）
	bind := os.Getenv("BIND")
	if bind == "" {
		bind = "0.0.0.0"
	}
	addr := net.JoinHostPort(bind, port)

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 优雅关停：先停止接收新请求并 drain 在途请求，再停业务服务，最后关闭数据库
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logger.Infof("Received signal %v, shutting down...", sig)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Warnf("HTTP server shutdown error: %v", err)
		}

		tokenblacklist.Shutdown()
		services.ConfigBackupServiceInstance.Shutdown()
		services.DeviceStatusProberInstance.Shutdown()
		services.SnmpPollerInstance.Shutdown()
		services.StopAlertRuleEvaluator()
		planning.Stop()

		if err := appdb.Close(); err != nil {
			logger.Warnf("Database close error: %v", err)
		}
	}()

	logger.Infof("Server listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Errorf("Server error: %v", err)
		os.Exit(1)
	}

	logger.Infof("Server stopped")
}
