// 服务器分组路由 — 对应 TS serverGroupRoutes.ts
// 挂载路径：/api/v1/server-groups
package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
)

// ServerGroupRoutes 服务器分组路由
func ServerGroupRoutes() chi.Router {
	r := chi.NewRouter()

	// GET / — 获取分组列表/树
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		tree, err := repos.ServerGroups.Tree()
		if err != nil {
			httpx.RespondError(w, "获取分组列表失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, tree)
	})

	// GET /tree — 分组树
	r.Get("/tree", func(w http.ResponseWriter, r *http.Request) {
		tree, err := repos.ServerGroups.Tree()
		if err != nil {
			httpx.RespondError(w, "获取分组树失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, tree)
	})

	// POST / — 创建分组
	r.With(middleware.RequireRole("admin")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        string  `json:"name"`
			Description *string `json:"description"`
			ParentID    *string `json:"parent_id"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			httpx.RespondError(w, "分组名称不能为空", http.StatusBadRequest)
			return
		}
		desc := ""
		if req.Description != nil {
			desc = *req.Description
		}
		parent := ""
		if req.ParentID != nil {
			parent = *req.ParentID
		}
		g, err := repos.ServerGroups.Create(req.Name, desc, parent)
		if err != nil {
			httpx.RespondError(w, "创建分组失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, g)
	})

	// PUT /{id} — 更新分组
	r.With(middleware.RequireRole("admin")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		existing, _ := repos.ServerGroups.GetByID(id)
		if existing == nil {
			httpx.RespondError(w, "分组不存在", http.StatusNotFound)
			return
		}
		var req struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			ParentID    *string `json:"parent_id"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		name := existing.Name
		if req.Name != nil && *req.Name != "" {
			name = *req.Name
		}
		g, err := repos.ServerGroups.Update(id, name, req.Description, req.ParentID)
		if err != nil {
			httpx.RespondError(w, "更新分组失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, g)
	})

	return r
}
