// Package routes - 数据库运行模式切换、配置生成与数据迁移路由
// 路径前缀 /api/v1/database-mode
package routes

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-sql-driver/mysql"

	"itops-backend/internal/crypto"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/database/services"
	"itops-backend/internal/repos"
)

// maskMySQLDSN 用 mysql.ParseDSN 重新序列化并抹掉密码，避免明文回传
func maskMySQLDSN(dsn string) string {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return ""
	}
	if cfg.Passwd != "" {
		cfg.Passwd = "***"
	}
	return cfg.FormatDSN()
}

// DatabaseModeRoutes 数据库模式管理路由
func DatabaseModeRoutes() chi.Router {
	r := chi.NewRouter()

	// GET / — 获取当前数据库引擎状态与统计
	r.With(middleware.RequireRole("admin", "operator")).Get("/", func(w http.ResponseWriter, r *http.Request) {
		status, err := services.GetDatabaseModeStatus()
		if err != nil {
			httpx.RespondError(w, "获取数据库状态失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, status)
	})

	// POST /generate-config — 生成生产 .env 配置文件文本
	r.With(middleware.RequireRole("admin", "operator")).Post("/generate-config", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DBConnectionID string `json:"db_connection_id"`
			DSN            string `json:"dsn"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		dsn := req.DSN
		if dsn == "" && req.DBConnectionID != "" {
			// 从已保存连接解析出的 DSN 含明文口令，仅 admin 可获取
			if u := middleware.GetUser(r); u == nil || u.Role != "admin" {
				httpx.RespondFailStatus(w, http.StatusForbidden, "仅管理员可通过数据库连接 ID 生成含口令的配置")
				return
			}
			resolved, err := resolveDSNFromConnectionID(req.DBConnectionID)
			if err != nil {
				httpx.RespondError(w, err.Error(), http.StatusBadRequest)
				return
			}
			dsn = resolved
		}

		if dsn == "" {
			httpx.RespondError(w, "缺少 MySQL DSN 或数据库连接 ID", http.StatusBadRequest)
			return
		}

		envContent := services.GenerateDotEnvConfig(dsn)
		httpx.Respond(w, map[string]interface{}{
			"env_content": envContent,
			"dsn":         maskMySQLDSN(dsn),
		})
	})

	// POST /migrate-to-mysql — 一键将 SQLite 全部数据迁移到 MySQL
	r.With(middleware.RequireRole("admin")).Post("/migrate-to-mysql", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DBConnectionID string `json:"db_connection_id"`
			DSN            string `json:"dsn"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		dsn := req.DSN
		if dsn == "" && req.DBConnectionID != "" {
			resolved, err := resolveDSNFromConnectionID(req.DBConnectionID)
			if err != nil {
				httpx.RespondError(w, err.Error(), http.StatusBadRequest)
				return
			}
			dsn = resolved
		}

		if dsn == "" {
			httpx.RespondError(w, "缺少 MySQL DSN 或数据库连接 ID", http.StatusBadRequest)
			return
		}

		report, err := services.MigrateSQLiteToMySQL(dsn)
		if err != nil {
			httpx.RespondError(w, "数据迁移失败: "+err.Error(), http.StatusBadRequest)
			return
		}

		httpx.Respond(w, report)
	})

	// POST /switch-mode — 切换主数据库引擎 (mysql / sqlite) 并持久化
	r.With(middleware.RequireRole("admin")).Post("/switch-mode", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Mode           string `json:"mode"` // "mysql" | "sqlite"
			DBConnectionID string `json:"db_connection_id"`
			DSN            string `json:"dsn"`
			PersistEnv     bool   `json:"persist_env"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		if req.Mode == "mysql" {
			dsn := req.DSN
			if dsn == "" && req.DBConnectionID != "" {
				resolved, err := resolveDSNFromConnectionID(req.DBConnectionID)
				if err != nil {
					httpx.RespondError(w, err.Error(), http.StatusBadRequest)
					return
				}
				dsn = resolved
			}

			if dsn == "" {
				httpx.RespondError(w, "启用 MySQL 模式必须提供 DSN 或选定有效的数据库连接", http.StatusBadRequest)
				return
			}

			if err := services.SwitchToMySQLMode(dsn, true); err != nil {
				httpx.RespondError(w, "切换到 MySQL 模式失败: "+err.Error(), http.StatusBadRequest)
				return
			}

			httpx.Respond(w, map[string]interface{}{
				"message": "已成功切换为 MySQL 生产主存储模式，配置已持久化",
				"mode":    "mysql",
			})
			return
		}

		if req.Mode == "sqlite" {
			if err := services.SwitchToSQLiteMode(true); err != nil {
				httpx.RespondError(w, "切换到 SQLite 模式失败: "+err.Error(), http.StatusBadRequest)
				return
			}

			httpx.Respond(w, map[string]interface{}{
				"message": "已成功切换为 SQLite 单机模式",
				"mode":    "sqlite",
			})
			return
		}

		httpx.RespondError(w, "不支持的运行模式: "+req.Mode, http.StatusBadRequest)
	})

	return r
}

// resolveDSNFromConnectionID 从已保存的连接中提取并解密 DSN
func resolveDSNFromConnectionID(connID string) (string, error) {
	conn, err := repos.DbConnections.GetByID(connID)
	if err != nil || conn == nil {
		return "", fmt.Errorf("数据库连接不存在: %s", connID)
	}

	if conn.DBType != "mysql" && conn.DBType != "" {
		return "", fmt.Errorf("所选连接不是 MySQL 类型 (当前为 %s)", conn.DBType)
	}

	decryptedPassword := ""
	if conn.Password != "" {
		if d, err := crypto.Decrypt(conn.Password); err == nil {
			decryptedPassword = d
		} else {
			decryptedPassword = conn.Password
		}
	}

	port := conn.Port
	if port == 0 {
		port = 3306
	}

	var dsn string
	if decryptedPassword != "" {
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			conn.Username, decryptedPassword, conn.Host, port, conn.Database)
	} else if conn.Username != "" {
		dsn = fmt.Sprintf("%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			conn.Username, conn.Host, port, conn.Database)
	} else {
		dsn = fmt.Sprintf("tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			conn.Host, port, conn.Database)
	}

	return dsn, nil
}
