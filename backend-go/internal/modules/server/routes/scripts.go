// 脚本中心 — /api/v1/scripts
// CRUD + 参数化执行（SSH 到目标服务器，复用 server-commands 的连接与审计基础设施）
package routes

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
	"itops-backend/internal/services/audit"
)

const maxScriptBodyBytes = 128 * 1024
const maxScriptContentBytes = 64 * 1024

var scriptTypes = map[string]bool{"shell": true, "python": true, "powershell": true}
var scriptParamName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,31}$`)
var scriptPlaceholder = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

// ScriptRoutes 返回 /api/v1/scripts 路由
func ScriptRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", handleListScripts)
	r.Get("/categories", handleScriptCategories)
	r.With(middleware.RequireRole("admin", "operator")).Post("/", handleCreateScript)
	r.With(middleware.RequireRole("admin", "operator")).Put("/{id}", handleUpdateScript)
	r.With(middleware.RequireRole("admin")).Delete("/{id}", handleDeleteScript)
	r.With(middleware.RequireRole("admin", "operator")).Post("/{id}/execute", handleExecuteScript)
	return r
}

func decodeScriptBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxScriptBodyBytes)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "无效的请求体")
		return false
	}
	return true
}

type scriptBody struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Content     string                 `json:"content"`
	Category    string                 `json:"category"`
	Parameters  []repos.ScriptParameter `json:"parameters"`
}

// validate 校验脚本输入，返回错误消息（空串=通过）
func (b *scriptBody) validate() string {
	b.Name = strings.TrimSpace(b.Name)
	b.Description = strings.TrimSpace(b.Description)
	b.Category = strings.TrimSpace(b.Category)
	if b.Name == "" || len(b.Name) > 100 {
		return "脚本名称必填且不超过 100 字符"
	}
	if !scriptTypes[b.Type] {
		return "type 必须为 shell/python/powershell"
	}
	if strings.TrimSpace(b.Content) == "" {
		return "脚本内容不能为空"
	}
	if len(b.Content) > maxScriptContentBytes {
		return "脚本内容不能超过 64KB"
	}
	if len(b.Description) > 500 {
		return "描述不超过 500 字符"
	}
	if len(b.Category) > 50 {
		return "分类不超过 50 字符"
	}
	if len(b.Parameters) > 20 {
		return "参数最多 20 个"
	}
	seen := map[string]bool{}
	for _, p := range b.Parameters {
		if !scriptParamName.MatchString(p.Name) {
			return "参数名需为字母/下划线开头的标识符（≤32 字符）: " + p.Name
		}
		if seen[p.Name] {
			return "参数名重复: " + p.Name
		}
		seen[p.Name] = true
	}
	return ""
}

func (b *scriptBody) toInput() repos.ScriptInput {
	params := b.Parameters
	if params == nil {
		params = []repos.ScriptParameter{}
	}
	return repos.ScriptInput{
		Name:        b.Name,
		Description: b.Description,
		Type:        b.Type,
		Content:     b.Content,
		Parameters:  params,
		Category:    b.Category,
	}
}

func auditScript(r *http.Request, action, scriptID string, details map[string]interface{}) {
	userID := ""
	if u := middleware.GetUser(r); u != nil {
		userID = u.ID
	}
	audit.CreateAuditLog(audit.AuditInput{
		UserID:       userID,
		Action:       action,
		ResourceType: "script",
		ResourceID:   scriptID,
		Details:      details,
		IPAddress:    middleware.ClientIP(r),
	})
}

func handleListScripts(w http.ResponseWriter, r *http.Request) {
	list, err := repos.Scripts.List(r.URL.Query().Get("search"), r.URL.Query().Get("category"))
	if err != nil {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "获取脚本列表失败")
		return
	}
	httpx.Respond(w, list)
}

func handleScriptCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := repos.Scripts.Categories()
	if err != nil {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "获取分类失败")
		return
	}
	httpx.Respond(w, cats)
}

func handleCreateScript(w http.ResponseWriter, r *http.Request) {
	var body scriptBody
	if !decodeScriptBody(w, r, &body) {
		return
	}
	if msg := body.validate(); msg != "" {
		httpx.RespondFailStatus(w, http.StatusBadRequest, msg)
		return
	}
	id := uuid.New().String()
	if err := repos.Scripts.Create(id, body.toInput()); err != nil {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "创建脚本失败")
		return
	}
	auditScript(r, "script_create", id, map[string]interface{}{"name": body.Name, "type": body.Type})
	s, _ := repos.Scripts.GetByID(id)
	httpx.Respond(w, s)
}

func handleUpdateScript(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := repos.Scripts.GetByID(id); err != nil {
		httpx.RespondFailStatus(w, http.StatusNotFound, "脚本不存在")
		return
	}
	var body scriptBody
	if !decodeScriptBody(w, r, &body) {
		return
	}
	if msg := body.validate(); msg != "" {
		httpx.RespondFailStatus(w, http.StatusBadRequest, msg)
		return
	}
	if _, err := repos.Scripts.Update(id, body.toInput()); err != nil {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "更新脚本失败")
		return
	}
	auditScript(r, "script_update", id, map[string]interface{}{"name": body.Name})
	s, _ := repos.Scripts.GetByID(id)
	httpx.Respond(w, s)
}

func handleDeleteScript(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s, err := repos.Scripts.GetByID(id)
	if err != nil {
		httpx.RespondFailStatus(w, http.StatusNotFound, "脚本不存在")
		return
	}
	if ok, err := repos.Scripts.Delete(id); err != nil || !ok {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "删除脚本失败")
		return
	}
	auditScript(r, "script_delete", id, map[string]interface{}{"name": s.Name})
	httpx.RespondMessage(w, "脚本已删除")
}

// shellQuoteParam 单引号转义，确保参数值不会被目标 shell 解释
func shellQuoteParam(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// renderScript 把 {{param}} 替换为按目标语言安全 quoting 的值。
// 替换后仍残留占位符说明参数缺失，返回错误而不是把字面量带上服务器执行。
func renderScript(s *repos.ScriptRecord, params map[string]string) (string, error) {
	for _, p := range s.Parameters {
		v, ok := params[p.Name]
		if p.Required && (!ok || strings.TrimSpace(v) == "") {
			return "", fmt.Errorf("缺少必填参数: %s", p.Name)
		}
	}
	var unknown []string
	rendered := scriptPlaceholder.ReplaceAllStringFunc(s.Content, func(m string) string {
		name := scriptPlaceholder.FindStringSubmatch(m)[1]
		v, ok := params[name]
		if !ok {
			unknown = append(unknown, name)
			return m
		}
		switch s.Type {
		case "python":
			return strconv.Quote(v)
		case "powershell":
			return "'" + strings.ReplaceAll(v, "'", "''") + "'"
		default:
			return shellQuoteParam(v)
		}
	})
	if len(unknown) > 0 {
		return "", fmt.Errorf("脚本引用了未定义的参数: %s", strings.Join(unknown, ", "))
	}
	return rendered, nil
}

// buildExecCommand 把脚本内容包装成一条可经 SSH exec 执行的命令。
// 内容走 base64 传输，杜绝脚本文本与 shell 元字符的注入面。
func buildExecCommand(scriptType, content string) string {
	b64 := base64.StdEncoding.EncodeToString([]byte(content))
	switch scriptType {
	case "python":
		return "echo " + b64 + " | base64 -d | python3 -"
	case "powershell":
		// Windows PowerShell -EncodedCommand 要求 UTF-16LE base64
		u := utf16.Encode([]rune(content))
		buf := make([]byte, len(u)*2)
		for i, c := range u {
			buf[i*2] = byte(c)
			buf[i*2+1] = byte(c >> 8)
		}
		return "powershell -NoProfile -EncodedCommand " + base64.StdEncoding.EncodeToString(buf)
	default:
		return "echo " + b64 + " | base64 -d | bash"
	}
}

func handleExecuteScript(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s, err := repos.Scripts.GetByID(id)
	if err != nil {
		httpx.RespondFailStatus(w, http.StatusNotFound, "脚本不存在")
		return
	}
	var body struct {
		Parameters map[string]string `json:"parameters"`
		ServerID   string            `json:"serverId"`
	}
	if !decodeScriptBody(w, r, &body) {
		return
	}
	rendered, err := renderScript(s, body.Parameters)
	if err != nil {
		httpx.RespondFailStatus(w, http.StatusBadRequest, err.Error())
		return
	}

	// 未选服务器：仅渲染预览，不执行
	if body.ServerID == "" {
		httpx.Respond(w, map[string]interface{}{"executed": false, "output": rendered})
		return
	}

	if exists, _ := repos.Servers.Exists(body.ServerID); !exists {
		httpx.RespondFailStatus(w, http.StatusNotFound, "目标服务器不存在")
		return
	}
	start := time.Now()
	stdout, stderr, execErr := ExecOnServer(body.ServerID, buildExecCommand(s.Type, rendered), 60*time.Second)
	duration := time.Since(start).Milliseconds()
	var connErr *SSHConnectError
	if errors.As(execErr, &connErr) {
		// 连接失败也是一次执行尝试，同样落审计
		auditScript(r, "script_execute", id, map[string]interface{}{
			"name": s.Name, "serverId": body.ServerID, "success": false, "error": "connect_failed",
		})
		httpx.RespondFailStatus(w, http.StatusBadGateway, execErr.Error())
		return
	}
	success := execErr == nil
	repos.Servers.UpdateLastConnected(body.ServerID)

	executedBy := ""
	if u := middleware.GetUser(r); u != nil {
		executedBy = u.Username
	}
	successInt := 0
	if success {
		successInt = 1
	}
	_ = repos.Servers.InsertCommandHistory(body.ServerID, "script:"+s.Name, stdout, stderr, successInt, int(duration), executedBy)
	auditScript(r, "script_execute", id, map[string]interface{}{
		"name": s.Name, "serverId": body.ServerID, "success": success,
	})

	output := stdout
	if stderr != "" {
		if output != "" {
			output += "\n"
		}
		output += "[stderr]\n" + stderr
	}
	if execErr != nil {
		output += "\n[exit] " + execErr.Error()
	}
	httpx.Respond(w, map[string]interface{}{
		"executed": true,
		"success":  success,
		"output":   output,
		"duration": duration,
	})
}