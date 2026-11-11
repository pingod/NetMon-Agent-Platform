// AI Models 路由 — 对应 TS aiModelRoutes.ts
// 挂载路径：/api/v1/ai-models
package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
)

// maskAPIKey 对外响应一律脱敏：仅保留前 3 位与后 4 位，完整密钥不出 HTTP 层
func maskAPIKey(key *string) *string {
	if key == nil || *key == "" {
		return key
	}
	k := *key
	var masked string
	if len(k) <= 8 {
		masked = "******"
	} else {
		masked = k[:3] + "****" + k[len(k)-4:]
	}
	return &masked
}

func maskModel(m *repos.AIModel) {
	if m != nil {
		m.APIKey = maskAPIKey(m.APIKey)
	}
}

// AIModelRoutes AI 模型管理路由
func AIModelRoutes() chi.Router {
	r := chi.NewRouter()

	// GET / — 模型列表（api_key 脱敏，完整密钥仅服务端内部使用）
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		models, err := repos.AIModels.List()
		if err != nil {
			httpx.RespondError(w, "获取模型列表失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for i := range models {
			maskModel(&models[i])
		}
		httpx.Respond(w, models)
	})

	// POST / — 创建模型
	r.With(middleware.RequireRole("admin")).Post("/", func(w http.ResponseWriter, r *http.Request) {
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
		maskModel(model)
		httpx.Respond(w, model)
	})

	// PUT /reorder — 重新排序模型
	r.With(middleware.RequireRole("admin")).Put("/reorder", func(w http.ResponseWriter, r *http.Request) {
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
	r.With(middleware.RequireRole("admin")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
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
		maskModel(model)
		httpx.Respond(w, model)
	})

	// DELETE /{id} — 删除模型
	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := repos.AIModels.Delete(id); err != nil {
			httpx.RespondError(w, "删除模型失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "删除成功")
	})

	// POST /{id}/test — 真实调用一次 chat/completions 验证连通性
	r.With(middleware.RequireRole("admin")).Post("/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		model, err := repos.AIModels.GetByID(id)
		if err != nil || model == nil {
			httpx.RespondError(w, "模型不存在", http.StatusNotFound)
			return
		}

		apiBase := "https://api.openai.com/v1"
		if model.APIBase != nil && *model.APIBase != "" {
			apiBase = strings.TrimRight(*model.APIBase, "/")
		}
		reqBody, _ := json.Marshal(map[string]interface{}{
			"model":      model.ModelID,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
			"max_tokens": 1,
		})
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, apiBase+"/chat/completions", bytes.NewReader(reqBody))
		if err != nil {
			httpx.RespondError(w, "构造测试请求失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if model.APIKey != nil && *model.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+*model.APIKey)
		}

		client := &http.Client{Timeout: 20 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			_ = repos.AIModels.UpdateTestStatus(id, "failed")
			httpx.Respond(w, map[string]interface{}{"success": false, "message": "连接失败: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			var errBody struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&errBody)
			msg := errBody.Error.Message
			if msg == "" {
				msg = resp.Status
			}
			_ = repos.AIModels.UpdateTestStatus(id, "failed")
			httpx.Respond(w, map[string]interface{}{"success": false, "message": fmt.Sprintf("接口返回 %d: %s", resp.StatusCode, msg)})
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
