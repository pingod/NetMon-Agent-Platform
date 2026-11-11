package ops

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
)

// WorkflowRoutes 工作流路由 (/api/v1/workflows)
func WorkflowRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		db := appdb.DB()
		rows, err := db.Query(`SELECT id, name, description, nodes, edges, is_template, created_at, updated_at FROM workflows ORDER BY created_at DESC`)
		if err != nil {
			httpx.RespondError(w, "获取工作流列表失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var list []map[string]interface{}
		for rows.Next() {
			var id, name string
			var desc, nodes, edges, updatedAt sql.NullString
			var isTemplate int
			var createdAt string
			if err := rows.Scan(&id, &name, &desc, &nodes, &edges, &isTemplate, &createdAt, &updatedAt); err == nil {
				var nodesObj, edgesObj interface{}
				if nodes.Valid && nodes.String != "" {
					_ = json.Unmarshal([]byte(nodes.String), &nodesObj)
				}
				if edges.Valid && edges.String != "" {
					_ = json.Unmarshal([]byte(edges.String), &edgesObj)
				}
				if nodesObj == nil {
					nodesObj = []interface{}{}
				}
				if edgesObj == nil {
					edgesObj = []interface{}{}
				}
				list = append(list, map[string]interface{}{
					"id":          id,
					"name":        name,
					"description": desc.String,
					"nodes":       nodesObj,
					"edges":       edgesObj,
					"is_template": isTemplate,
					"created_at":  createdAt,
					"updated_at":  updatedAt.String,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	r.Get("/providers/list", func(w http.ResponseWriter, r *http.Request) {
		strField := func(title, desc string) map[string]interface{} {
			return map[string]interface{}{"type": "string", "title": title, "description": desc}
		}
		providers := []map[string]interface{}{
			{"id": "bash", "name": "Bash 脚本执行器", "type": "script", "configSchema": map[string]interface{}{"properties": map[string]interface{}{
				"command": strField("命令", "在平台主机执行，支持 {{params.x}}/{{alert.x}} 模板（自动 shell 转义）"),
			}}},
			{"id": "ssh", "name": "SSH 远程命令", "type": "remote", "configSchema": map[string]interface{}{"properties": map[string]interface{}{
				"server_id": strField("服务器 ID", "已注册服务器 ID（优先，复用凭证库）；留空则用 host 直连"),
				"host":      strField("主机", "直连模式主机地址"),
				"port":      strField("端口", "默认 22"),
				"username":  strField("用户名", "直连模式用户名"),
				"password":  strField("密码", "直连模式密码"),
				"command":   strField("命令", "支持 {{params.x}}/{{alert.x}} 模板（自动 shell 转义）"),
			}}},
			{"id": "http", "name": "HTTP / Webhook 请求", "type": "api", "configSchema": map[string]interface{}{"properties": map[string]interface{}{
				"url":    strField("URL", "支持 {{params.x}}/{{alert.x}} 模板"),
				"method": map[string]interface{}{"type": "string", "title": "方法", "enum": []string{"GET", "POST", "PUT", "DELETE"}, "default": "POST"},
				"body":   strField("请求体", "默认 Content-Type: application/json"),
			}}},
			{"id": "snmp", "name": "SNMP 采集与查询", "type": "snmp", "configSchema": map[string]interface{}{"properties": map[string]interface{}{
				"host":      strField("主机", "目标设备地址"),
				"port":      strField("端口", "默认 161"),
				"community": strField("Community", "默认 public"),
				"oid":       strField("OID", "例如 1.3.6.1.2.1.1.1.0"),
			}}},
			{"id": "python", "name": "Python 自动化执行器", "type": "script", "configSchema": map[string]interface{}{"properties": map[string]interface{}{
				"script": strField("脚本", "python3 执行，模板值自动按 Python 字符串转义"),
			}}},
			{"id": "notify", "name": "多渠道通知", "type": "notification", "configSchema": map[string]interface{}{"properties": map[string]interface{}{
				"channel":     map[string]interface{}{"type": "string", "title": "渠道", "enum": []string{"webhook", "wecom", "dingtalk"}, "default": "webhook"},
				"title":       strField("标题", "默认：工作流通知"),
				"message":     strField("内容", "支持 {{params.x}}/{{alert.x}} 模板"),
				"webhook_url": strField("Webhook 地址", "留空则使用系统通知配置"),
			}}},
		}
		httpx.Respond(w, providers)
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		db := appdb.DB()
		var name string
		var desc, nodes, edges, updatedAt sql.NullString
		var isTemplate int
		var createdAt string
		err := db.QueryRow(`SELECT id, name, description, nodes, edges, is_template, created_at, updated_at FROM workflows WHERE id = ?`, id).
			Scan(&id, &name, &desc, &nodes, &edges, &isTemplate, &createdAt, &updatedAt)
		if err != nil {
			httpx.RespondError(w, "工作流不存在", http.StatusNotFound)
			return
		}
		var nodesObj, edgesObj interface{}
		if nodes.Valid && nodes.String != "" {
			_ = json.Unmarshal([]byte(nodes.String), &nodesObj)
		}
		if edges.Valid && edges.String != "" {
			_ = json.Unmarshal([]byte(edges.String), &edgesObj)
		}
		if nodesObj == nil {
			nodesObj = []interface{}{}
		}
		if edgesObj == nil {
			edgesObj = []interface{}{}
		}
		httpx.Respond(w, map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": desc.String,
			"nodes":       nodesObj,
			"edges":       edgesObj,
			"is_template": isTemplate,
			"created_at":  createdAt,
			"updated_at":  updatedAt.String,
		})
	})

	r.With(middleware.RequireRole("admin")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        string      `json:"name"`
			Description string      `json:"description"`
			Nodes       interface{} `json:"nodes"`
			Edges       interface{} `json:"edges"`
			IsTemplate  int         `json:"is_template"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "无效的请求格式", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			httpx.RespondError(w, "工作流名称不能为空", http.StatusBadRequest)
			return
		}
		nodesJSON, _ := json.Marshal(req.Nodes)
		edgesJSON, _ := json.Marshal(req.Edges)
		id := uuid.NewString()
		now := time.Now().Format("2006-01-02 15:04:05")

		_, err := appdb.DB().Exec(`
			INSERT INTO workflows (id, name, description, nodes, edges, is_template, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, req.Name, req.Description, string(nodesJSON), string(edgesJSON), req.IsTemplate, now, now)
		if err != nil {
			httpx.RespondError(w, "创建工作流失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, map[string]interface{}{
			"id":          id,
			"name":        req.Name,
			"description": req.Description,
			"nodes":       req.Nodes,
			"edges":       req.Edges,
			"is_template": req.IsTemplate,
			"created_at":  now,
		})
	})

	r.With(middleware.RequireRole("admin")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			Name        *string     `json:"name"`
			Description *string     `json:"description"`
			Nodes       interface{} `json:"nodes"`
			Edges       interface{} `json:"edges"`
			IsTemplate  *int        `json:"is_template"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "无效的请求格式", http.StatusBadRequest)
			return
		}
		now := time.Now().Format("2006-01-02 15:04:05")
		nodesJSON, _ := json.Marshal(req.Nodes)
		edgesJSON, _ := json.Marshal(req.Edges)

		_, err := appdb.DB().Exec(`
			UPDATE workflows SET 
				name = COALESCE(?, name),
				description = COALESCE(?, description),
				nodes = CASE WHEN ? != 'null' THEN ? ELSE nodes END,
				edges = CASE WHEN ? != 'null' THEN ? ELSE edges END,
				is_template = COALESCE(?, is_template),
				updated_at = ?
			WHERE id = ?`,
			req.Name, req.Description, string(nodesJSON), string(nodesJSON), string(edgesJSON), string(edgesJSON), req.IsTemplate, now, id)
		if err != nil {
			httpx.RespondError(w, "更新工作流失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{"id": id, "updated_at": now})
	})

	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, err := appdb.DB().Exec(`DELETE FROM workflows WHERE id = ?`, id)
		if err != nil {
			httpx.RespondError(w, "删除工作流失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "工作流已删除")
	})

	return r
}

// TaskRoutes 任务执行路由 (/api/v1/tasks)
func TaskRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		db := appdb.DB()
		rows, err := db.Query(`
			SELECT t.id, t.name, t.workflow_id, t.status, t.start_time, t.end_time, t.current_node_id, t.node_results, t.logs, t.created_at, COALESCE(w.name, '') as workflow_name
			FROM tasks t LEFT JOIN workflows w ON t.workflow_id = w.id
			ORDER BY t.created_at DESC LIMIT 100`)
		if err != nil {
			httpx.RespondError(w, "获取任务列表失败", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var list []map[string]interface{}
		for rows.Next() {
			var id, name, status string
			var workflowID, startTime, endTime, currentNodeID, nodeResults, logs, createdAt, workflowName sql.NullString
			if err := rows.Scan(&id, &name, &workflowID, &status, &startTime, &endTime, &currentNodeID, &nodeResults, &logs, &createdAt, &workflowName); err == nil {
				var nodeResultsObj, logsObj interface{}
				if nodeResults.Valid && nodeResults.String != "" {
					_ = json.Unmarshal([]byte(nodeResults.String), &nodeResultsObj)
				}
				if logs.Valid && logs.String != "" {
					_ = json.Unmarshal([]byte(logs.String), &logsObj)
				}
				if nodeResultsObj == nil {
					nodeResultsObj = map[string]interface{}{}
				}
				if logsObj == nil {
					logsObj = []interface{}{}
				}
				list = append(list, map[string]interface{}{
					"id":              id,
					"name":            name,
					"workflow_id":     workflowID.String,
					"workflow_name":   workflowName.String,
					"status":          status,
					"start_time":      startTime.String,
					"end_time":        endTime.String,
					"current_node_id": currentNodeID.String,
					"node_results":    nodeResultsObj,
					"logs":            logsObj,
					"created_at":      createdAt.String,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	r.With(middleware.RequireRole("admin", "operator")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WorkflowID string                 `json:"workflow_id"`
			Name       string                 `json:"name"`
			Params     map[string]interface{} `json:"params"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		if req.WorkflowID == "" {
			httpx.RespondError(w, "workflow_id 必填", http.StatusBadRequest)
			return
		}
		var wfName string
		if appdb.DB().QueryRow(`SELECT name FROM workflows WHERE id = ?`, req.WorkflowID).Scan(&wfName) != nil {
			httpx.RespondError(w, "工作流不存在", http.StatusNotFound)
			return
		}
		id := uuid.NewString()
		now := time.Now().Format("2006-01-02 15:04:05")
		name := req.Name
		if name == "" {
			name = wfName + "-" + time.Now().Format("150405")
		}
		if req.Params == nil {
			req.Params = map[string]interface{}{}
		}
		ctxJSON, _ := json.Marshal(map[string]interface{}{"params": req.Params})

		_, err := appdb.DB().Exec(`
			INSERT INTO tasks (id, name, workflow_id, status, start_time, context, created_at, updated_at)
			VALUES (?, ?, ?, 'running', ?, ?, ?, ?)`,
			id, name, req.WorkflowID, now, string(ctxJSON), now, now)
		if err != nil {
			httpx.RespondError(w, "创建任务失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		StartTaskExecution(id)
		httpx.RespondStatus(w, http.StatusCreated, map[string]interface{}{
			"id":          id,
			"name":        name,
			"workflow_id": req.WorkflowID,
			"status":      "running",
			"start_time":  now,
			"created_at":  now,
		})
	})

	r.With(middleware.RequireRole("admin", "operator")).Put("/{id}/pause", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		res, err := appdb.DB().Exec(`UPDATE tasks SET status = 'paused', updated_at = ? WHERE id = ? AND status = 'running'`,
			time.Now().Format("2006-01-02 15:04:05"), id)
		if err != nil || affected(res) == 0 {
			httpx.RespondError(w, "只有执行中的任务可以暂停", http.StatusConflict)
			return
		}
		httpx.RespondMessage(w, "任务将在当前节点完成后暂停")
	})

	r.With(middleware.RequireRole("admin", "operator")).Put("/{id}/resume", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		res, err := appdb.DB().Exec(`UPDATE tasks SET status = 'running', updated_at = ? WHERE id = ? AND status = 'paused'`,
			time.Now().Format("2006-01-02 15:04:05"), id)
		if err != nil || affected(res) == 0 {
			httpx.RespondError(w, "只有已暂停的任务可以恢复", http.StatusConflict)
			return
		}
		StartTaskExecution(id)
		httpx.RespondMessage(w, "任务已恢复")
	})

	r.With(middleware.RequireRole("admin", "operator")).Put("/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		now := time.Now().Format("2006-01-02 15:04:05")
		res, err := appdb.DB().Exec(`UPDATE tasks SET status = 'cancelled', end_time = ?, updated_at = ? WHERE id = ? AND status IN ('running','paused','pending','waiting_approval')`, now, now, id)
		if err != nil || affected(res) == 0 {
			httpx.RespondError(w, "任务当前状态不可取消", http.StatusConflict)
			return
		}
		appdb.DB().Exec(`UPDATE approval_requests SET status = 'rejected', reject_reason = '任务已取消', updated_at = ? WHERE task_id = ? AND status = 'pending'`, now, id)
		httpx.RespondMessage(w, "任务已取消")
	})

	return r
}

func affected(res sql.Result) int64 {
	n, _ := res.RowsAffected()
	return n
}

// ScheduledTaskRoutes 定时任务路由 (/api/v1/scheduled-tasks)
func ScheduledTaskRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		db := appdb.DB()
		rows, err := db.Query(`
			SELECT s.id, s.name, s.description, s.workflow_id, s.cron_expression, s.enabled, s.last_run_at, s.next_run_at, s.created_at, s.updated_at, COALESCE(w.name, '') as workflow_name
			FROM scheduled_tasks s LEFT JOIN workflows w ON s.workflow_id = w.id
			ORDER BY s.created_at DESC`)
		if err != nil {
			httpx.RespondError(w, "获取定时任务失败", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var list []map[string]interface{}
		for rows.Next() {
			var id, name, cronExpr string
			var desc, workflowID, lastRunAt, nextRunAt, createdAt, updatedAt, workflowName sql.NullString
			var enabled int
			if err := rows.Scan(&id, &name, &desc, &workflowID, &cronExpr, &enabled, &lastRunAt, &nextRunAt, &createdAt, &updatedAt, &workflowName); err == nil {
				list = append(list, map[string]interface{}{
					"id":              id,
					"name":            name,
					"description":     desc.String,
					"workflow_id":     workflowID.String,
					"workflow_name":   workflowName.String,
					"cron_expression": cronExpr,
					"enabled":         enabled,
					"last_run_at":     lastRunAt.String,
					"next_run_at":     nextRunAt.String,
					"created_at":      createdAt.String,
					"updated_at":      updatedAt.String,
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
			Name           string `json:"name"`
			Description    string `json:"description"`
			WorkflowID     string `json:"workflow_id"`
			CronExpression string `json:"cron_expression"`
			Enabled        *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		enabledInt := 1
		if req.Enabled != nil && !*req.Enabled {
			enabledInt = 0
		}
		id := uuid.NewString()
		now := time.Now().Format("2006-01-02 15:04:05")

		_, err := appdb.DB().Exec(`
			INSERT INTO scheduled_tasks (id, name, description, workflow_id, cron_expression, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, req.Name, req.Description, req.WorkflowID, req.CronExpression, enabledInt, now, now)
		if err != nil {
			httpx.RespondError(w, "创建定时任务失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, map[string]interface{}{"id": id, "name": req.Name})
	})

	r.With(middleware.RequireRole("admin")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			Name           *string `json:"name"`
			Description    *string `json:"description"`
			WorkflowID     *string `json:"workflow_id"`
			CronExpression *string `json:"cron_expression"`
			Enabled        *bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		var enabledVal *int
		if req.Enabled != nil {
			v := 0
			if *req.Enabled {
				v = 1
			}
			enabledVal = &v
		}
		now := time.Now().Format("2006-01-02 15:04:05")
		_, err := appdb.DB().Exec(`
			UPDATE scheduled_tasks SET
				name = COALESCE(?, name),
				description = COALESCE(?, description),
				workflow_id = COALESCE(?, workflow_id),
				cron_expression = COALESCE(?, cron_expression),
				enabled = COALESCE(?, enabled),
				updated_at = ?
			WHERE id = ?`,
			req.Name, req.Description, req.WorkflowID, req.CronExpression, enabledVal, now, id)
		if err != nil {
			httpx.RespondError(w, "更新定时任务失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{"id": id, "updated_at": now})
	})

	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, _ = appdb.DB().Exec(`DELETE FROM scheduled_tasks WHERE id = ?`, id)
		httpx.RespondMessage(w, "定时任务已删除")
	})

	return r
}

