// VNC 配置路由 — 2 个端点
// 等价于 TS routes/vncRoutes.ts
package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
)

// VncRoutes VNC 配置路由
func VncRoutes() chi.Router {
	r := chi.NewRouter()

	// GET /config/{serverId} — 获取服务器 VNC 配置（密码由仓库层解密，限制 admin/operator 访问）
	r.With(middleware.RequireRole("admin", "operator")).Get("/config/{serverId}", func(w http.ResponseWriter, r *http.Request) {
		serverID := chi.URLParam(r, "serverId")
		hostname, port, password, err := repos.Servers.GetVncConfig(serverID)
		if err != nil {
			if contains(err.Error(), "not found") {
				httpx.RespondError(w, "服务器不存在", http.StatusNotFound)
				return
			}
			httpx.RespondError(w, "查询失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if port <= 0 {
			port = 5900
		}
		cfg := map[string]interface{}{
			"hostname":     hostname,
			"vnc_port":     port,
			"has_password": password != "",
		}
		httpx.Respond(w, cfg)
	})

	// PUT /config/{serverId} — 更新服务器 VNC 配置
	r.With(middleware.RequireRole("admin")).Put("/config/{serverId}", func(w http.ResponseWriter, r *http.Request) {
		serverID := chi.URLParam(r, "serverId")
		exists, _ := repos.Servers.Exists(serverID)
		if !exists {
			httpx.RespondError(w, "服务器不存在", http.StatusNotFound)
			return
		}

		var req struct {
			VncPort     int    `json:"vnc_port"`
			VncPassword string `json:"vnc_password"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		if req.VncPort == 0 {
			req.VncPort = 5900
		}
		port := req.VncPort
		update := repos.ServerUpdateInput{
			VncPort:     &port,
			VncPassword: &req.VncPassword,
		}
		if _, err := repos.Servers.Update(serverID, update); err != nil {
			httpx.RespondError(w, "更新 VNC 配置失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "updated")
	})

	return r
}
