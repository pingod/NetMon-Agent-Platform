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
		providers := []map[string]interface{}{
			{"id": "bash", "name": "Bash 脚本执行器", "type": "script", "configSchema": map[string]interface{}{"command": "string"}},
			{"id": "ssh", "name": "SSH 远程命令", "type": "remote", "configSchema": map[string]interface{}{"host": "string", "port": 22, "command": "string"}},
			{"id": "http", "name": "HTTP / Webhook 请求", "type": "api", "configSchema": map[string]interface{}{"url": "string", "method": "POST"}},
			{"id": "snmp", "name": "SNMP 采集与查询", "type": "snmp", "configSchema": map[string]interface{}{"oid": "string", "version": "v2c"}},
			{"id": "python", "name": "Python 自动化执行器", "type": "script", "configSchema": map[string]interface{}{"script": "string"}},
			{"id": "notify", "name": "多渠道通知", "type": "notification", "configSchema": map[string]interface{}{"channel": "wecom", "message": "string"}},
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

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WorkflowID string `json:"workflow_id"`
			Name       string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		id := uuid.NewString()
		now := time.Now().Format("2006-01-02 15:04:05")
		name := req.Name
		if name == "" {
			name = "任务-" + time.Now().Format("150405")
		}

		_, err := appdb.DB().Exec(`
			INSERT INTO tasks (id, name, workflow_id, status, start_time, created_at)
			VALUES (?, ?, ?, 'completed', ?, ?)`,
			id, name, req.WorkflowID, now, now)
		if err != nil {
			httpx.RespondError(w, "创建任务失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, map[string]interface{}{
			"id":          id,
			"name":        name,
			"workflow_id": req.WorkflowID,
			"status":      "completed",
			"start_time":  now,
			"created_at":  now,
		})
	})

	r.Put("/{id}/pause", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, _ = appdb.DB().Exec(`UPDATE tasks SET status = 'paused' WHERE id = ?`, id)
		httpx.RespondMessage(w, "任务已暂停")
	})

	r.Put("/{id}/resume", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, _ = appdb.DB().Exec(`UPDATE tasks SET status = 'running' WHERE id = ?`, id)
		httpx.RespondMessage(w, "任务已恢复")
	})

	r.Put("/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, _ = appdb.DB().Exec(`UPDATE tasks SET status = 'cancelled' WHERE id = ?`, id)
		httpx.RespondMessage(w, "任务已取消")
	})

	return r
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

