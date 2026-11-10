// 巡检模版管理 — CRUD API 路由
// 挂载路径：/api/v1/inspection-templates
package inspection

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
)

// Routes 巡检模版路由
func Routes() chi.Router {
	r := chi.NewRouter()

	// GET / — 列表（支持筛选）
	r.Get("/", listTemplates)

	// GET /vendors — 获取所有厂商列表
	r.Get("/vendors", listVendors)

	// POST / — 创建模版
	r.With(middleware.RequireRole("admin")).Post("/", createTemplate)

	// GET /{id} — 获取单个模版
	r.Get("/{id}", getTemplate)

	// PUT /{id} — 更新模版
	r.With(middleware.RequireRole("admin")).Put("/{id}", updateTemplate)

	// DELETE /{id} — 删除模版
	r.With(middleware.RequireRole("admin")).Delete("/{id}", deleteTemplate)

	// POST /{id}/clone — 克隆模版
	r.With(middleware.RequireRole("admin")).Post("/{id}/clone", cloneTemplate)

	return r
}

// templateRow 数据库行映射
type templateRow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Vendor      string `json:"vendor"`
	Category    string `json:"category"`
	DeviceType  string `json:"deviceType"`
	OSType      string `json:"osType"`
	Description string `json:"description"`
	Commands    string `json:"-"` // 原始 JSON，下面解析
	Variables   string `json:"-"`
	Tags        string `json:"-"`
	IsPreset    int    `json:"isPreset"`
	Enabled     int    `json:"enabled"`
	UsageCount  int    `json:"usageCount"`
	CreatedBy   string `json:"createdBy"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// templateResponse 前端响应结构
type templateResponse struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Vendor      string          `json:"vendor"`
	Category    string          `json:"category"`
	DeviceType  string          `json:"deviceType"`
	OSType      string          `json:"osType"`
	Description string          `json:"description"`
	Commands    json.RawMessage `json:"commands"`
	Variables   json.RawMessage `json:"variables"`
	Tags        json.RawMessage `json:"tags"`
	IsPreset    bool            `json:"isPreset"`
	Enabled     bool            `json:"enabled"`
	UsageCount  int             `json:"usageCount"`
	CreatedBy   string          `json:"createdBy"`
	CreatedAt   string          `json:"createdAt"`
	UpdatedAt   string          `json:"updatedAt"`
}

func rowToResponse(row templateRow) templateResponse {
	cmds := json.RawMessage(row.Commands)
	if row.Commands == "" {
		cmds = json.RawMessage("[]")
	}
	vars := json.RawMessage(row.Variables)
	if row.Variables == "" {
		vars = json.RawMessage("[]")
	}
	tags := json.RawMessage(row.Tags)
	if row.Tags == "" {
		tags = json.RawMessage("[]")
	}
	return templateResponse{
		ID:          row.ID,
		Name:        row.Name,
		Vendor:      row.Vendor,
		Category:    row.Category,
		DeviceType:  row.DeviceType,
		OSType:      row.OSType,
		Description: row.Description,
		Commands:    cmds,
		Variables:   vars,
		Tags:        tags,
		IsPreset:    row.IsPreset == 1,
		Enabled:     row.Enabled == 1,
		UsageCount:  row.UsageCount,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

// listTemplates GET / — 模版列表
func listTemplates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	category := q.Get("category")
	vendor := q.Get("vendor")
	search := q.Get("search")

	query := "SELECT id, name, vendor, category, device_type, os_type, description, commands, variables, tags, is_preset, enabled, usage_count, created_by, created_at, updated_at FROM inspection_templates WHERE 1=1"
	var args []interface{}
	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}
	if vendor != "" {
		query += " AND vendor = ?"
		args = append(args, vendor)
	}
	if search != "" {
		query += " AND (name LIKE ? OR description LIKE ?)"
		args = append(args, "%"+search+"%", "%"+search+"%")
	}
	query += " ORDER BY is_preset DESC, updated_at DESC"

	rows, err := appdb.DB().Query(query, args...)
	if err != nil {
		httpx.RespondError(w, "查询失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var list []templateResponse
	for rows.Next() {
		var row templateRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Vendor, &row.Category, &row.DeviceType, &row.OSType, &row.Description, &row.Commands, &row.Variables, &row.Tags, &row.IsPreset, &row.Enabled, &row.UsageCount, &row.CreatedBy, &row.CreatedAt, &row.UpdatedAt); err != nil {
			continue
		}
		list = append(list, rowToResponse(row))
	}
	if list == nil {
		list = []templateResponse{}
	}
	httpx.Respond(w, list)
}

// listVendors GET /vendors — 获取厂商列表
func listVendors(w http.ResponseWriter, r *http.Request) {
	vendors := []map[string]string{
		{"value": "huawei", "label": "华为"},
		{"value": "cisco", "label": "思科"},
		{"value": "h3c", "label": "华三"},
		{"value": "ruijie", "label": "锐捷"},
		{"value": "zte", "label": "中兴"},
		{"value": "sangfor", "label": "深信服"},
		{"value": "fortinet", "label": "飞塔"},
		{"value": "juniper", "label": "瞻博"},
		{"value": "paloalto", "label": "PaloAlto"},
		{"value": "hpe", "label": "HPE"},
		{"value": "other", "label": "通用/其他"},
	}
	httpx.Respond(w, vendors)
}

// getTemplate GET /{id} — 获取单个模版
func getTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var row templateRow
	err := appdb.DB().QueryRow("SELECT id, name, vendor, category, device_type, os_type, description, commands, variables, tags, is_preset, enabled, usage_count, created_by, created_at, updated_at FROM inspection_templates WHERE id = ?", id).
		Scan(&row.ID, &row.Name, &row.Vendor, &row.Category, &row.DeviceType, &row.OSType, &row.Description, &row.Commands, &row.Variables, &row.Tags, &row.IsPreset, &row.Enabled, &row.UsageCount, &row.CreatedBy, &row.CreatedAt, &row.UpdatedAt)
	if err == sql.ErrNoRows {
		httpx.RespondError(w, "模版不存在", http.StatusNotFound)
		return
	}
	if err != nil {
		httpx.RespondError(w, "查询失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, rowToResponse(row))
}

// createTemplate POST / — 创建模版
func createTemplate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string          `json:"name"`
		Vendor      string          `json:"vendor"`
		Category    string          `json:"category"`
		DeviceType  string          `json:"deviceType"`
		OSType      string          `json:"osType"`
		Description string          `json:"description"`
		Commands    json.RawMessage `json:"commands"`
		Variables   json.RawMessage `json:"variables"`
		Tags        json.RawMessage `json:"tags"`
		Enabled     *bool           `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.RespondError(w, "无效的请求体", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		httpx.RespondError(w, "名称不能为空", http.StatusBadRequest)
		return
	}

	if req.Vendor == "" {
		req.Vendor = "other"
	}
	if req.Category == "" {
		req.Category = "network"
	}
	if req.DeviceType == "" {
		req.DeviceType = "unknown"
	}
	if req.OSType == "" {
		req.OSType = "linux"
	}
	if len(req.Commands) == 0 {
		req.Commands = json.RawMessage("[]")
	}
	if len(req.Variables) == 0 {
		req.Variables = json.RawMessage("[]")
	}
	if len(req.Tags) == 0 {
		req.Tags = json.RawMessage("[]")
	}
	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}

	createdBy := "admin"
	if u := middleware.GetUser(r); u != nil && u.Username != "" {
		createdBy = u.Username
	}

	id := "tpl-" + uuid.NewString()[:12]
	_, err := appdb.DB().Exec(`INSERT INTO inspection_templates (id, name, vendor, category, device_type, os_type, description, commands, variables, tags, is_preset, enabled, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		id, req.Name, req.Vendor, req.Category, req.DeviceType, req.OSType, req.Description, string(req.Commands), string(req.Variables), string(req.Tags), enabled, createdBy)
	if err != nil {
		httpx.RespondError(w, "创建失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	httpx.Respond(w, map[string]string{"id": id, "message": "创建成功"})
}

// updateTemplate PUT /{id} — 更新模版
func updateTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name        *string         `json:"name"`
		Vendor      *string         `json:"vendor"`
		Category    *string         `json:"category"`
		DeviceType  *string         `json:"deviceType"`
		OSType      *string         `json:"osType"`
		Description *string         `json:"description"`
		Commands    json.RawMessage `json:"commands"`
		Variables   json.RawMessage `json:"variables"`
		Tags        json.RawMessage `json:"tags"`
		Enabled     *bool           `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.RespondError(w, "无效的请求体", http.StatusBadRequest)
		return
	}

	// 构建动态 SET：字段为 nil 表示未传，空串是合法更新值
	updates := ""
	args := []interface{}{}
	if req.Name != nil {
		if *req.Name == "" {
			httpx.RespondError(w, "名称不能为空", http.StatusBadRequest)
			return
		}
		updates += "name = ?, "
		args = append(args, *req.Name)
	}
	if req.Vendor != nil {
		updates += "vendor = ?, "
		args = append(args, *req.Vendor)
	}
	if req.Category != nil {
		updates += "category = ?, "
		args = append(args, *req.Category)
	}
	if req.DeviceType != nil {
		updates += "device_type = ?, "
		args = append(args, *req.DeviceType)
	}
	if req.OSType != nil {
		updates += "os_type = ?, "
		args = append(args, *req.OSType)
	}
	if req.Description != nil {
		updates += "description = ?, "
		args = append(args, *req.Description)
	}
	if len(req.Commands) > 0 {
		updates += "commands = ?, "
		args = append(args, string(req.Commands))
	}
	if len(req.Variables) > 0 {
		updates += "variables = ?, "
		args = append(args, string(req.Variables))
	}
	if len(req.Tags) > 0 {
		updates += "tags = ?, "
		args = append(args, string(req.Tags))
	}
	if req.Enabled != nil {
		enabled := 0
		if *req.Enabled {
			enabled = 1
		}
		updates += "enabled = ?, "
		args = append(args, enabled)
	}
	if updates == "" {
		httpx.RespondError(w, "无更新内容", http.StatusBadRequest)
		return
	}
	updates += "updated_at = datetime('now','localtime')"
	args = append(args, id)

	_, err := appdb.DB().Exec("UPDATE inspection_templates SET "+updates+" WHERE id = ?", args...)
	if err != nil {
		httpx.RespondError(w, "更新失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	httpx.RespondMessage(w, "更新成功")
}

// deleteTemplate DELETE /{id} — 删除模版
func deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// 预置模版不允许删除
	var isPreset int
	_ = appdb.DB().QueryRow("SELECT is_preset FROM inspection_templates WHERE id = ?", id).Scan(&isPreset)
	if isPreset == 1 {
		httpx.RespondError(w, "预置模版不可删除，可禁用或克隆后修改", http.StatusBadRequest)
		return
	}

	_, err := appdb.DB().Exec("DELETE FROM inspection_templates WHERE id = ?", id)
	if err != nil {
		httpx.RespondError(w, "删除失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.RespondMessage(w, "删除成功")
}

// cloneTemplate POST /{id}/clone — 克隆模版
func cloneTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var row templateRow
	err := appdb.DB().QueryRow("SELECT id, name, vendor, category, device_type, os_type, description, commands, variables, tags, is_preset, enabled, usage_count, created_by, created_at, updated_at FROM inspection_templates WHERE id = ?", id).
		Scan(&row.ID, &row.Name, &row.Vendor, &row.Category, &row.DeviceType, &row.OSType, &row.Description, &row.Commands, &row.Variables, &row.Tags, &row.IsPreset, &row.Enabled, &row.UsageCount, &row.CreatedBy, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		httpx.RespondError(w, "模版不存在", http.StatusNotFound)
		return
	}

	newID := "tpl-" + uuid.NewString()[:12]
	newName := row.Name + " (副本)"
	createdBy := "admin"
	if u := middleware.GetUser(r); u != nil && u.Username != "" {
		createdBy = u.Username
	}

	_, err = appdb.DB().Exec(`INSERT INTO inspection_templates (id, name, vendor, category, device_type, os_type, description, commands, variables, tags, is_preset, enabled, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 1, ?)`,
		newID, newName, row.Vendor, row.Category, row.DeviceType, row.OSType, row.Description, row.Commands, row.Variables, row.Tags, createdBy)
	if err != nil {
		httpx.RespondError(w, "克隆失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	httpx.Respond(w, map[string]string{"id": newID, "message": "克隆成功"})
}
