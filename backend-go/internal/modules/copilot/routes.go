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
		userID := "admin"
		if u := middleware.GetUser(r); u != nil && u.ID != "" {
			userID = u.ID
		}

		list, err := repos.CopilotConversations.List(userID)
		if err != nil {
			httpx.RespondError(w, "获取对话列表失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, list)
	})

	// POST /conversations — 创建新会话
	r.Post("/conversations", func(w http.ResponseWriter, r *http.Request) {
		userID := "admin"
		if u := middleware.GetUser(r); u != nil && u.ID != "" {
			userID = u.ID
		}

		conv, err := repos.CopilotConversations.Create(userID)
		if err != nil {
			httpx.RespondError(w, "创建会话失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, conv)
	})

	// DELETE /conversations/{id} — 删除指定会话
	r.Delete("/conversations/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			httpx.RespondError(w, "会话ID不能为空", http.StatusBadRequest)
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

		userID := "admin"
		if u := middleware.GetUser(r); u != nil && u.ID != "" {
			userID = u.ID
		}

		// 确保会话存在
		convID := req.ConversationID
		if convID == "" {
			newConv, err := repos.CopilotConversations.Create(userID)
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

		// 1. 调用 Copilot 对话驱动引擎
		assistantReply := ProcessMessage(r.Context(), req.Message)

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
