package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"itops-backend/internal/config"
	"itops-backend/internal/httpx"
	"itops-backend/internal/logger"
)

// ───────────────────────────────────────────────
// 限流（等价 TS middleware/rateLimiter.ts）
// ───────────────────────────────────────────────

const (
	maxStoreSize       = 10000
	cleanupIntervalRL  = 60 * time.Second
	defaultWindowMs    = 60 * 1000
	defaultMaxRequests = 300
)

type rateLimitEntry struct {
	count      int
	resetTime  time.Time
	lastAccess time.Time
}

type rateLimitCfg struct {
	prefix string
	window time.Duration
	max    int
}

// 按 TS rateLimitConfig 的顺序匹配（前缀优先）
var rateLimitRoutes = []rateLimitCfg{
	{prefix: "/api/v1/auth/login", window: 15 * time.Minute, max: 20},
	{prefix: "/api/v1/auth", window: 1 * time.Minute, max: 60},
	{prefix: "/api/v1/copilot", window: 1 * time.Minute, max: 30},
	{prefix: "/api/v1/settings/api-keys", window: 1 * time.Minute, max: 10},
	{prefix: "/api/v1/webhooks", window: 1 * time.Second, max: 10},
}

var (
	rlMu    sync.Mutex
	rlStore = make(map[string]*rateLimitEntry)
)

// ClientIP 提取客户端 IP（剥离 IPv4 映射前缀与端口）
// remoteHost 提取 RemoteAddr 的 IP（去端口与 IPv4 映射前缀）
func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return strings.TrimPrefix(strings.TrimSpace(host), "::ffff:")
}

// isTrustedProxy 判断远端地址是否命中 TRUSTED_PROXIES（IP 或 CIDR）
func isTrustedProxy(ip string) bool {
	if config.Env == nil || ip == "" {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, entry := range config.Env.TrustedProxies {
		if strings.Contains(entry, "/") {
			if _, network, err := net.ParseCIDR(entry); err == nil && network.Contains(parsed) {
				return true
			}
			continue
		}
		if p := net.ParseIP(entry); p != nil && p.Equal(parsed) {
			return true
		}
	}
	return false
}

// ClientIP 提取客户端 IP。
// 只有当直连地址命中可信代理白名单时才信任 X-Forwarded-For / X-Real-IP，
// 避免客户端伪造转发头绕过限流或污染审计日志。
func ClientIP(r *http.Request) string {
	direct := remoteHost(r.RemoteAddr)
	if !isTrustedProxy(direct) {
		return direct
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if first != "" {
			return strings.TrimPrefix(first, "::ffff:")
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return strings.TrimPrefix(xri, "::ffff:")
	}
	return direct
}

func ipWhitelist() []string {
	if config.Env == nil || config.Env.WebhookIPWhitelist == "" {
		return nil
	}
	parts := strings.Split(config.Env.WebhookIPWhitelist, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func isIPInCIDR(ip, cidr string) bool {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return network.Contains(parsed)
}

// IsIPWhitelisted 判断 IP 是否在白名单
func IsIPWhitelisted(ip string) bool {
	list := ipWhitelist()
	if len(list) == 0 || ip == "" {
		return false
	}
	clientIP := strings.TrimPrefix(ip, "::ffff:")
	for _, w := range list {
		if w == "*" {
			return true
		}
		if strings.Contains(w, "/") {
			if isIPInCIDR(clientIP, w) {
				return true
			}
			continue
		}
		if clientIP == w {
			return true
		}
	}
	return false
}

func findMatchingConfig(path string) (time.Duration, int) {
	for _, c := range rateLimitRoutes {
		if strings.HasPrefix(path, c.prefix) {
			return c.window, c.max
		}
	}
	return time.Duration(defaultWindowMs) * time.Millisecond, defaultMaxRequests
}

func cleanupExpiredRL() int {
	now := time.Now()
	deleted := 0
	for k, e := range rlStore {
		if now.After(e.resetTime) {
			delete(rlStore, k)
			deleted++
		}
	}
	return deleted
}

func evictLRU() {
	if len(rlStore) < maxStoreSize {
		return
	}
	target := maxStoreSize * 8 / 10
	toDelete := len(rlStore) - target
	// 简单选择排序找最早访问（容量上限场景低频触发）
	deleted := 0
	for deleted < toDelete {
		var oldestKey string
		var oldestTime time.Time
		first := true
		for k, e := range rlStore {
			if first || e.lastAccess.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.lastAccess
				first = false
			}
		}
		if oldestKey == "" {
			break
		}
		delete(rlStore, oldestKey)
		deleted++
	}
	logger.Warnf("Rate limiter store full, evicted %d oldest entries (LRU)", deleted)
}

// StartRateLimiterCleanup 启动限流存储定期清理
func StartRateLimiterCleanup() {
	go func() {
		ticker := time.NewTicker(cleanupIntervalRL)
		defer ticker.Stop()
		for range ticker.C {
			rlMu.Lock()
			count := cleanupExpiredRL()
			size := len(rlStore)
			rlMu.Unlock()
			if count > 0 {
				logger.Debugf("Rate limiter cleanup: removed %d expired entries (remaining: %d)", count, size)
			}
		}
	}()
}

// RateLimiter 全局限流中间件（等价 rateLimiter）
func RateLimiter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsIPWhitelisted(ClientIP(r)) {
			next.ServeHTTP(w, r)
			return
		}

		window, max := findMatchingConfig(r.URL.Path)
		key := ClientIP(r) + ":" + r.Method + ":" + r.URL.Path
		now := time.Now()

		rlMu.Lock()
		entry, ok := rlStore[key]
		if !ok || now.After(entry.resetTime) {
			if len(rlStore) >= maxStoreSize {
				if cleanupExpiredRL() == 0 {
					evictLRU()
				}
			}
			entry = &rateLimitEntry{count: 0, resetTime: now.Add(window), lastAccess: now}
			rlStore[key] = entry
		}
		entry.count++
		entry.lastAccess = now
		count := entry.count
		resetTime := entry.resetTime
		rlMu.Unlock()

		remaining := max - count
		if remaining < 0 {
			remaining = 0
		}
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(max))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

		if count > max {
			retryAfter := int(time.Until(resetTime).Seconds()) + 1
			httpx.RespondRaw(w, http.StatusTooManyRequests, map[string]interface{}{
				"success":    false,
				"message":    "请求过于频繁，请稍后再试",
				"retryAfter": retryAfter,
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// WebhookIPFilter webhook IP 白名单过滤（等价 webhookIpFilter）
func WebhookIPFilter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		list := ipWhitelist()
		if len(list) > 0 && strings.HasPrefix(r.URL.Path, "/api/v1/webhooks") {
			clientIP := ClientIP(r)
			if !IsIPWhitelisted(clientIP) {
				logger.Warnf("Webhook request blocked: IP %s not in whitelist (%s %s)", clientIP, r.Method, r.URL.Path)
				httpx.RespondFailStatus(w, http.StatusForbidden, "IP not allowed")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
