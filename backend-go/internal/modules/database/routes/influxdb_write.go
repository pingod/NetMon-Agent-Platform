// InfluxDB 写入配置路由 — 对应前端 Settings.tsx + TS influxdbWriteRoutes.ts
// 路径前缀 /api/v1/influxdb-write
package routes

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/database/services"
	"itops-backend/internal/repos"
)

// InfluxdbWriteRoutes InfluxDB 写入配置路由
func InfluxdbWriteRoutes() chi.Router {
	r := chi.NewRouter()

	// GET / — 获取写入配置 + InfluxDB 连接列表
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		settings := services.GetInfluxdbSettings()

		// 获取所有 InfluxDB 类型的数据库连接
		allConns, err := repos.DbConnections.ListAll()
		if err != nil {
			httpx.RespondError(w, "Failed to get InfluxDB connections", http.StatusInternalServerError)
			return
		}

		type influxConnInfo struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			DBType string `json:"db_type"`
			Host   string `json:"host"`
			Port   int    `json:"port"`
			Enable int    `json:"enabled"`
		}

		influxConns := make([]influxConnInfo, 0)
		for _, c := range allConns {
			dbType := strings.ToLower(strings.TrimSpace(c.DBType))
			if strings.Contains(dbType, "influx") {
				influxConns = append(influxConns, influxConnInfo{
					ID:     c.ID,
					Name:   c.Name,
					DBType: c.DBType,
					Host:   c.Host,
					Port:   c.Port,
					Enable: c.Enabled,
				})
			}
		}

		httpx.Respond(w, map[string]interface{}{
			"settings":            settings,
			"influxdbConnections": influxConns,
		})
	})

	// PUT / — 更新写入配置
	r.With(middleware.RequireRole("admin")).Put("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Enabled           *int    `json:"enabled"`
			DBConnectionID    *string `json:"db_connection_id"`
			MeasurementPrefix *string `json:"measurement_prefix"`
			WriteMode         *string `json:"write_mode"`
			BatchSize         *int    `json:"batch_size"`
			FlushIntervalMs   *int    `json:"flush_interval_ms"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		// 验证 db_connection_id 有效且为 InfluxDB 类型
		if req.DBConnectionID != nil && *req.DBConnectionID != "" {
			conn, err := repos.DbConnections.GetByID(*req.DBConnectionID)
			if err != nil || conn == nil {
				httpx.RespondError(w, "指定的数据库连接不存在", http.StatusBadRequest)
				return
			}
			dbType := strings.ToLower(strings.TrimSpace(conn.DBType))
			if !strings.Contains(dbType, "influx") {
				httpx.RespondError(w, "指定的连接不是 InfluxDB 类型", http.StatusBadRequest)
				return
			}
		}

		// 如果请求启用但未指定连接，且当前配置中也没有连接，尝试自动绑定第一个可用的 InfluxDB 连接
		if req.Enabled != nil && *req.Enabled == 1 && (req.DBConnectionID == nil || *req.DBConnectionID == "") {
			curSettings := services.GetInfluxdbSettings()
			if curSettings == nil || curSettings.DBConnectionID == nil || *curSettings.DBConnectionID == "" {
				allConns, _ := repos.DbConnections.ListAll()
				for _, c := range allConns {
					dbType := strings.ToLower(strings.TrimSpace(c.DBType))
					if strings.Contains(dbType, "influx") && c.Enabled == 1 {
						autoID := c.ID
						req.DBConnectionID = &autoID
						break
					}
				}
				if req.DBConnectionID == nil {
					for _, c := range allConns {
						dbType := strings.ToLower(strings.TrimSpace(c.DBType))
						if strings.Contains(dbType, "influx") {
							autoID := c.ID
							req.DBConnectionID = &autoID
							break
						}
					}
				}
			}
		}

		fields := map[string]interface{}{}
		if req.Enabled != nil {
			fields["enabled"] = *req.Enabled
		}
		if req.DBConnectionID != nil {
			fields["db_connection_id"] = *req.DBConnectionID
		}
		if req.MeasurementPrefix != nil {
			fields["measurement_prefix"] = *req.MeasurementPrefix
		}
		if req.WriteMode != nil {
			fields["write_mode"] = *req.WriteMode
		}
		if req.BatchSize != nil {
			fields["batch_size"] = *req.BatchSize
		}
		if req.FlushIntervalMs != nil {
			fields["flush_interval_ms"] = *req.FlushIntervalMs
		}

		settings := services.UpdateInfluxdbSettings(fields)
		httpx.Respond(w, settings)
	})

	// POST /test-write — 发送一条测试数据
	r.With(middleware.RequireRole("admin")).Post("/test-write", func(w http.ResponseWriter, r *http.Request) {
		settings := services.GetInfluxdbSettings()
		if settings == nil || settings.DBConnectionID == nil || *settings.DBConnectionID == "" {
			httpx.RespondError(w, "请先选择 InfluxDB 连接", http.StatusBadRequest)
			return
		}

		conn, err := repos.DbConnections.GetByID(*settings.DBConnectionID)
		if err != nil || conn == nil {
			httpx.RespondError(w, "指定的 InfluxDB 数据库连接不存在", http.StatusBadRequest)
			return
		}

		err = services.TestWriteToInfluxdb(conn, settings.MeasurementPrefix)
		if err != nil {
			httpx.RespondError(w, "测试写入失败: "+err.Error(), http.StatusBadRequest)
			return
		}

		httpx.RespondMessage(w, "测试数据写入成功")
	})

	return r
}
