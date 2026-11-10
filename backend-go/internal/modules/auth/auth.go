// Package auth 认证模块路由，等价于 TS modules/auth/routes/authRoutes.ts
package auth

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"itops-backend/internal/config"
	"itops-backend/internal/httpx"
	"itops-backend/internal/jwtutil"
	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
	"itops-backend/internal/services/audit"
	"itops-backend/internal/services/loginthrottle"
	"itops-backend/internal/services/tokenblacklist"
	"itops-backend/internal/utils"
)

// Routes 返回 /api/v1/auth 路由
func Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/login", login)
	r.Post("/refresh", refresh)
	r.With(middleware.Authenticate).Get("/me", me)
	r.With(middleware.Authenticate).Post("/logout", logout)
	r.With(middleware.Authenticate).Post("/change-password", changePassword)
	return r
}

func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// POST /login 登录
func login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "请求参数验证失败: 无效的请求体")
		return
	}

	// 等价 authSchemas.login 校验
	var errs []string
	if strings.TrimSpace(body.Username) == "" {
		errs = append(errs, "username: 用户名不能为空")
	}
	if strings.TrimSpace(body.Password) == "" {
		errs = append(errs, "password: 密码不能为空")
	}
	if len(errs) > 0 {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "请求参数验证失败: "+strings.Join(errs, "; "))
		return
	}

	username, password := body.Username, body.Password

	// 锁定检查
	lockout := loginthrottle.CheckLoginLockout(username)
	if lockout.Locked {
		remainingMinutes := int(math.Ceil(lockout.LockoutUntil.Sub(time.Now()).Minutes()))
		if remainingMinutes < 0 {
			remainingMinutes = 0
		}
		msg := "账户已被锁定，请" + itoa(remainingMinutes) + "分钟后再试"
		httpx.RespondFailStatus(w, http.StatusLocked, msg)
		return
	}

	user, err := repos.Users.GetForAuth(username)
	if err != nil || user == nil {
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	if user.Enabled == 0 {
		httpx.RespondFailStatus(w, http.StatusForbidden, "账户已被禁用")
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		result := loginthrottle.RecordFailedLogin(username)
		if result.Locked {
			httpx.RespondFailStatus(w, http.StatusLocked, "密码错误次数过多，账户已被锁定30分钟")
			return
		}
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "用户名或密码错误，剩余"+itoa(result.RemainingAttempts)+"次尝试机会")
		return
	}

	loginthrottle.ResetFailedLoginAttempts(user.ID)

	accessToken, err := jwtutil.SignAccessToken(user.ID, user.Username, user.Role, user.Email)
	if err != nil {
		logger.Errorf("登录失败: %v", err)
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	refreshToken, err := jwtutil.SignRefreshToken(user.ID)
	if err != nil {
		logger.Errorf("登录失败: %v", err)
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	repos.Users.TouchLogin(user.ID)

	audit.CreateAuditLog(audit.AuditInput{
		UserID:       user.ID,
		Action:       "login",
		ResourceType: "auth",
		ResourceID:   "login",
		Details:      map[string]interface{}{"username": username},
		IPAddress:    middleware.ClientIP(r),
	})

	httpx.RespondRaw(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "登录成功",
		"data": map[string]interface{}{
			"token":        accessToken,
			"refreshToken": refreshToken,
			"user": map[string]interface{}{
				"id":                 user.ID,
				"username":           user.Username,
				"email":              user.Email,
				"role":               user.Role,
				"passwordMustChange": user.PasswordMustChange != 0,
			},
		},
	})
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// POST /refresh 刷新 token
func refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	_ = decodeJSON(r, &body)

	if body.RefreshToken == "" {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "请提供refresh token")
		return
	}

	if tokenblacklist.IsBlacklisted(body.RefreshToken) {
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "Token已失效")
		return
	}

	claims := jwt.MapClaims{}
	tk, err := jwt.ParseWithClaims(body.RefreshToken, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(config.Env.JWTSecret), nil
	})
	if err != nil || !tk.Valid {
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "Refresh token已过期或无效")
		return
	}

	tokenType, _ := claims["type"].(string)
	if tokenType != "refresh" {
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "无效的refresh token")
		return
	}
	id, _ := claims["id"].(string)

	user, uerr := repos.Users.GetForWebSocket(id)
	if uerr != nil || user == nil || user.Enabled == 0 {
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "用户不存在或已被禁用")
		return
	}

	newAccessToken, err := jwtutil.SignAccessToken(user.ID, user.Username, user.Role, user.Email)
	if err != nil {
		logger.Errorf("Token刷新失败: %v", err)
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	newRefreshToken, err := jwtutil.SignRefreshToken(user.ID)
	if err != nil {
		logger.Errorf("Token刷新失败: %v", err)
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	tokenblacklist.AddToBlacklist(body.RefreshToken, "token-refresh", id)

	httpx.RespondRaw(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"token":        newAccessToken,
			"refreshToken": newRefreshToken,
		},
	})
}

// GET /me 获取当前用户信息
func me(w http.ResponseWriter, r *http.Request) {
	authUser := middleware.GetUser(r)
	if authUser == nil {
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "无效的token")
		return
	}
	user, err := repos.Users.GetProfile(authUser.ID)
	if err != nil || user == nil {
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "用户不存在")
		return
	}
	httpx.Respond(w, user)
}

// POST /logout 退出登录
func logout(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		userID := ""
		if u := middleware.GetUser(r); u != nil {
			userID = u.ID
		}
		tokenblacklist.AddToBlacklist(token, "user-logout", userID)
	}
	httpx.RespondRaw(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "退出成功",
	})
}

// POST /change-password 修改密码
func changePassword(w http.ResponseWriter, r *http.Request) {
	authUser := middleware.GetUser(r)
	if authUser == nil {
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "无效的token")
		return
	}

	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	_ = decodeJSON(r, &body)

	if body.CurrentPassword == "" || body.NewPassword == "" {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "请提供当前密码和新密码")
		return
	}

	user, err := repos.Users.GetByID(authUser.ID)
	if err != nil || user == nil {
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "用户不存在")
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.CurrentPassword)) != nil {
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "当前密码错误")
		return
	}

	check := utils.ValidatePassword(body.NewPassword)
	if !check.Valid {
		httpx.RespondFailStatus(w, http.StatusBadRequest, check.Message)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), 12)
	if err != nil {
		logger.Errorf("修改密码失败: %v", err)
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	if _, err := repos.Users.UpdatePassword(user.ID, string(hashed)); err != nil {
		logger.Errorf("修改密码失败: %v", err)
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	middleware.InvalidateUserCache(user.ID)

	audit.CreateAuditLog(audit.AuditInput{
		UserID:       user.ID,
		Action:       "change_password",
		ResourceType: "auth",
		ResourceID:   "password",
		Details:      map[string]interface{}{"username": user.Username},
		IPAddress:    middleware.ClientIP(r),
	})

	logger.Infof("用户 %s 修改了密码", user.Username)

	httpx.RespondRaw(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "密码修改成功",
	})
}

// 占位避免 strconv 重复导入（login 中用于拼接剩余次数）
var _ = uuid.NewString
