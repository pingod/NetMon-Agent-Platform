// Compose 编排项目管理（/api/v1/compose）。
//
// 之前前端调用该前缀一直是 404。这里提供项目 CRUD、YAML 校验、服务解析与日志；
// 配置了 Docker 宿主机时，up/down/restart 会真正通过 SSH 执行 docker compose。
package ops

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
)

type ComposeProject struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	YAMLContent  string `json:"yaml_content"`
	Status       string `json:"status"`
	ServiceCount int    `json:"service_count"`
	RunningCount int    `json:"running_count"`
	WorkingDir   string `json:"working_dir"`
	Tags         string `json:"tags"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type composeInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	YAMLContent string `json:"yaml_content"`
	Tags        string `json:"tags"`
}

// ComposeRoutes /api/v1/compose
func ComposeRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", composeList)
	r.Get("/{id}/services", composeServices)
	r.Get("/{id}/logs", composeLogs)
	r.Post("/validate", composeValidate)
	write := middleware.RequireRole("admin", "operator")
	r.With(write).Post("/", composeCreate)
	r.With(write).Put("/{id}", composeUpdate)
	r.With(write).Delete("/{id}", composeDelete)
	r.With(write).Post("/{id}/{action}", composeAction)
	return r
}

const composeQuery = `SELECT id, name, COALESCE(description,''), COALESCE(compose_content,''), COALESCE(status,'stopped'),
	COALESCE(service_count,0), COALESCE(running_count,0), COALESCE(working_dir,''), COALESCE(tags,''),
	COALESCE(created_at,''), COALESCE(updated_at,'') FROM compose_projects`

func scanCompose(row interface{ Scan(...any) error }) (ComposeProject, error) {
	var p ComposeProject
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.YAMLContent, &p.Status, &p.ServiceCount,
		&p.RunningCount, &p.WorkingDir, &p.Tags, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func composeList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	db := appdb.DB()
	where, args := "", []any{}
	if q := strings.TrimSpace(r.URL.Query().Get("search")); q != "" {
		where = " WHERE name LIKE ? OR description LIKE ?"
		args = append(args, "%"+q+"%", "%"+q+"%")
	}
	var total int
	_ = db.QueryRow(`SELECT COUNT(*) FROM compose_projects`+where, args...).Scan(&total)
	rows, err := db.Query(composeQuery+where+` ORDER BY updated_at DESC LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		httpx.RespondError(w, "读取 Compose 项目失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	items := []ComposeProject{}
	for rows.Next() {
		if p, err := scanCompose(rows); err == nil {
			items = append(items, p)
		}
	}
	httpx.RespondRaw(w, http.StatusOK, map[string]any{"success": true, "data": items, "total": total})
}

func composeCreate(w http.ResponseWriter, r *http.Request) {
	var in composeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, "请求体不是合法 JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.YAMLContent) == "" {
		httpx.RespondError(w, "项目名和 YAML 内容不能为空", http.StatusBadRequest)
		return
	}
	if _, err := parseComposeServices(in.YAMLContent); err != nil {
		httpx.RespondError(w, "YAML 解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	id := uuid.NewString()
	services, _ := parseComposeServices(in.YAMLContent)
	_, err := appdb.DB().Exec(`INSERT INTO compose_projects
		(id,name,description,compose_content,status,service_count,running_count,working_dir,tags,created_at,updated_at)
		VALUES (?,?,?,?, 'stopped', ?,0,?,?,datetime('now','localtime'),datetime('now','localtime'))`,
		id, in.Name, in.Description, in.YAMLContent, len(services), "/tmp/netmon-compose/"+id, in.Tags)
	if err != nil {
		httpx.RespondError(w, "创建 Compose 项目失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	p, _ := scanCompose(appdb.DB().QueryRow(composeQuery+` WHERE id = ?`, id))
	httpx.Respond(w, p)
}

func composeUpdate(w http.ResponseWriter, r *http.Request) {
	var in composeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, "请求体不是合法 JSON", http.StatusBadRequest)
		return
	}
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.YAMLContent) == "" {
		httpx.RespondError(w, "项目名和 YAML 内容不能为空", http.StatusBadRequest)
		return
	}
	services, err := parseComposeServices(in.YAMLContent)
	if err != nil {
		httpx.RespondError(w, "YAML 解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	res, err := appdb.DB().Exec(`UPDATE compose_projects SET name=?, description=?, compose_content=?,
		service_count=?, tags=?, updated_at=datetime('now','localtime') WHERE id=?`,
		in.Name, in.Description, in.YAMLContent, len(services), in.Tags, id)
	if err != nil {
		httpx.RespondError(w, "更新 Compose 项目失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.RespondError(w, "Compose 项目不存在", http.StatusNotFound)
		return
	}
	p, _ := scanCompose(appdb.DB().QueryRow(composeQuery+` WHERE id = ?`, id))
	httpx.Respond(w, p)
}

func composeDelete(w http.ResponseWriter, r *http.Request) {
	res, err := appdb.DB().Exec(`DELETE FROM compose_projects WHERE id = ?`, chi.URLParam(r, "id"))
	if err != nil {
		httpx.RespondError(w, "删除 Compose 项目失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.RespondError(w, "Compose 项目不存在", http.StatusNotFound)
		return
	}
	httpx.RespondMessage(w, "Compose 项目已删除")
}

type composeDoc struct {
	Services map[string]struct {
		Command any    `yaml:"command"`
		Image   string `yaml:"image"`
		Ports   []any  `yaml:"ports"`
	} `yaml:"services"`
}

type composeService struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	State   string `json:"state"`
	Ports   string `json:"ports"`
	Image   string `json:"image"`
}

func parseComposeServices(content string) ([]composeService, error) {
	var doc composeDoc
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, err
	}
	if len(doc.Services) == 0 {
		return nil, fmt.Errorf("services 段为空")
	}
	out := make([]composeService, 0, len(doc.Services))
	for name, svc := range doc.Services {
		cmd := ""
		switch v := svc.Command.(type) {
		case string:
			cmd = v
		case []any:
			parts := make([]string, 0, len(v))
			for _, p := range v {
				parts = append(parts, fmt.Sprint(p))
			}
			cmd = strings.Join(parts, " ")
		}
		ports := make([]string, 0, len(svc.Ports))
		for _, p := range svc.Ports {
			ports = append(ports, fmt.Sprint(p))
		}
		out = append(out, composeService{Name: name, Command: cmd, Ports: strings.Join(ports, ", "), Image: svc.Image})
	}
	return out, nil
}

func composeValidate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondRaw(w, http.StatusOK, map[string]any{"success": true, "valid": false, "error": "请求体不是合法 JSON"})
		return
	}
	if _, err := parseComposeServices(in.Content); err != nil {
		httpx.RespondRaw(w, http.StatusOK, map[string]any{"success": true, "valid": false, "error": err.Error()})
		return
	}
	httpx.RespondRaw(w, http.StatusOK, map[string]any{"success": true, "valid": true})
}

func composeServices(w http.ResponseWriter, r *http.Request) {
	var content, status string
	err := appdb.DB().QueryRow(`SELECT compose_content, COALESCE(status,'stopped') FROM compose_projects WHERE id = ?`,
		chi.URLParam(r, "id")).Scan(&content, &status)
	if err != nil {
		httpx.RespondError(w, "Compose 项目不存在", http.StatusNotFound)
		return
	}
	services, err := parseComposeServices(content)
	if err != nil {
		httpx.RespondError(w, "YAML 解析失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	for i := range services {
		services[i].State = status
	}
	httpx.Respond(w, services)
}

func composeAction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	action := chi.URLParam(r, "action")
	switch action {
	case "up", "down", "restart":
	default:
		httpx.RespondError(w, "不支持的操作: "+action, http.StatusBadRequest)
		return
	}
	p, err := scanCompose(appdb.DB().QueryRow(composeQuery+` WHERE id = ?`, id))
	if err != nil {
		httpx.RespondError(w, "Compose 项目不存在", http.StatusNotFound)
		return
	}
	status, running := "running", p.ServiceCount
	if action == "down" {
		status, running = "stopped", 0
	}
	executed, runErr := runComposeOnHost(p, action)
	if runErr != nil {
		_, _ = appdb.DB().Exec(`UPDATE compose_projects SET status='error', running_count=0, updated_at=datetime('now','localtime') WHERE id=?`, id)
		httpx.RespondFailStatus(w, http.StatusBadGateway, "Compose 执行失败: "+runErr.Error())
		return
	}
	note := ""
	if executed {
		note = "（已在 Docker 宿主机执行）"
	} else {
		// 没有可用宿主机：不谎报 running
		status, running = "stopped", 0
		note = "（未配置 Docker 宿主机，仅更新记录）"
	}
	_, _ = appdb.DB().Exec(`UPDATE compose_projects SET status=?, running_count=?, updated_at=datetime('now','localtime') WHERE id=?`,
		status, running, id)
	httpx.RespondRaw(w, http.StatusOK, map[string]any{"success": true, "status": status, "message": action + " 完成" + note})
}

// runComposeOnHost 尝试通过已纳管服务器执行 docker compose；未配置宿主机时返回 false,nil。
func runComposeOnHost(p ComposeProject, action string) (bool, error) {
	client, _, err := getDockerSSHClient("")
	if err != nil {
		return false, nil // 没有可用宿主机：仅更新记录，不阻断界面
	}
	defer client.Close()
	name := composeProjectName(p)
	composeCmd := "up -d"
	switch action {
	case "down":
		composeCmd = "down"
	case "restart":
		composeCmd = "restart"
	}
	// 需要 docker compose 插件；缺失时快速失败，不在请求里触发镜像 pull。
	if _, _, err := client.ExecCommand("docker compose version >/dev/null 2>&1", 10*time.Second); err != nil {
		return false, fmt.Errorf("当前 Docker 环境未安装 compose 插件")
	}
	cmd := fmt.Sprintf("docker compose -p %s -f - %s", shellQuote(name), composeCmd)
	stdout, stderr, err := client.ExecCommandInput(cmd, p.YAMLContent, 180*time.Second)
	if err != nil {
		return false, fmt.Errorf("%v %s %s", err, strings.TrimSpace(stderr), strings.TrimSpace(stdout))
	}
	return true, nil
}

var composeNameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func composeProjectName(p ComposeProject) string {
	name := composeNameRe.ReplaceAllString(p.Name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		name = p.ID
	}
	return strings.ToLower(name)
}

func composeLogs(w http.ResponseWriter, r *http.Request) {
	p, err := scanCompose(appdb.DB().QueryRow(composeQuery+` WHERE id = ?`, chi.URLParam(r, "id")))
	if err != nil {
		httpx.RespondError(w, "Compose 项目不存在", http.StatusNotFound)
		return
	}
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 {
		tail = 100
	}
	if client, _, err := getDockerSSHClient(""); err == nil {
		defer client.Close()
		out, stderr, err := client.ExecCommand(fmt.Sprintf(
			"docker compose -p %s logs --tail %d 2>&1",
			shellQuote(composeProjectName(p)), tail), 60*time.Second)
		if err == nil {
			httpx.Respond(w, out)
			return
		}
		httpx.Respond(w, strings.TrimSpace(out+"\n"+stderr))
		return
	}
	httpx.Respond(w, "暂无日志：未配置 Docker 宿主机。")
}
