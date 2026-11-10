// Package tokenblacklist token 黑名单服务，等价于 TS modules/auth/services/tokenBlacklist.ts
package tokenblacklist

import (
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"itops-backend/internal/logger"
	"itops-backend/internal/repos"
)

const (
	cleanupInterval = 10 * time.Minute
	maxCacheSize    = 10000
)

type cachedToken struct {
	expiresAt time.Time
}

var (
	mu    sync.Mutex
	cache = make(map[string]cachedToken)
	stop  chan struct{}
)

func decodeExpiry(token string) time.Time {
	// 仅解析不校验签名（等价 jwt.decode）
	p := jwt.NewParser()
	if tk, _, err := p.ParseUnverified(token, jwt.MapClaims{}); err == nil {
		if exp, ok := tk.Claims.(jwt.MapClaims)["exp"]; ok {
			switch v := exp.(type) {
			case float64:
				return time.Unix(int64(v), 0)
			case int64:
				return time.Unix(v, 0)
			case json_Number:
				if n, err := v.Int64(); err == nil {
					return time.Unix(n, 0)
				}
			}
		}
	}
	return time.Now().Add(24 * time.Hour)
}

// json_Number 兼容类型（避免引入 encoding/json 依赖的 Number 判断）
type json_Number interface{ Int64() (int64, error) }

func enforceCacheLimit() {
	if len(cache) <= maxCacheSize {
		return
	}
	now := time.Now()
	for t, c := range cache {
		if c.expiresAt.Before(now) {
			delete(cache, t)
		}
	}
	if len(cache) > maxCacheSize {
		// 删除一半最早过期的
		type kv struct {
			t string
			e time.Time
		}
		list := make([]kv, 0, len(cache))
		for t, c := range cache {
			list = append(list, kv{t, c.expiresAt})
		}
		// 简单按过期时间排序
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				if list[j].e.Before(list[i].e) {
					list[i], list[j] = list[j], list[i]
				}
			}
		}
		removeCount := (len(cache) + 1) / 2
		for i := 0; i < removeCount && i < len(list); i++ {
			delete(cache, list[i].t)
		}
	}
}

// AddToBlacklist 将 token 加入黑名单
func AddToBlacklist(token, reason, userID string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Failed to add token to blacklist: %v", r)
		}
	}()

	expiresAt := decodeExpiry(token)

	mu.Lock()
	cache[token] = cachedToken{expiresAt: expiresAt}
	enforceCacheLimit()
	mu.Unlock()

	var uidPtr, reasonPtr *string
	if userID != "" {
		uidPtr = &userID
	}
	if reason != "" {
		reasonPtr = &reason
	}
	expIso := expiresAt.UTC().Format(time.RFC3339)
	if err := repos.TokenBlacklists.Add(uuid.NewString(), token, uidPtr, reasonPtr, expIso); err != nil {
		logger.Errorf("Failed to persist blacklisted token: %v", err)
	}
}

// IsBlacklisted 判断 token 是否已被拉黑
func IsBlacklisted(token string) bool {
	mu.Lock()
	if c, ok := cache[token]; ok {
		if c.expiresAt.After(time.Now()) {
			mu.Unlock()
			return true
		}
		delete(cache, token)
	}
	mu.Unlock()

	blacklisted := repos.TokenBlacklists.IsBlacklisted(token)
	if blacklisted {
		mu.Lock()
		cache[token] = cachedToken{expiresAt: decodeExpiry(token)}
		enforceCacheLimit()
		mu.Unlock()
	}
	return blacklisted
}

// CleanExpiredTokens 清理过期 token（缓存 + DB）
func CleanExpiredTokens() {
	now := time.Now()
	mu.Lock()
	cleaned := 0
	for t, c := range cache {
		if c.expiresAt.Before(now) {
			delete(cache, t)
			cleaned++
		}
	}
	mu.Unlock()
	if cleaned > 0 {
		logger.Infof("Cleaned %d expired tokens from cache", cleaned)
	}
	dbCleaned := repos.TokenBlacklists.CleanExpired()
	logger.Infof("Cleaned up %d expired tokens from blacklist", dbCleaned)
}

// Init 启动时清理并开启定期清理（等价 initTokenBlacklist）
func Init() {
	CleanExpiredTokens()
	stop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				CleanExpiredTokens()
			case <-stop:
				return
			}
		}
	}()
}

// Shutdown 停止定期清理
func Shutdown() {
	if stop != nil {
		close(stop)
	}
}
