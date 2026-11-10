// Copilot 路由 — 提供智能运维助手对话、会话管理与快捷推荐问答
// 挂载路径：/api/v1/copilot
package copilot

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
)

// currentUser 读取当前登录用户；未认证时返回 nil。
// 会话归属校验一律以该用户 ID 为准，绝不使用固定 "admin" 兜底。
func currentUser(r *http.Request) *middleware.AuthUser {
	return middleware.GetUser(r)
}

// ensureConversationOwnership 校验会话存在且属于当前用户。
// 会话不存在或归属他人时统一返回 (nil, false)，调用方应返回 404，避免泄露他人会话是否存在。
// 说明：repos.CopilotConversationRepo 的 GetForUser(id, userID) 尚未提供，
// 这里用 GetByID + user_id 比对实现等价校验，后续可替换为 repo 层方法。
func ensureConversationOwnership(id, userID string) (*repos.CopilotConversation, bool, error) {
	conv, err := repos.CopilotConversations.GetByID(id)
	if err != nil {
		return nil, false, err
	}
	if conv == nil || conv.UserID != userID {
		return nil, false, nil
	}
	return conv, true, nil
}

// Routes 返回 Copilot 路由
func Routes() chi.Router {
	r := chi.NewRouter()

	// GET /suggestions — 快捷推荐问题
	r.Get("/suggestions", func(w http.ResponseWriter, r *http.Request) {
		suggestions := []string{
			"巡检所有网络设备",
			"帮我巡检核心交换机-1",
			"查一下数据库里有哪些表",
			"查一下数据库里的告警",
			"去设备查询配置 (display current-configuration)",
			"多设备版本比对 (display version)",
			"查询设备接口状态 (display interface brief)",
			"查询所有网络设备状态",
			"查看最新指标与采集数据",
			"列出交换机与服务器汇总表格",
			"展示当前网络拓扑结构与互联链路",
			"查看当前告警状态与自愈建议",
		}
		httpx.Respond(w, suggestions)
	})

	// GET /conversations — 会话历史列表
	r.Get("/conversations", func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil || u.ID == "" {
			httpx.RespondError(w, "未认证或会话已失效", http.StatusUnauthorized)
			return
		}

		list, err := repos.CopilotConversations.List(u.ID)
		if err != nil {
			httpx.RespondError(w, "获取对话列表失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, list)
	})

	// POST /conversations — 创建新会话
	r.Post("/conversations", func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil || u.ID == "" {
			httpx.RespondError(w, "未认证或会话已失效", http.StatusUnauthorized)
			return
		}

		conv, err := repos.CopilotConversations.Create(u.ID)
		if err != nil {
			httpx.RespondError(w, "创建会话失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, conv)
	})

	// DELETE /conversations/{id} — 删除指定会话（仅限本人）
	r.Delete("/conversations/{id}", func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil || u.ID == "" {
			httpx.RespondError(w, "未认证或会话已失效", http.StatusUnauthorized)
			return
		}

		id := chi.URLParam(r, "id")
		if id == "" {
			httpx.RespondError(w, "会话ID不能为空", http.StatusBadRequest)
			return
		}

		if _, ok, err := ensureConversationOwnership(id, u.ID); err != nil {
			httpx.RespondError(w, "删除会话失败: "+err.Error(), http.StatusInternalServerError)
			return
		} else if !ok {
			httpx.RespondError(w, "会话不存在", http.StatusNotFound)
			return
		}

		if err := repos.CopilotConversations.Delete(id); err != nil {
			httpx.RespondError(w, "删除会话失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "删除成功")
	})

	// POST /chat — 发送消息并获取智能回复
	r.Post("/chat", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ConversationID string `json:"conversationId"`
			Message        string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "无效请求参数: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Message == "" {
			httpx.RespondError(w, "消息内容不能为空", http.StatusBadRequest)
			return
		}

		u := currentUser(r)
		if u == nil || u.ID == "" {
			httpx.RespondError(w, "未认证或会话已失效", http.StatusUnauthorized)
			return
		}
		role := u.Role
		if role == "" {
			role = "admin"
		}

		// 会话归属校验：不存在或属于他人时一律 404
		convID := req.ConversationID
		if convID != "" {
			if _, ok, err := ensureConversationOwnership(convID, u.ID); err != nil {
				httpx.RespondError(w, "读取会话失败: "+err.Error(), http.StatusInternalServerError)
				return
			} else if !ok {
				httpx.RespondError(w, "会话不存在", http.StatusNotFound)
				return
			}
		} else {
			newConv, err := repos.CopilotConversations.Create(u.ID)
			if err != nil {
				httpx.RespondError(w, "创建会话失败: "+err.Error(), http.StatusInternalServerError)
				return
			}
			convID = newConv.ID
		}

		now := time.Now().Format("2006-01-02 15:04:05")
		userMsg := repos.CopilotMessage{
			Role:      "user",
			Content:   req.Message,
			Timestamp: now,
		}

		// 1. 调用 Copilot 对话驱动引擎（携带当前角色用于执行前安全策略校验）
		assistantReply := ProcessMessage(r.Context(), req.Message, role)

		replyTime := time.Now().Format("2006-01-02 15:04:05")
		assistantMsg := repos.CopilotMessage{
			Role:      "assistant",
			Content:   assistantReply,
			Timestamp: replyTime,
		}

		// 2. 将用户提问与助手回复保存至数据库会话
		updatedConv, err := repos.CopilotConversations.AppendMessages(convID, userMsg, assistantMsg)
		if err != nil {
			httpx.RespondError(w, "保存会话消息失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		httpx.Respond(w, map[string]interface{}{
			"conversationId": convID,
			"reply":          assistantReply,
			"conversation":   updatedConv,
		})
	})

	return r
}
