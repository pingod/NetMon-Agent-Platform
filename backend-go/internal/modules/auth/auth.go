// Package auth 认证模块路由，等价于 TS modules/auth/routes/authRoutes.ts
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
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

const maxAuthBodyBytes = 64 * 1024

// dummyBcryptHash 用于用户不存在时执行一次固定 bcrypt 比对，消除时间侧信道
const dummyBcryptHash = "$2a$12$oqDXIaNfzuOoXS5MqfkDNu8THvL6Ng4ojgKv29DvD1vJWZv15i9de"

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

func auditLoginFailure(r *http.Request, username, userID string) {
	audit.CreateAuditLog(audit.AuditInput{
		UserID:       userID,
		Action:       "login_failed",
		ResourceType: "auth",
		ResourceID:   "login",
		Details:      map[string]interface{}{"username": username},
		IPAddress:    middleware.ClientIP(r),
	})
}

// POST /login 登录
func login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)

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
	const genericMsg = "用户名或密码错误"

	user, err := repos.Users.GetForAuth(username)
	if err != nil || user == nil {
		// 用户不存在时也执行一次 bcrypt 比对，避免通过响应时间枚举用户名
		_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
		auditLoginFailure(r, username, "")
		httpx.RespondFailStatus(w, http.StatusUnauthorized, genericMsg)
		return
	}

	// 先验证密码，再判断账户状态，避免向未持有凭据者泄露账户存在性/禁用状态
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		loginthrottle.RecordFailedLogin(username)
		auditLoginFailure(r, username, user.ID)
		httpx.RespondFailStatus(w, http.StatusUnauthorized, genericMsg)
		return
	}

	if user.Enabled == 0 {
		auditLoginFailure(r, username, user.ID)
		httpx.RespondFailStatus(w, http.StatusUnauthorized, genericMsg)
		return
	}

	lockout := loginthrottle.CheckLoginLockout(username)
	if lockout.Locked {
		remainingMinutes := int(time.Until(lockout.LockoutUntil).Minutes()) + 1
		if remainingMinutes < 0 {
			remainingMinutes = 0
		}
		httpx.RespondFailStatus(w, http.StatusLocked, fmt.Sprintf("账户已被锁定，请%d分钟后再试", remainingMinutes))
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

// refreshTokenFromRequest 从 body 或 X-Refresh-Token 头读取 refresh token
func refreshTokenFromRequest(r *http.Request) string {
	defer r.Body.Close()
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, maxAuthBodyBytes)).Decode(&body)
	if strings.TrimSpace(body.RefreshToken) != "" {
		return strings.TrimSpace(body.RefreshToken)
	}
	return strings.TrimSpace(r.Header.Get("X-Refresh-Token"))
}

// parseRefreshToken 校验签名、类型与吊销状态，返回用户 ID
func parseRefreshToken(token string) (string, bool) {
	if token == "" || config.Env == nil || tokenblacklist.IsBlacklisted(token) {
		return "", false
	}
	claims := jwt.MapClaims{}
	tk, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(config.Env.JWTSecret), nil
	})
	if err != nil || !tk.Valid {
		return "", false
	}
	if typ, _ := claims["type"].(string); typ != "refresh" {
		return "", false
	}
	id, _ := claims["id"].(string)
	if id == "" {
		return "", false
	}
	if invalidBefore := repos.Users.GetTokenInvalidBefore(id); invalidBefore > 0 {
		iat, ok := claimTime(claims["iat"])
		if !ok || iat < invalidBefore {
			return "", false
		}
	}
	return id, true
}

func claimTime(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	}
	return 0, false
}

// POST /refresh 刷新 token
func refresh(w http.ResponseWriter, r *http.Request) {
	token := refreshTokenFromRequest(r)
	if token == "" {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "请提供refresh token")
		return
	}

	id, ok := parseRefreshToken(token)
	if !ok {
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "Refresh token已过期或无效")
		return
	}

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

	tokenblacklist.AddToBlacklist(token, "token-refresh", id)

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

// POST /logout 退出登录：同时吊销 access token 与 refresh token
func logout(w http.ResponseWriter, r *http.Request) {
	userID := ""
	if u := middleware.GetUser(r); u != nil {
		userID = u.ID
	}

	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		accessToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		tokenblacklist.AddToBlacklist(accessToken, "user-logout", userID)
	}
	if refreshToken := refreshTokenFromRequest(r); refreshToken != "" {
		tokenblacklist.AddToBlacklist(refreshToken, "user-logout-refresh", userID)
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

	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
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
		auditLoginFailure(r, user.Username, user.ID)
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

	// 修改密码后让该用户此前签发的所有 token 立即失效
	invalidBefore := time.Now().Unix()
	if _, err := repos.Users.UpdatePasswordAndRevokeTokens(user.ID, string(hashed), invalidBefore); err != nil {
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
