// 用户管理路由 — /api/v1/users（仅 admin）
// 对应前端 Users.tsx 的列表/创建/更新/删除
package auth

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
	"itops-backend/internal/services/audit"
	"itops-backend/internal/utils"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

var validRoles = map[string]bool{"admin": true, "operator": true, "viewer": true}

// UserRoutes 返回 /api/v1/users 路由
func UserRoutes() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Authenticate, middleware.RequireRole("admin"))

	r.Get("/", listUsers)
	r.Post("/", createUser)
	r.Put("/{id}", updateUser)
	r.Delete("/{id}", deleteUser)
	return r
}

func auditUser(r *http.Request, action, targetID string, details map[string]interface{}) {
	userID := ""
	if u := middleware.GetUser(r); u != nil {
		userID = u.ID
	}
	audit.CreateAuditLog(audit.AuditInput{
		UserID:       userID,
		Action:       action,
		ResourceType: "user",
		ResourceID:   targetID,
		Details:      details,
		IPAddress:    middleware.ClientIP(r),
	})
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := repos.Users.List()
	if err != nil {
		httpx.RespondError(w, "获取用户列表失败", http.StatusInternalServerError)
		return
	}
	if users == nil {
		users = []repos.UserListItem{}
	}
	httpx.Respond(w, users)
}

func createUser(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	var body struct {
		Username string  `json:"username"`
		Password string  `json:"password"`
		Email    *string `json:"email"`
		Role     string  `json:"role"`
		Enabled  *bool   `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "无效的请求体")
		return
	}

	body.Username = strings.TrimSpace(body.Username)
	if !usernamePattern.MatchString(body.Username) {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "用户名需为 3-32 位字母、数字、下划线或连字符")
		return
	}
	if !validRoles[body.Role] {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "role 必须为 admin/operator/viewer")
		return
	}
	if check := utils.ValidatePassword(body.Password); !check.Valid {
		httpx.RespondFailStatus(w, http.StatusBadRequest, check.Message)
		return
	}
	if body.Email != nil && strings.TrimSpace(*body.Email) != "" && !strings.Contains(*body.Email, "@") {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "邮箱格式无效")
		return
	}
	if repos.Users.ExistsByUsername(body.Username) {
		httpx.RespondFailStatus(w, http.StatusConflict, "用户名已存在")
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(body.Password), 12)
	if err != nil {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "密码加密失败")
		return
	}

	enabled := 1
	if body.Enabled != nil && !*body.Enabled {
		enabled = 0
	}
	var email *string
	if body.Email != nil && strings.TrimSpace(*body.Email) != "" {
		e := strings.TrimSpace(*body.Email)
		email = &e
	}

	id := uuid.New().String()
	if err := repos.Users.Create(repos.UserCreateInput{
		ID:             id,
		Username:       body.Username,
		HashedPassword: string(hashed),
		Email:          email,
		Role:           body.Role,
		Enabled:        enabled,
	}); err != nil {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "创建用户失败")
		return
	}

	auditUser(r, "user_create", id, map[string]interface{}{"username": body.Username, "role": body.Role})
	user, _ := repos.Users.GetByIDSafe(id)
	httpx.Respond(w, user)
}

func updateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	target, err := repos.Users.GetByIDSafe(id)
	if err != nil || target == nil {
		httpx.RespondFailStatus(w, http.StatusNotFound, "用户不存在")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	var body struct {
		Username *string `json:"username"`
		Password *string `json:"password"`
		Email    *string `json:"email"`
		Role     *string `json:"role"`
		Enabled  *bool   `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "无效的请求体")
		return
	}

	actor := middleware.GetUser(r)
	isSelf := actor != nil && actor.ID == id

	// 自我保护：不允许撤销自己的管理员权限或停用自己，避免把系统锁死
	if isSelf {
		if body.Role != nil && *body.Role != "admin" {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "不能降级自己的角色")
			return
		}
		if body.Enabled != nil && !*body.Enabled {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "不能停用自己的账号")
			return
		}
	}

	// 最后一名启用管理员保护
	losesAdmin := (body.Role != nil && *body.Role != "admin") || (body.Enabled != nil && !*body.Enabled)
	if target.Role == "admin" && target.Enabled == 1 && losesAdmin {
		if repos.Users.CountEnabledAdmins() <= 1 {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "系统至少需要保留一名启用的管理员")
			return
		}
	}

	input := repos.UserUpdateInput{}
	changed := map[string]interface{}{}

	if body.Username != nil {
		name := strings.TrimSpace(*body.Username)
		if !usernamePattern.MatchString(name) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "用户名需为 3-32 位字母、数字、下划线或连字符")
			return
		}
		if name != target.Username && repos.Users.ExistsByUsername(name) {
			httpx.RespondFailStatus(w, http.StatusConflict, "用户名已存在")
			return
		}
		input.Username = &name
		changed["username"] = name
	}
	if body.Role != nil {
		if !validRoles[*body.Role] {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "role 必须为 admin/operator/viewer")
			return
		}
		input.Role = body.Role
		changed["role"] = *body.Role
	}
	if body.Enabled != nil {
		v := 0
		if *body.Enabled {
			v = 1
		}
		input.Enabled = &v
		changed["enabled"] = v
	}
	if body.Email != nil {
		e := strings.TrimSpace(*body.Email)
		var ep *string
		if e == "" {
			changed["email"] = ""
		} else {
			if !strings.Contains(e, "@") {
				httpx.RespondFailStatus(w, http.StatusBadRequest, "邮箱格式无效")
				return
			}
			ep = &e
			changed["email"] = e
		}
		input.Email = &ep
	}

	if len(changed) > 0 {
		if _, err := repos.Users.Update(id, input); err != nil {
			httpx.RespondFailStatus(w, http.StatusInternalServerError, "更新用户失败")
			return
		}
	}

	// 密码单独处理：更新并吊销该用户全部已签发 token
	if body.Password != nil && *body.Password != "" {
		if check := utils.ValidatePassword(*body.Password); !check.Valid {
			httpx.RespondFailStatus(w, http.StatusBadRequest, check.Message)
			return
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(*body.Password), 12)
		if err != nil {
			httpx.RespondFailStatus(w, http.StatusInternalServerError, "密码加密失败")
			return
		}
		if _, err := repos.Users.UpdatePasswordAndRevokeTokens(id, string(hashed), time.Now().Unix()); err != nil {
			httpx.RespondFailStatus(w, http.StatusInternalServerError, "更新密码失败")
			return
		}
		changed["password"] = "reset"
	}

	if len(changed) == 0 {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "没有需要更新的字段")
		return
	}

	// 角色/启用状态变化立即生效（中间件有 10s 用户缓存）
	middleware.InvalidateUserCache(id)
	auditUser(r, "user_update", id, changed)
	user, _ := repos.Users.GetByIDSafe(id)
	httpx.Respond(w, user)
}

func deleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	target, err := repos.Users.GetByIDSafe(id)
	if err != nil || target == nil {
		httpx.RespondFailStatus(w, http.StatusNotFound, "用户不存在")
		return
	}

	if actor := middleware.GetUser(r); actor != nil && actor.ID == id {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "不能删除自己的账号")
		return
	}
	if target.Role == "admin" && target.Enabled == 1 && repos.Users.CountEnabledAdmins() <= 1 {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "系统至少需要保留一名启用的管理员")
		return
	}

	deleted, err := repos.Users.Delete(id)
	if err != nil || !deleted {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "删除用户失败")
		return
	}

	middleware.InvalidateUserCache(id)
	auditUser(r, "user_delete", id, map[string]interface{}{"username": target.Username})
	httpx.RespondMessage(w, "用户已删除")
}