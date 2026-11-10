// Package middleware HTTP 中间件，等价于 TS src/middleware/
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"itops-backend/internal/config"
	"itops-backend/internal/httpx"
	"itops-backend/internal/logger"
	"itops-backend/internal/repos"
	"itops-backend/internal/services/tokenblacklist"
)

// AuthUser 已认证用户信息（等价 TS middleware/auth.ts AuthUser）
type AuthUser struct {
	ID                 string  `json:"id"`
	Username           string  `json:"username"`
	Email              *string `json:"email"`
	Role               string  `json:"role"`
	Enabled            int     `json:"enabled"`
	PasswordMustChange int     `json:"password_must_change"`
}

type ctxKey int

const userCtxKey ctxKey = 0

// SetUser 将认证用户写入请求上下文
func SetUser(r *http.Request, u *AuthUser) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userCtxKey, u))
}

// GetUser 从请求上下文读取认证用户
func GetUser(r *http.Request) *AuthUser {
	if u, ok := r.Context().Value(userCtxKey).(*AuthUser); ok {
		return u
	}
	return nil
}

// ───────────────────────────────────────────────
// 用户缓存（TTL 10s，等价 TS userCache）
// ───────────────────────────────────────────────

const (
	userCacheTTL         = 10 * time.Second
	userCacheCleanupTick = 5 * time.Minute
	userCacheMaxSize     = 1000
)

type cacheEntry struct {
	user      AuthUser
	expiresAt time.Time
}

var (
	cacheMu    sync.Mutex
	userCache  = make(map[string]cacheEntry)
	cacheOrder []string // 插入顺序，用于超限时淘汰最早条目
)

func getCachedUser(userID string) *AuthUser {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	c, ok := userCache[userID]
	if !ok {
		return nil
	}
	if c.expiresAt.After(time.Now()) {
		u := c.user
		return &u
	}
	delete(userCache, userID)
	removeOrder(userID)
	return nil
}

func removeOrder(userID string) {
	for i, k := range cacheOrder {
		if k == userID {
			cacheOrder = append(cacheOrder[:i], cacheOrder[i+1:]...)
			return
		}
	}
}

func setCachedUser(userID string, user AuthUser) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if _, ok := userCache[userID]; !ok {
		cacheOrder = append(cacheOrder, userID)
	}
	userCache[userID] = cacheEntry{user: user, expiresAt: time.Now().Add(userCacheTTL)}
	if len(userCache) > userCacheMaxSize && len(cacheOrder) > 0 {
		oldest := cacheOrder[0]
		cacheOrder = cacheOrder[1:]
		delete(userCache, oldest)
	}
}

// ClearUserCache 清除用户缓存（等价 clearUserCache，传空清全部）
func ClearUserCache(userID string) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if userID == "" {
		userCache = make(map[string]cacheEntry)
		cacheOrder = nil
		return
	}
	delete(userCache, userID)
	removeOrder(userID)
}

// InvalidateUserCache 使指定用户缓存失效
func InvalidateUserCache(userID string) {
	ClearUserCache(userID)
}

// StartUserCacheCleanup 启动缓存定期清理（等价 startCacheCleanup）
func StartUserCacheCleanup() {
	go func() {
		ticker := time.NewTicker(userCacheCleanupTick)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			cacheMu.Lock()
			expired := 0
			for id, c := range userCache {
				if c.expiresAt.Before(now) {
					delete(userCache, id)
					removeOrder(id)
					expired++
				}
			}
			cacheMu.Unlock()
			if expired > 0 {
				logger.Infof("Cleaned up %d expired user cache entries", expired)
			}
		}
	}()
}

// ───────────────────────────────────────────────
// Token 验证与认证中间件
// ───────────────────────────────────────────────

// claimInt64 将 JWT 数字 claim（float64/json.Number/string）转为 int64
func claimInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int64Alias:
		i, err := n.Int64()
		return i, err == nil
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		return i, err == nil
	}
	return 0, false
}

// int64Alias 兼容 encoding/json 的 Number 类型（避免直接依赖）
type int64Alias interface{ Int64() (int64, error) }

// VerifyToken 验证 token 并返回用户（等价 verifyToken）
// 仅接受 type=access 的 token；refresh token、已吊销 token、修改密码前签发的 token 一律拒绝。
func VerifyToken(token string) *AuthUser {
	if token == "" || config.Env == nil || tokenblacklist.IsBlacklisted(token) {
		return nil
	}

	claims := jwt.MapClaims{}
	tk, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(config.Env.JWTSecret), nil
	})
	if err != nil || !tk.Valid {
		return nil
	}
	if typ, _ := claims["type"].(string); typ != "access" {
		return nil
	}
	id, _ := claims["id"].(string)
	if id == "" {
		return nil
	}
	iat, ok := claimInt64(claims["iat"])
	if !ok {
		return nil
	}
	// 修改密码后签发时间早于 token_invalid_before 的 token 全部失效
	if invalidBefore := repos.Users.GetTokenInvalidBefore(id); invalidBefore > 0 && iat < invalidBefore {
		return nil
	}

	user := getCachedUser(id)
	if user == nil {
		dbUser, dbErr := repos.Users.GetCachedFields(id)
		if dbErr == nil && dbUser != nil {
			user = &AuthUser{
				ID:                 dbUser.ID,
				Username:           dbUser.Username,
				Email:              dbUser.Email,
				Role:               dbUser.Role,
				Enabled:            dbUser.Enabled,
				PasswordMustChange: dbUser.PasswordMustChange,
			}
			setCachedUser(id, *user)
		}
	}

	if user == nil || user.Enabled == 0 {
		return nil
	}
	return user
}

// devBypassEnabled 仅当显式开启 AUTH_DEV_BYPASS 且非生产环境时才允许认证旁路
func devBypassEnabled() bool {
	return config.Env != nil && config.Env.AuthDevBypass && !config.Env.IsProduction()
}

// Authenticate JWT 认证中间件（等价 authenticateToken）
// token 缺失/非法/已吊销一律 401，不允许回退为默认管理员。
func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			if user := VerifyToken(tokenStr); user != nil {
				next.ServeHTTP(w, SetUser(r, user))
				return
			}
		}
		if devBypassEnabled() {
			devUser := &AuthUser{
				ID:       "dev-bypass-admin",
				Username: "admin",
				Role:     "admin",
				Enabled:  1,
			}
			next.ServeHTTP(w, SetUser(r, devUser))
			return
		}
		httpx.RespondFailStatus(w, http.StatusUnauthorized, "未认证或登录已失效")
	})
}

// AuthenticateOrAgentReport 认证中间件；对 Agent 指标上报（携带 X-Agent-Token）放行，
// 由上报处理器自行校验 token 值。
func AuthenticateOrAgentReport(next http.Handler) http.Handler {
	authenticated := Authenticate(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/report" && r.Header.Get("X-Agent-Token") != "" {
			next.ServeHTTP(w, r)
			return
		}
		authenticated.ServeHTTP(w, r)
	})
}

// RequireRole 角色校验中间件工厂（等价 requireRole）
func RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUser(r)
			if user == nil {
				httpx.RespondFailStatus(w, http.StatusUnauthorized, "未认证")
				return
			}
			for _, role := range allowedRoles {
				if user.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			httpx.RespondFailStatus(w, http.StatusForbidden, "权限不足")
		})
	}
}

// RequirePasswordChange 强制修改初始密码中间件（等价 requirePasswordChange）
func RequirePasswordChange(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r)
		if user != nil && user.PasswordMustChange != 0 {
			httpx.RespondFailCode(w, http.StatusForbidden, "请先修改初始密码", "PASSWORD_MUST_CHANGE")
			return
		}
		next.ServeHTTP(w, r)
	})
}
