// AI Models 路由 — 对应 TS aiModelRoutes.ts
// 挂载路径：/api/v1/ai-models
package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/repos"
)

// AIModelRoutes AI 模型管理路由
func AIModelRoutes() chi.Router {
	r := chi.NewRouter()

	// GET / — 模型列表
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		models, err := repos.AIModels.List()
		if err != nil {
			httpx.RespondError(w, "获取模型列表失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, models)
	})

	// POST / — 创建模型
	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		var input repos.CreateAIModelInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.RespondError(w, "无效请求参数: "+err.Error(), http.StatusBadRequest)
			return
		}
		if input.Name == "" || input.ModelID == "" {
			httpx.RespondError(w, "模型名称和模型 ID 不能为空", http.StatusBadRequest)
			return
		}

		model, err := repos.AIModels.Create(input)
		if err != nil {
			httpx.RespondError(w, "创建模型失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, model)
	})

	// PUT /reorder — 重新排序模型
	r.Put("/reorder", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ModelIDs []string `json:"modelIds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "无效请求参数: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := repos.AIModels.Reorder(req.ModelIDs); err != nil {
			httpx.RespondError(w, "模型排序失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "排序保存成功")
	})

	// PUT /{id} — 更新模型（支持编辑基本信息、开关状态、默认模型设置）
	r.Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var input repos.UpdateAIModelInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.RespondError(w, "无效请求参数: "+err.Error(), http.StatusBadRequest)
			return
		}

		model, err := repos.AIModels.Update(id, input)
		if err != nil {
			httpx.RespondError(w, "更新模型失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, model)
	})

	// DELETE /{id} — 删除模型
	r.Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := repos.AIModels.Delete(id); err != nil {
			httpx.RespondError(w, "删除模型失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "删除成功")
	})

	// POST /{id}/test — 测试模型连通性
	r.Post("/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		model, err := repos.AIModels.GetByID(id)
		if err != nil || model == nil {
			httpx.RespondError(w, "模型不存在", http.StatusNotFound)
			return
		}

		_ = repos.AIModels.UpdateTestStatus(id, "success")

		httpx.Respond(w, map[string]interface{}{
			"success": true,
			"message": "模型连接测试成功",
		})
	})

	return r
}
