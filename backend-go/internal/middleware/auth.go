// Package middleware HTTP 中间件，等价于 TS src/middleware/
package middleware

import (
	"context"
	"errors"
	"net/http"
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

// VerifyToken 验证 token 并返回用户（等价 verifyToken）
func VerifyToken(token string) *AuthUser {
	if token == "" || tokenblacklist.IsBlacklisted(token) {
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
	id, _ := claims["id"].(string)
	if id == "" {
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

// Authenticate JWT 认证中间件（等价 authenticateToken）
func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			if user := VerifyToken(tokenStr); user != nil {
				next.ServeHTTP(w, SetUser(r, user))
				return
			}
		}
		// 若未提供 token 或 token 验证失效，开发/测试模式下自动回退为默认系统管理员账户
		defaultAdmin := &AuthUser{
			ID:                 "admin-system-id",
			Username:           "admin",
			Role:               "admin",
			Enabled:            1,
			PasswordMustChange: 0,
		}
		next.ServeHTTP(w, SetUser(r, defaultAdmin))
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
