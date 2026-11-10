// 数据库连接路由 — 对应前端 DbConnections.tsx + TS dbConnectionsRoutes.ts
// 路径前缀 /api/v1/db-connections
package routes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/crypto"
	"itops-backend/internal/httpx"
	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/database/services"
	"itops-backend/internal/repos"
)

// FlexTags 兼容 string 或 string[] 两种 JSON 格式
type FlexTags []string

func (ft *FlexTags) UnmarshalJSON(data []byte) error {
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*ft = arr
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if str == "" {
			*ft = []string{}
			return nil
		}
		parts := strings.Split(str, ",")
		var cleaned []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				cleaned = append(cleaned, p)
			}
		}
		*ft = cleaned
		return nil
	}
	return nil
}

func (ft *FlexTags) ToStringPtr() *string {
	if ft == nil || len(*ft) == 0 {
		return nil
	}
	s := strings.Join(*ft, ",")
	return &s
}

// isInfluxdb 判断 db_type 是否为 InfluxDB 类型
func isInfluxdb(dbType string) bool {
	norm := strings.ToLower(strings.TrimSpace(dbType))
	return strings.Contains(norm, "influx")
}

// defaultPort 根据 db_type 返回默认端口
func defaultPort(dbType string) int {
	norm := strings.ToLower(strings.TrimSpace(dbType))
	switch {
	case strings.Contains(norm, "influx"):
		return 8086
	case norm == "postgresql":
		return 5432
	case norm == "oracle":
		return 1521
	default:
		return 3306
	}
}

// parseJSON 解析请求 JSON body
func parseJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		httpx.RespondError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// DbConnectionRoutes 数据库连接管理路由
func DbConnectionRoutes() chi.Router {
	r := chi.NewRouter()

	// GET / — 列出所有连接（抹去敏感字段）
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		conns, err := repos.DbConnections.ListAll()
		if err != nil {
			httpx.RespondError(w, "Failed to get database connections", http.StatusInternalServerError)
			return
		}
		// 抹去密码和 token
		for i := range conns {
			conns[i].Password = ""
			if conns[i].Token != nil {
				conns[i].Token = strPtr("")
			}
		}
		httpx.Respond(w, conns)
	})

	// GET /{id} — 获取单个连接（抹去敏感字段）
	r.With(middleware.RequireRole("admin", "operator")).Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		conn, err := repos.DbConnections.GetByID(id)
		if err != nil {
			httpx.RespondError(w, "Failed to get database connection", http.StatusInternalServerError)
			return
		}
		if conn == nil {
			httpx.RespondError(w, "Database connection not found", http.StatusNotFound)
			return
		}
		conn.Password = ""
		if conn.Token != nil {
			conn.Token = strPtr("")
		}
		httpx.Respond(w, conn)
	})

	// POST / — 创建数据库连接
	r.With(middleware.RequireRole("admin", "operator")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name            string    `json:"name"`
			DBType          string    `json:"db_type"`
			Host            string    `json:"host"`
			Port            int       `json:"port"`
			Username        string    `json:"username"`
			Password        string    `json:"password"`
			Database        string    `json:"database"`
			Description     *string   `json:"description"`
			Tags            *FlexTags `json:"tags"`
			Token           *string   `json:"token"`
			Organization    *string   `json:"organization"`
			Bucket          *string   `json:"bucket"`
			RetentionPolicy *string   `json:"retention_policy"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		isInflux := isInfluxdb(req.DBType)
		if req.Name == "" || req.Host == "" {
			httpx.RespondError(w, "Missing required fields: name, host", http.StatusBadRequest)
			return
		}
		if !isInflux && (req.Username == "" || req.Password == "" || req.Database == "") {
			httpx.RespondError(w, "Missing required fields: username, password, database", http.StatusBadRequest)
			return
		}

		id := uuid.New().String()

		// 加密密码
		encryptedPassword := ""
		if req.Password != "" {
			encryptedPassword = crypto.Encrypt(req.Password)
		}

		// 加密 token（仅 InfluxDB v2/v3）
		var encryptedToken *string
		if req.Token != nil && *req.Token != "" {
			et := crypto.Encrypt(*req.Token)
			encryptedToken = &et
		}

		port := req.Port
		if port == 0 {
			port = defaultPort(req.DBType)
		}

		dbType := req.DBType
		if dbType == "" {
			dbType = "mysql"
		}

		var tagsPtr *string
		if req.Tags != nil {
			tagsPtr = req.Tags.ToStringPtr()
		}

		conn := &repos.DbConnection{
			ID:              id,
			Name:            req.Name,
			DBType:          dbType,
			Host:            req.Host,
			Port:            port,
			Username:        req.Username,
			Password:        encryptedPassword,
			Database:        req.Database,
			Description:     req.Description,
			Tags:            tagsPtr,
			Enabled:         1,
			Token:           encryptedToken,
			Organization:    req.Organization,
			Bucket:          req.Bucket,
			RetentionPolicy: req.RetentionPolicy,
		}

		if err := repos.DbConnections.Insert(conn); err != nil {
			logger.Errorf("Failed to create database connection: %v", err)
			httpx.RespondError(w, "创建数据库连接失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		httpx.Respond(w, map[string]string{"id": id})
	})

	// PUT /{id} — 更新数据库连接
	r.With(middleware.RequireRole("admin", "operator")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		existing, err := repos.DbConnections.GetByID(id)
		if err != nil {
			httpx.RespondError(w, "Failed to get database connection", http.StatusInternalServerError)
			return
		}
		if existing == nil {
			httpx.RespondError(w, "Database connection not found", http.StatusNotFound)
			return
		}

		var req struct {
			Name            string    `json:"name"`
			DBType          string    `json:"db_type"`
			Host            string    `json:"host"`
			Port            int       `json:"port"`
			Username        string    `json:"username"`
			Password        string    `json:"password"`
			Database        string    `json:"database"`
			Description     *string   `json:"description"`
			Tags            *FlexTags `json:"tags"`
			Enabled         *int      `json:"enabled"`
			Token           *string   `json:"token"`
			Organization    *string   `json:"organization"`
			Bucket          *string   `json:"bucket"`
			RetentionPolicy *string   `json:"retention_policy"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		// 密码：有新值则加密，否则保留旧值
		encryptedPassword := existing.Password
		if req.Password != "" {
			encryptedPassword = crypto.Encrypt(req.Password)
		}

		// Token：有新值则加密，否则保留旧值
		encryptedToken := existing.Token
		if req.Token != nil && *req.Token != "" {
			et := crypto.Encrypt(*req.Token)
			encryptedToken = &et
		}

		// 字段合并
		name := req.Name
		if name == "" {
			name = existing.Name
		}
		dbType := req.DBType
		if dbType == "" {
			dbType = existing.DBType
		}
		host := req.Host
		if host == "" {
			host = existing.Host
		}
		port := req.Port
		if port == 0 {
			port = existing.Port
		}
		username := existing.Username
		if req.Username != "" {
			username = req.Username
		}
		database := existing.Database
		if req.Database != "" {
			database = req.Database
		}
		enabled := existing.Enabled
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		description := existing.Description
		if req.Description != nil {
			description = req.Description
		}
		tags := existing.Tags
		if req.Tags != nil {
			tags = req.Tags.ToStringPtr()
		}
		organization := existing.Organization
		if req.Organization != nil {
			organization = req.Organization
		}
		bucket := existing.Bucket
		if req.Bucket != nil {
			bucket = req.Bucket
		}
		retentionPolicy := existing.RetentionPolicy
		if req.RetentionPolicy != nil {
			retentionPolicy = req.RetentionPolicy
		}

		conn := &repos.DbConnection{
			Name:            name,
			DBType:          dbType,
			Host:            host,
			Port:            port,
			Username:        username,
			Password:        encryptedPassword,
			Database:        database,
			Description:     description,
			Tags:            tags,
			Enabled:         enabled,
			Token:           encryptedToken,
			Organization:    organization,
			Bucket:          bucket,
			RetentionPolicy: retentionPolicy,
		}

		if err := repos.DbConnections.Update(id, conn); err != nil {
			logger.Errorf("Failed to update database connection: %v", err)
			httpx.RespondError(w, "更新数据库连接失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		httpx.RespondMessage(w, "Database connection updated")
	})

	// DELETE /{id} — 删除数据库连接
	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := repos.DbConnections.Delete(id); err != nil {
			logger.Errorf("Failed to delete database connection: %v", err)
			httpx.RespondError(w, "删除数据库连接失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "Database connection deleted")
	})

	// POST /{id}/test — 测试已保存的连接
	r.With(middleware.RequireRole("admin", "operator")).Post("/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		conn, err := repos.DbConnections.GetByID(id)
		if err != nil {
			httpx.RespondError(w, "Failed to get database connection", http.StatusInternalServerError)
			return
		}
		if conn == nil {
			httpx.RespondError(w, "Database connection not found", http.StatusNotFound)
			return
		}

		// InfluxDB 类型走 HTTP 测试
		if isInfluxdb(conn.DBType) {
			decryptedPassword := ""
			if conn.Password != "" {
				if d, err := crypto.Decrypt(conn.Password); err == nil {
					decryptedPassword = d
				} else {
					decryptedPassword = conn.Password
				}
			}
			decryptedToken := ""
			if conn.Token != nil && *conn.Token != "" {
				if d, err := crypto.Decrypt(*conn.Token); err == nil {
					decryptedToken = d
				} else {
					decryptedToken = *conn.Token
				}
			}

			success, message, detail := services.TestInfluxdbConnection(
				conn.DBType, conn.Host, conn.Port,
				conn.Username, decryptedPassword, conn.Database,
				decryptedToken,
				derefStr(conn.Organization),
				derefStr(conn.Bucket),
			)
			if success {
				httpx.Respond(w, map[string]string{"message": message})
			} else {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   message,
					"detail":  detail,
				})
			}
			return
		}

		// 关系型数据库 (MySQL 等) 测试
		decryptedPassword := ""
		if conn.Password != "" {
			if d, err := crypto.Decrypt(conn.Password); err == nil {
				decryptedPassword = d
			} else {
				decryptedPassword = conn.Password
			}
		}
		success, message, detail := testRelationalDBConnection(conn.DBType, conn.Host, conn.Port, conn.Username, decryptedPassword, conn.Database)
		if success {
			httpx.Respond(w, map[string]string{"message": message})
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			errMsg := message
			if detail != "" {
				errMsg = fmt.Sprintf("%s: %s", message, detail)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   errMsg,
				"message": errMsg,
				"detail":  detail,
			})
		}
	})

	// POST /test-connect — 直接测试连接（不保存）
	r.With(middleware.RequireRole("admin", "operator")).Post("/test-connect", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DBType       string `json:"db_type"`
			Host         string `json:"host"`
			Port         int    `json:"port"`
			Username     string `json:"username"`
			Password     string `json:"password"`
			Database     string `json:"database"`
			Token        string `json:"token"`
			Organization string `json:"organization"`
			Bucket       string `json:"bucket"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		if req.Host == "" {
			httpx.RespondError(w, "Missing required fields: host", http.StatusBadRequest)
			return
		}

		// InfluxDB 类型走 HTTP 测试
		if isInfluxdb(req.DBType) {
			port := req.Port
			if port == 0 {
				port = 8086
			}
			success, message, detail := services.TestInfluxdbConnection(
				req.DBType, req.Host, port,
				req.Username, req.Password, req.Database,
				req.Token, req.Organization, req.Bucket,
			)
			if success {
				httpx.Respond(w, map[string]string{"message": message})
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				errMsg := message
				if detail != "" {
					errMsg = fmt.Sprintf("%s: %s", message, detail)
				}
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   errMsg,
					"message": errMsg,
					"detail":  detail,
				})
			}
			return
		}

		// 关系型数据库 (MySQL 等) 测试
		success, message, detail := testRelationalDBConnection(req.DBType, req.Host, req.Port, req.Username, req.Password, req.Database)
		if success {
			httpx.Respond(w, map[string]string{"message": message})
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			errMsg := message
			if detail != "" {
				errMsg = fmt.Sprintf("%s: %s", message, detail)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   errMsg,
				"message": errMsg,
				"detail":  detail,
			})
		}
	})

	return r
}

// testRelationalDBConnection 测试关系型数据库连接（MySQL 原生 Ping / 其他类型 TCP 探测）
func testRelationalDBConnection(dbType, host string, port int, username, password, database string) (bool, string, string) {
	if dbType == "" || dbType == "mysql" {
		if port == 0 {
			port = 3306
		}
		var dsn string
		if password != "" {
			dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=5s&charset=utf8mb4&parseTime=True&loc=Local", username, password, host, port, database)
		} else if username != "" {
			dsn = fmt.Sprintf("%s@tcp(%s:%d)/%s?timeout=5s&charset=utf8mb4&parseTime=True&loc=Local", username, host, port, database)
		} else {
			dsn = fmt.Sprintf("tcp(%s:%d)/%s?timeout=5s&charset=utf8mb4&parseTime=True&loc=Local", host, port, database)
		}
		testDB, err := sql.Open("mysql", dsn)
		if err != nil {
			return false, "MySQL 配置参数解析失败", err.Error()
		}
		defer testDB.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := testDB.PingContext(ctx); err != nil {
			return false, "MySQL 连接测试失败", err.Error()
		}
		return true, "MySQL 数据库连接成功！", ""
	}

	// 针对其他类型，尝试 TCP 端口连通性探测
	if port == 0 {
		port = defaultPort(dbType)
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return false, fmt.Sprintf("%s 端口 (%s) 无法连接", dbType, addr), err.Error()
	}
	conn.Close()
	return true, fmt.Sprintf("%s 主机与端口连通正常", dbType), ""
}

// strPtr 辅助：string → *string
func strPtr(s string) *string {
	return &s
}

// derefStr 辅助：*string → string（nil 返回空字符串）
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
