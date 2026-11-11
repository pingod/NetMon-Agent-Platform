// Package configmgmt 配置模板中心。
//
// 表由迁移 v22 创建；此前只有前端页面没有 Go 路由，访问 /api/v1/config-templates
// 一直 404。这里补齐 CRUD 与渲染，渲染只做 {{var}} 替换（模板本身不执行）。
package configmgmt

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
)

type Template struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Category    string   `json:"category"`
	ServiceName string   `json:"service_name"`
	Content     string   `json:"content"`
	Variables   []string `json:"variables"`
	OSType      string   `json:"os_type"`
	TargetType  string   `json:"target_type"`
	TargetPath  string   `json:"target_path"`
	Tags        []string `json:"tags"`
	Version     int      `json:"version"`
	IsSystem    int      `json:"is_system"`
	CreatedBy   string   `json:"created_by"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type templateInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Category    string   `json:"category"`
	ServiceName string   `json:"service_name"`
	Content     string   `json:"content"`
	Variables   []string `json:"variables"`
	OSType      string   `json:"os_type"`
	TargetType  string   `json:"target_type"`
	TargetPath  string   `json:"target_path"`
	Tags        []string `json:"tags"`
}

// Routes /api/v1/config-templates
func Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", list)
	r.Get("/{id}", get)
	write := middleware.RequireRole("admin", "operator")
	r.With(write).Post("/", create)
	r.With(write).Put("/{id}", update)
	r.With(write).Delete("/{id}", remove)
	r.With(write).Post("/{id}/render", render)
	return r
}

const templateQuery = `SELECT id, name, COALESCE(description,''), COALESCE(type,'generic'), COALESCE(category,''),
	COALESCE(service_name,''), COALESCE(content,''), COALESCE(variables,'[]'), COALESCE(os_type,'linux'),
	COALESCE(target_type,'server'), COALESCE(target_path,''), COALESCE(tags,'[]'), COALESCE(version,1),
	COALESCE(is_system,0), COALESCE(created_by,''), COALESCE(created_at,''), COALESCE(updated_at,'')
	FROM config_templates`

func scanTemplate(scanner interface{ Scan(...any) error }) (Template, error) {
	var t Template
	var vars, tags string
	err := scanner.Scan(&t.ID, &t.Name, &t.Description, &t.Type, &t.Category, &t.ServiceName, &t.Content,
		&vars, &t.OSType, &t.TargetType, &t.TargetPath, &tags, &t.Version, &t.IsSystem, &t.CreatedBy,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return t, err
	}
	t.Variables = decodeList(vars)
	t.Tags = decodeList(tags)
	return t, nil
}

func decodeList(raw string) []string {
	var out []string
	if json.Unmarshal([]byte(raw), &out) != nil || out == nil {
		out = []string{}
	}
	return out
}

func encodeList(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func list(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if size <= 0 {
		size, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	}
	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	where := []string{"1=1"}
	args := []any{}
	if q := strings.TrimSpace(r.URL.Query().Get("search")); q != "" {
		where = append(where, "(name LIKE ? OR description LIKE ? OR content LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	if typ := strings.TrimSpace(r.URL.Query().Get("type")); typ != "" {
		where = append(where, "type = ?")
		args = append(args, typ)
	}
	if tt := strings.TrimSpace(r.URL.Query().Get("target_type")); tt != "" {
		where = append(where, "target_type = ?")
		args = append(args, tt)
	}
	cond := " WHERE " + strings.Join(where, " AND ")
	db := appdb.DB()
	if db == nil {
		httpx.RespondError(w, "数据库未初始化", http.StatusInternalServerError)
		return
	}
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM config_templates`+cond, args...).Scan(&total); err != nil {
		httpx.RespondError(w, "读取配置模板失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rows, err := db.Query(templateQuery+cond+` ORDER BY is_system DESC, updated_at DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		httpx.RespondError(w, "读取配置模板失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	items := []Template{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			continue
		}
		items = append(items, t)
	}
	httpx.RespondRaw(w, http.StatusOK, map[string]any{"success": true, "data": items, "total": total})
}

func get(w http.ResponseWriter, r *http.Request) {
	row := appdb.DB().QueryRow(templateQuery+` WHERE id = ?`, chi.URLParam(r, "id"))
	t, err := scanTemplate(row)
	if err != nil {
		httpx.RespondError(w, "配置模板不存在", http.StatusNotFound)
		return
	}
	httpx.Respond(w, t)
}

func create(w http.ResponseWriter, r *http.Request) {
	var in templateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, "请求体不是合法 JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Content) == "" {
		httpx.RespondError(w, "名称和模板内容不能为空", http.StatusBadRequest)
		return
	}
	if in.Type == "" {
		in.Type = "generic"
	}
	if in.TargetType == "" {
		in.TargetType = "server"
	}
	if in.OSType == "" {
		in.OSType = "linux"
	}
	id := uuid.NewString()
	_, err := appdb.DB().Exec(`INSERT INTO config_templates
		(id,name,description,type,category,service_name,content,template_content,variables,os_type,target_type,target_path,tags,version,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,1,datetime('now','localtime'),datetime('now','localtime'))`,
		id, in.Name, in.Description, in.Type, in.Category, in.ServiceName, in.Content, in.Content,
		encodeList(in.Variables), in.OSType, in.TargetType, in.TargetPath, encodeList(in.Tags))
	if err != nil {
		httpx.RespondError(w, "创建配置模板失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	row := appdb.DB().QueryRow(templateQuery+` WHERE id = ?`, id)
	t, _ := scanTemplate(row)
	httpx.Respond(w, t)
}

func update(w http.ResponseWriter, r *http.Request) {
	var in templateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, "请求体不是合法 JSON", http.StatusBadRequest)
		return
	}
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Content) == "" {
		httpx.RespondError(w, "名称和模板内容不能为空", http.StatusBadRequest)
		return
	}
	if in.Type == "" {
		in.Type = "generic"
	}
	if in.TargetType == "" {
		in.TargetType = "server"
	}
	if in.OSType == "" {
		in.OSType = "linux"
	}
	res, err := appdb.DB().Exec(`UPDATE config_templates SET
		name=?, description=?, type=?, category=?, service_name=?, content=?, variables=?, os_type=?,
		target_type=?, target_path=?, tags=?, version=COALESCE(version,1)+1,
		updated_at=datetime('now','localtime') WHERE id=?`,
		in.Name, in.Description, in.Type, in.Category, in.ServiceName, in.Content,
		encodeList(in.Variables), in.OSType, in.TargetType, in.TargetPath, encodeList(in.Tags), id)
	if err != nil {
		httpx.RespondError(w, "更新配置模板失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.RespondError(w, "配置模板不存在", http.StatusNotFound)
		return
	}
	row := appdb.DB().QueryRow(templateQuery+` WHERE id = ?`, id)
	t, _ := scanTemplate(row)
	httpx.Respond(w, t)
}

func remove(w http.ResponseWriter, r *http.Request) {
	res, err := appdb.DB().Exec(`DELETE FROM config_templates WHERE id = ? AND COALESCE(is_system,0)=0`, chi.URLParam(r, "id"))
	if err != nil {
		httpx.RespondError(w, "删除配置模板失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.RespondError(w, "配置模板不存在或为系统内置模板", http.StatusBadRequest)
		return
	}
	httpx.RespondMessage(w, "配置模板已删除")
}

type renderInput struct {
	Variables map[string]string `json:"variables"`
}

func render(w http.ResponseWriter, r *http.Request) {
	var in renderInput
	_ = json.NewDecoder(r.Body).Decode(&in)
	row := appdb.DB().QueryRow(`SELECT COALESCE(content,'') FROM config_templates WHERE id = ?`, chi.URLParam(r, "id"))
	var content string
	if err := row.Scan(&content); err != nil {
		if err == sql.ErrNoRows {
			httpx.RespondError(w, "配置模板不存在", http.StatusNotFound)
			return
		}
		httpx.RespondError(w, "读取配置模板失败", http.StatusInternalServerError)
		return
	}
	out := content
	for k, v := range in.Variables {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
		out = strings.ReplaceAll(out, "{{ "+k+" }}", v)
	}
	httpx.Respond(w, map[string]any{"rendered": out})
}
