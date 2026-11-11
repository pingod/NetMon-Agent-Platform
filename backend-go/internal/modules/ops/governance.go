package ops

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/services/audit"
)

// KnowledgeRoutes 知识库路由 (/api/v1/knowledge)
func KnowledgeRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		category := r.URL.Query().Get("category")
		searchQ := r.URL.Query().Get("q")
		db := appdb.DB()

		query := `SELECT id, title, category, content, tags, solutions, related_alerts, usage_count, created_at, updated_at FROM knowledge_base WHERE 1=1`
		var args []interface{}
		if category != "" && category != "all" {
			query += ` AND category = ?`
			args = append(args, category)
		}
		if searchQ != "" {
			query += ` AND (title LIKE ? OR content LIKE ? OR tags LIKE ?)`
			args = append(args, "%"+searchQ+"%", "%"+searchQ+"%", "%"+searchQ+"%")
		}
		query += ` ORDER BY updated_at DESC LIMIT 100`

		rows, err := db.Query(query, args...)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()

		var list []map[string]interface{}
		for rows.Next() {
			var id, title, content string
			var categoryStr, tagsStr, solutions, relatedAlerts, createdAt, updatedAt sql.NullString
			var usageCount int
			if rows.Scan(&id, &title, &categoryStr, &content, &tagsStr, &solutions, &relatedAlerts, &usageCount, &createdAt, &updatedAt) == nil {
				var tagsObj interface{}
				if tagsStr.Valid && tagsStr.String != "" {
					_ = json.Unmarshal([]byte(tagsStr.String), &tagsObj)
				}
				if tagsObj == nil {
					tagsObj = []string{}
				}
				list = append(list, map[string]interface{}{
					"id":             id,
					"title":          title,
					"category":       categoryStr.String,
					"content":        content,
					"tags":           tagsObj,
					"solutions":      solutions.String,
					"related_alerts": relatedAlerts.String,
					"usage_count":    usageCount,
					"created_at":     createdAt.String,
					"updated_at":     updatedAt.String,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	r.With(middleware.RequireRole("admin")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Title    string      `json:"title"`
			Category string      `json:"category"`
			Content  string      `json:"content"`
			Tags     interface{} `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		tagsJSON, _ := json.Marshal(req.Tags)
		id := uuid.NewString()
		now := time.Now().Format("2006-01-02 15:04:05")

		_, err := appdb.DB().Exec(`
			INSERT INTO knowledge_base (id, title, category, content, tags, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, req.Title, req.Category, req.Content, string(tagsJSON), now, now)
		if err != nil {
			httpx.RespondError(w, "创建知识文章失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, map[string]interface{}{"id": id, "title": req.Title})
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var title, content string
		var categoryStr, tagsStr, solutions, relatedAlerts, createdAt, updatedAt sql.NullString
		var usageCount int
		err := appdb.DB().QueryRow(`SELECT id, title, category, content, tags, solutions, related_alerts, usage_count, created_at, updated_at FROM knowledge_base WHERE id = ?`, id).
			Scan(&id, &title, &categoryStr, &content, &tagsStr, &solutions, &relatedAlerts, &usageCount, &createdAt, &updatedAt)
		if err != nil {
			httpx.RespondError(w, "文章不存在", http.StatusNotFound)
			return
		}
		var tagsObj interface{}
		if tagsStr.Valid && tagsStr.String != "" {
			_ = json.Unmarshal([]byte(tagsStr.String), &tagsObj)
		}
		if tagsObj == nil {
			tagsObj = []string{}
		}
		httpx.Respond(w, map[string]interface{}{
			"id":             id,
			"title":          title,
			"category":       categoryStr.String,
			"content":        content,
			"tags":           tagsObj,
			"solutions":      solutions.String,
			"related_alerts": relatedAlerts.String,
			"usage_count":    usageCount,
			"created_at":     createdAt.String,
			"updated_at":     updatedAt.String,
		})
	})

	r.With(middleware.RequireRole("admin")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			Title    *string     `json:"title"`
			Category *string     `json:"category"`
			Content  *string     `json:"content"`
			Tags     interface{} `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		tagsJSON, _ := json.Marshal(req.Tags)
		now := time.Now().Format("2006-01-02 15:04:05")

		_, err := appdb.DB().Exec(`
			UPDATE knowledge_base SET
				title = COALESCE(?, title),
				category = COALESCE(?, category),
				content = COALESCE(?, content),
				tags = CASE WHEN ? != 'null' THEN ? ELSE tags END,
				updated_at = ?
			WHERE id = ?`,
			req.Title, req.Category, req.Content, string(tagsJSON), string(tagsJSON), now, id)
		if err != nil {
			httpx.RespondError(w, "更新文章失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{"id": id, "updated_at": now})
	})

	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, _ = appdb.DB().Exec(`DELETE FROM knowledge_base WHERE id = ?`, id)
		httpx.RespondMessage(w, "文章已删除")
	})

	return r
}

// AuditRoutes 审计日志路由 (/api/v1/audit, /api/v1/audit-logs)
func AuditRoutes() chi.Router {
	r := chi.NewRouter()

	handleLogs := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 {
			limit = 50
		}
		db := appdb.DB()
		rows, err := db.Query(`SELECT id, user_id, action, resource_type, resource_id, details, ip_address, created_at FROM audit_logs ORDER BY created_at DESC LIMIT ?`, limit)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()

		var list []map[string]interface{}
		for rows.Next() {
			var id, action string
			var userID, resType, resID, details, ipAddr, createdAt sql.NullString
			if rows.Scan(&id, &userID, &action, &resType, &resID, &details, &ipAddr, &createdAt) == nil {
				list = append(list, map[string]interface{}{
					"id":            id,
					"user_id":       userID.String,
					"action":        action,
					"resource_type": resType.String,
					"resource_id":   resID.String,
					"details":       details.String,
					"ip_address":    ipAddr.String,
					"created_at":    createdAt.String,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	}

	r.Get("/", handleLogs)
	r.Get("/logs", handleLogs)

	return r
}

// ToolLinkRoutes 工具链接路由 (/api/v1/tool-links)
func ToolLinkRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT id, name, url, icon, category, description, sort_order, is_external, created_at FROM tool_links ORDER BY sort_order ASC, created_at DESC`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()

		var list []map[string]interface{}
		for rows.Next() {
			var id, name, url string
			var icon, category, desc, createdAt sql.NullString
			var sortOrder, isExternal int
			if rows.Scan(&id, &name, &url, &icon, &category, &desc, &sortOrder, &isExternal, &createdAt) == nil {
				list = append(list, map[string]interface{}{
					"id":          id,
					"name":        name,
					"title":       name,
					"url":         url,
					"icon":        icon.String,
					"category":    category.String,
					"description": desc.String,
					"sort_order":  sortOrder,
					"is_external": isExternal == 1,
					"created_at":  createdAt.String,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	r.With(middleware.RequireRole("admin")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        string `json:"name"`
			Title       string `json:"title"`
			URL         string `json:"url"`
			Icon        string `json:"icon"`
			Category    string `json:"category"`
			Description string `json:"description"`
			SortOrder   int    `json:"sort_order"`
			IsExternal  bool   `json:"is_external"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		name := req.Name
		if name == "" {
			name = req.Title
		}
		isExtInt := 0
		if req.IsExternal {
			isExtInt = 1
		}
		id := uuid.NewString()
		now := time.Now().Format("2006-01-02 15:04:05")

		_, err := appdb.DB().Exec(`
			INSERT INTO tool_links (id, name, url, icon, category, description, sort_order, is_external, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, name, req.URL, req.Icon, req.Category, req.Description, req.SortOrder, isExtInt, now, now)
		if err != nil {
			httpx.RespondError(w, "创建工具链接失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, map[string]interface{}{"id": id, "name": name})
	})

	r.With(middleware.RequireRole("admin")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			Name        *string `json:"name"`
			Title       *string `json:"title"`
			URL         *string `json:"url"`
			Icon        *string `json:"icon"`
			Category    *string `json:"category"`
			Description *string `json:"description"`
			SortOrder   *int    `json:"sort_order"`
			IsExternal  *bool   `json:"is_external"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		name := req.Name
		if name == nil && req.Title != nil {
			name = req.Title
		}
		var isExtVal *int
		if req.IsExternal != nil {
			v := 0
			if *req.IsExternal {
				v = 1
			}
			isExtVal = &v
		}
		now := time.Now().Format("2006-01-02 15:04:05")

		_, err := appdb.DB().Exec(`
			UPDATE tool_links SET
				name = COALESCE(?, name),
				url = COALESCE(?, url),
				icon = COALESCE(?, icon),
				category = COALESCE(?, category),
				description = COALESCE(?, description),
				sort_order = COALESCE(?, sort_order),
				is_external = COALESCE(?, is_external),
				updated_at = ?
			WHERE id = ?`,
			name, req.URL, req.Icon, req.Category, req.Description, req.SortOrder, isExtVal, now, id)
		if err != nil {
			httpx.RespondError(w, "更新工具链接失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{"id": id, "updated_at": now})
	})

	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, _ = appdb.DB().Exec(`DELETE FROM tool_links WHERE id = ?`, id)
		httpx.RespondMessage(w, "工具链接已删除")
	})

	return r
}

// ApprovalRoutes 审批流路由 (/api/v1/approvals)
func ApprovalRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`
			SELECT a.id, a.task_id, a.node_id, a.node_label, a.description, a.status, a.requested_by, a.approved_by, a.approved_at, a.created_at, COALESCE(t.name, '') as task_name
			FROM approval_requests a LEFT JOIN tasks t ON a.task_id = t.id
			ORDER BY a.created_at DESC LIMIT 50`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()

		var list []map[string]interface{}
		for rows.Next() {
			var id, taskID, nodeID, nodeLabel, status string
			var desc, reqBy, appBy, appAt, createdAt, taskName sql.NullString
			if rows.Scan(&id, &taskID, &nodeID, &nodeLabel, &desc, &status, &reqBy, &appBy, &appAt, &createdAt, &taskName) == nil {
				list = append(list, map[string]interface{}{
					"id":           id,
					"task_id":      taskID,
					"task_name":    taskName.String,
					"node_id":      nodeID,
					"node_label":   nodeLabel,
					"description":  desc.String,
					"status":       status,
					"requested_by": reqBy.String,
					"approved_by":  appBy.String,
					"approved_at":  appAt.String,
					"created_at":   createdAt.String,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	r.With(middleware.RequireRole("admin")).Post("/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var taskID, status string
		if appdb.DB().QueryRow(`SELECT task_id, status FROM approval_requests WHERE id = ?`, id).Scan(&taskID, &status) != nil {
			httpx.RespondError(w, "审批请求不存在", http.StatusNotFound)
			return
		}
		if status != "pending" {
			httpx.RespondError(w, "该审批已处理", http.StatusConflict)
			return
		}
		user := middleware.GetUser(r)
		now := time.Now().Format("2006-01-02 15:04:05")
		appdb.DB().Exec(`UPDATE approval_requests SET status = 'approved', approved_by = ?, approved_at = ?, updated_at = ? WHERE id = ?`, user.Username, now, now, id)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "approval_approve", ResourceType: "approval_request", ResourceID: id, Details: map[string]interface{}{"task_id": taskID}, IPAddress: middleware.ClientIP(r)})
		// 恢复挂起的工作流任务
		res, _ := appdb.DB().Exec(`UPDATE tasks SET status = 'running', updated_at = ? WHERE id = ? AND status = 'waiting_approval'`, now, taskID)
		if n, _ := res.RowsAffected(); n > 0 {
			StartTaskExecution(taskID)
		}
		httpx.RespondMessage(w, "审批已通过")
	})

	r.With(middleware.RequireRole("admin")).Post("/{id}/reject", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var taskID, status string
		if appdb.DB().QueryRow(`SELECT task_id, status FROM approval_requests WHERE id = ?`, id).Scan(&taskID, &status) != nil {
			httpx.RespondError(w, "审批请求不存在", http.StatusNotFound)
			return
		}
		if status != "pending" {
			httpx.RespondError(w, "该审批已处理", http.StatusConflict)
			return
		}
		user := middleware.GetUser(r)
		now := time.Now().Format("2006-01-02 15:04:05")
		appdb.DB().Exec(`UPDATE approval_requests SET status = 'rejected', approved_by = ?, approved_at = ?, reject_reason = ?, updated_at = ? WHERE id = ?`, user.Username, now, body.Reason, now, id)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "approval_reject", ResourceType: "approval_request", ResourceID: id, Details: map[string]interface{}{"task_id": taskID, "reason": body.Reason}, IPAddress: middleware.ClientIP(r)})
		failTask(taskID, "审批被拒绝: "+body.Reason)
		httpx.RespondMessage(w, "审批已驳回")
	})

	return r
}

// AiRemediationRoutes AI 自愈方案路由 (/api/v1/ai-remediations)
func AiRemediationRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT id, title, description, alert_id, status, strategy, result, device_id, device_name, device_ip, risk_level, created_at FROM ai_remediations ORDER BY created_at DESC LIMIT 50`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()
		var list []map[string]interface{}
		for rows.Next() {
			var id, title, status string
			var desc, alertID, strategy, result, devID, devName, devIP, riskLevel, createdAt sql.NullString
			if rows.Scan(&id, &title, &desc, &alertID, &status, &strategy, &result, &devID, &devName, &devIP, &riskLevel, &createdAt) == nil {
				list = append(list, map[string]interface{}{
					"id":          id,
					"title":       title,
					"description": desc.String,
					"alert_id":    alertID.String,
					"status":      status,
					"strategy":    strategy.String,
					"result":      result.String,
					"device_id":   devID.String,
					"device_name": devName.String,
					"device_ip":   devIP.String,
					"risk_level":  riskLevel.String,
					"created_at":  createdAt.String,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	return r
}

// RemediationRoutes 自愈策略路由 (/api/v1/remediations)
func RemediationRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		var policyCount, executionCount, successCount int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM remediation_policies`).Scan(&policyCount)
		appdb.DB().QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status='success' OR status='completed' THEN 1 ELSE 0 END),0) FROM remediation_executions`).Scan(&executionCount, &successCount)
		successRate := 0.0
		if executionCount > 0 {
			successRate = float64(successCount) / float64(executionCount) * 100
		}
		httpx.Respond(w, map[string]interface{}{
			"policy_count":    policyCount,
			"execution_count": executionCount,
			"success_count":   successCount,
			"success_rate":    successRate,
		})
	})

	r.Get("/policies", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT id, name, description, alert_source, alert_severity, execution_mode, enabled, created_at FROM remediation_policies ORDER BY created_at DESC`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()
		var list []map[string]interface{}
		for rows.Next() {
			var id, name, alertSource, execMode, createdAt string
			var desc, alertSev sql.NullString
			var enabled int
			if rows.Scan(&id, &name, &desc, &alertSource, &alertSev, &execMode, &enabled, &createdAt) == nil {
				list = append(list, map[string]interface{}{
					"id":             id,
					"name":           name,
					"description":    desc.String,
					"alert_source":   alertSource,
					"alert_severity": alertSev.String,
					"execution_mode": execMode,
					"enabled":        enabled == 1,
					"created_at":     createdAt,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	r.Get("/executions", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`
			SELECT e.id, e.policy_id, e.alert_id, e.status, e.started_at, e.completed_at, e.created_at, COALESCE(p.name, '') as policy_name
			FROM remediation_executions e LEFT JOIN remediation_policies p ON e.policy_id = p.id
			ORDER BY e.created_at DESC LIMIT 50`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()
		var list []map[string]interface{}
		for rows.Next() {
			var id, policyID, alertID, status, createdAt, policyName string
			var startedAt, completedAt sql.NullString
			if rows.Scan(&id, &policyID, &alertID, &status, &startedAt, &completedAt, &createdAt, &policyName) == nil {
				list = append(list, map[string]interface{}{
					"id":           id,
					"policy_id":    policyID,
					"policy_name":  policyName,
					"alert_id":     alertID,
					"status":       status,
					"started_at":   startedAt.String,
					"completed_at": completedAt.String,
					"created_at":   createdAt,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	return r
}

