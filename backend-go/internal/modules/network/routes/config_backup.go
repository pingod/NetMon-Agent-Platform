// 配置备份路由 — 16 个端点
// 等价于 TS routes/configBackupRoutes.ts
package routes

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

// ConfigBackupRoutes 配置备份路由
func ConfigBackupRoutes() chi.Router {
	r := chi.NewRouter()

	// GET / — 配置备份概览
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		var tplCount, taskCount, recCount int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM config_backup_templates`).Scan(&tplCount)
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM config_backup_tasks`).Scan(&taskCount)
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM config_backup_records`).Scan(&recCount)
		httpx.Respond(w, map[string]interface{}{
			"templates_count": tplCount,
			"tasks_count":     taskCount,
			"records_count":   recCount,
		})
	})

	// GET /history — 备份历史（别名指向 records）
	r.Get("/history", func(w http.ResponseWriter, r *http.Request) {
		records, err := services.ConfigBackupServiceInstance.ListRecords("", "", "", 50, 0)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		httpx.Respond(w, records)
	})

	// GET /templates — 获取模板列表
	r.Get("/templates", func(w http.ResponseWriter, r *http.Request) {
		templates, err := services.ConfigBackupServiceInstance.ListTemplates()
		if err != nil {
			httpx.RespondError(w, "获取备份模板列表失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, templates)
	})

	// GET /templates/:id — 获取单个模板
	r.Get("/templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		template, err := services.ConfigBackupServiceInstance.GetTemplate(chi.URLParam(r, "id"))
		if err != nil || template == nil {
			httpx.RespondError(w, "模板不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, template)
	})

	// POST /templates/restore-presets — 恢复默认预置模板
	r.With(middleware.RequireRole("admin")).Post("/templates/restore-presets", func(w http.ResponseWriter, r *http.Request) {
		templates, err := services.RestoreBackupPresets()
		if err != nil {
			httpx.RespondError(w, "恢复预置模板失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, templates)
	})

	// POST /templates — 创建模板
	r.With(middleware.RequireRole("admin")).Post("/templates", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Vendor      string   `json:"vendor"`
			Commands    []string `json:"commands"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Name == "" || req.Vendor == "" || len(req.Commands) == 0 {
			httpx.RespondError(w, "名称、厂商和指令列表为必填项", http.StatusBadRequest)
			return
		}

		template, err := services.ConfigBackupServiceInstance.CreateTemplate(repos.TemplateCreateInput{
			Name:        req.Name,
			Description: req.Description,
			Vendor:      req.Vendor,
			Commands:    req.Commands,
		})
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, template)
	})

	// PUT /templates/:id — 更新模板
	r.With(middleware.RequireRole("admin")).Put("/templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        *string   `json:"name"`
			Description *string   `json:"description"`
			Commands    *[]string `json:"commands"`
			Vendor      *string   `json:"vendor"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		input := repos.TemplateUpdateInput{
			Name:        req.Name,
			Description: req.Description,
			Commands:    req.Commands,
			Vendor:      req.Vendor,
		}

		template, err := services.ConfigBackupServiceInstance.UpdateTemplate(chi.URLParam(r, "id"), input)
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if template == nil {
			httpx.RespondError(w, "模板不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, template)
	})

	// DELETE /templates/:id — 删除模板
	r.With(middleware.RequireRole("admin")).Delete("/templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		err := services.ConfigBackupServiceInstance.DeleteTemplate(chi.URLParam(r, "id"))
		if err != nil {
			if contains(err.Error(), "正在被备份任务使用") {
				httpx.RespondError(w, err.Error(), http.StatusConflict)
				return
			}
			httpx.RespondError(w, "删除备份模板失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "模板已删除")
	})

	// GET /tasks — 获取任务列表
	r.Get("/tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks, err := services.ConfigBackupServiceInstance.ListTasks()
		if err != nil {
			httpx.RespondError(w, "获取备份任务列表失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, tasks)
	})

	// GET /tasks/:id — 获取单个任务
	r.Get("/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		task, err := services.ConfigBackupServiceInstance.GetTask(chi.URLParam(r, "id"))
		if err != nil || task == nil {
			httpx.RespondError(w, "任务不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, task)
	})

	// POST /tasks — 创建任务
	r.With(middleware.RequireRole("admin")).Post("/tasks", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name          string   `json:"name"`
			TemplateID    string   `json:"template_id"`
			ScheduleType  string   `json:"schedule_type"`
			ScheduleCron  string   `json:"schedule_cron"`
			DeviceIDs     []string `json:"device_ids"`
			Enabled       bool     `json:"enabled"`
			RetentionDays int      `json:"retention_days"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Name == "" || req.TemplateID == "" || req.ScheduleType == "" || len(req.DeviceIDs) == 0 {
			httpx.RespondError(w, "名称、模板、调度类型和设备列表为必填项", http.StatusBadRequest)
			return
		}
		validTypes := map[string]bool{"manual": true, "daily": true, "weekly": true, "monthly": true, "cron": true}
		if !validTypes[req.ScheduleType] {
			httpx.RespondError(w, "无效的调度类型", http.StatusBadRequest)
			return
		}
		if req.ScheduleType == "cron" && req.ScheduleCron == "" {
			httpx.RespondError(w, "cron 类型必须提供 cron 表达式", http.StatusBadRequest)
			return
		}

		task, err := services.ConfigBackupServiceInstance.CreateTask(repos.TaskCreateInput{
			Name:          req.Name,
			TemplateID:    req.TemplateID,
			ScheduleType:  req.ScheduleType,
			ScheduleCron:  req.ScheduleCron,
			DeviceIDs:     req.DeviceIDs,
			Enabled:       req.Enabled,
			RetentionDays: req.RetentionDays,
		})
		if err != nil {
			if contains(err.Error(), "不存在") {
				httpx.RespondError(w, err.Error(), http.StatusBadRequest)
				return
			}
			httpx.RespondError(w, "创建备份任务失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, task)
	})

	// PUT /tasks/:id — 更新任务
	r.With(middleware.RequireRole("admin")).Put("/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name          *string   `json:"name"`
			TemplateID    *string   `json:"template_id"`
			ScheduleType  *string   `json:"schedule_type"`
			ScheduleCron  *string   `json:"schedule_cron"`
			DeviceIDs     *[]string `json:"device_ids"`
			Enabled       *bool     `json:"enabled"`
			RetentionDays *int      `json:"retention_days"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		input := repos.TaskUpdateInput{
			Name:          req.Name,
			TemplateID:    req.TemplateID,
			ScheduleType:  req.ScheduleType,
			ScheduleCron:  req.ScheduleCron,
			DeviceIDs:     req.DeviceIDs,
			Enabled:       req.Enabled,
			RetentionDays:  req.RetentionDays,
		}

		task, err := services.ConfigBackupServiceInstance.UpdateTask(chi.URLParam(r, "id"), input)
		if err != nil {
			httpx.RespondError(w, "更新备份任务失败", http.StatusInternalServerError)
			return
		}
		if task == nil {
			httpx.RespondError(w, "任务不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, task)
	})

	// DELETE /tasks/:id — 删除任务
	r.With(middleware.RequireRole("admin")).Delete("/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		err := services.ConfigBackupServiceInstance.DeleteTask(chi.URLParam(r, "id"))
		if err != nil {
			httpx.RespondError(w, "删除备份任务失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "任务及关联记录已删除")
	})

	// POST /tasks/:id/toggle — 启用/禁用任务
	r.With(middleware.RequireRole("admin")).Post("/tasks/{id}/toggle", func(w http.ResponseWriter, r *http.Request) {
		task, err := services.ConfigBackupServiceInstance.ToggleTask(chi.URLParam(r, "id"))
		if err != nil {
			httpx.RespondError(w, "切换备份任务状态失败", http.StatusInternalServerError)
			return
		}
		if task == nil {
			httpx.RespondError(w, "任务不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, task)
	})

	// POST /tasks/:id/execute — 手动执行任务（异步）
	r.With(middleware.RequireRole("admin")).Post("/tasks/{id}/execute", func(w http.ResponseWriter, r *http.Request) {
		progress, err := services.ConfigBackupServiceInstance.ExecuteTask(chi.URLParam(r, "id"))
		if err != nil {
			if contains(err.Error(), "不存在") {
				httpx.RespondError(w, err.Error(), http.StatusNotFound)
				return
			}
			httpx.RespondError(w, "执行备份任务失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, progress)
	})

	// POST /batch-execute — 批量备份（异步）
	r.With(middleware.RequireRole("admin")).Post("/batch-execute", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DeviceIDs  []string `json:"device_ids"`
			TemplateID string   `json:"template_id"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if len(req.DeviceIDs) == 0 {
			httpx.RespondError(w, "请选择至少一台设备", http.StatusBadRequest)
			return
		}
		if req.TemplateID == "" {
			httpx.RespondError(w, "请选择备份模板", http.StatusBadRequest)
			return
		}

		progress, err := services.ConfigBackupServiceInstance.BatchBackup(req.DeviceIDs, req.TemplateID)
		if err != nil {
			if contains(err.Error(), "不存在") {
				httpx.RespondError(w, err.Error(), http.StatusBadRequest)
				return
			}
			httpx.RespondError(w, "批量备份失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, progress)
	})

	// GET /records — 获取备份记录列表
	r.Get("/records", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 {
			limit = 100
		}
		offset, _ := strconv.Atoi(q.Get("offset"))

		records, err := services.ConfigBackupServiceInstance.ListRecords(
			q.Get("task_id"), q.Get("device_id"), q.Get("status"), limit, offset,
		)
		if err != nil {
			httpx.RespondError(w, "获取备份记录列表失败", http.StatusInternalServerError)
			return
		}
		total, _ := services.ConfigBackupServiceInstance.GetRecordCount(
			q.Get("task_id"), q.Get("device_id"), q.Get("status"),
		)
		httpx.Respond(w, map[string]interface{}{"records": records, "total": total})
	})

	// GET /records/:id — 获取单条记录
	r.Get("/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		record, err := services.ConfigBackupServiceInstance.GetRecord(chi.URLParam(r, "id"))
		if err != nil || record == nil {
			httpx.RespondError(w, "记录不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, record)
	})

	// DELETE /records/:id — 删除记录
	r.With(middleware.RequireRole("admin")).Delete("/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		err := services.ConfigBackupServiceInstance.DeleteRecord(chi.URLParam(r, "id"))
		if err != nil {
			httpx.RespondError(w, "删除备份记录失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "记录已删除")
	})

	// GET /progress/:taskId — 获取备份执行进度
	r.Get("/progress/{taskId}", func(w http.ResponseWriter, r *http.Request) {
		taskID := chi.URLParam(r, "taskId")
		progress := services.GetBackupProgress(taskID)
		httpx.Respond(w, progress)
	})

	return r
}
