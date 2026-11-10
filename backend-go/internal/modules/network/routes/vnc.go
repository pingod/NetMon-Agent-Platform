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

	// GET /config/{serverId} — 获取服务器 VNC 配置
	r.Get("/config/{serverId}", func(w http.ResponseWriter, r *http.Request) {
		serverID := chi.URLParam(r, "serverId")
		s, err := repos.Servers.GetByID(serverID)
		if err != nil {
			httpx.RespondError(w, "查询失败", http.StatusInternalServerError)
			return
		}
		if s == nil {
			httpx.RespondError(w, "服务器不存在", http.StatusNotFound)
			return
		}

		cfg := map[string]interface{}{
			"hostname": s.Hostname,
		}
		if s.VncPort != nil && *s.VncPort > 0 {
			cfg["vnc_port"] = *s.VncPort
		} else {
			cfg["vnc_port"] = 5900
		}
		if s.VncPassword != nil && *s.VncPassword != "" {
			cfg["vnc_password"] = *s.VncPassword
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
