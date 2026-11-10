// Package config 等价于 TS utils/env.ts 的环境配置加载
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"itops-backend/internal/paths"
)

type Config struct {
	NodeEnv             string
	Port                int
	DatabasePath        string
	MetricsDatabasePath string
	LogLevel            string
	JWTSecret           string
	JWTExpiresIn        string
	AllowedOrigins      []string

	// 加密密钥（用于 PBKDF2 派生 AES 密钥）；未设置时回退到 JWT_SECRET
	EncryptionKey string

	// AuthDevBypass 仅用于开发环境：未认证请求注入 admin（生产环境强制关闭）
	AuthDevBypass bool

	// TrustedProxies 可信反向代理 IP/CIDR 列表；仅当 RemoteAddr 命中时才信任 XFF/X-Real-IP
	TrustedProxies []string

	// MySQL 可选后端（业务与常用元数据存储）
	MySQLEnabled bool
	MySQLDSN     string // user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local

	// InfluxDB 可选时序库后端（网络设备与指标时序数据存储）
	InfluxdbEnabled           bool
	InfluxdbVersion           string // "v1" | "v2" | "v3"
	InfluxdbHost              string
	InfluxdbPort              int
	InfluxdbToken             string
	InfluxdbOrg               string
	InfluxdbBucket            string
	InfluxdbUsername          string
	InfluxdbPassword          string
	InfluxdbDatabase          string
	InfluxdbRetentionPolicy   string
	InfluxdbMeasurementPrefix string
	InfluxdbWriteMode         string // "async" | "sync"
	InfluxdbBatchSize         int
	InfluxdbFlushIntervalMs   int

	DoubaoAPIKey   string
	DoubaoAPIBase  string
	DoubaoModel    string
	OpenAIAPIKey   string
	OpenAIAPIBase  string
	OpenAIModel    string
	LocalAIAPIKey  string
	LocalAIAPIBase string
	LocalAIModel   string

	WebhookVerifyEnabled string // "true" | "false" | "warn"
	WebhookSecret        string
	WebhookIPWhitelist   string

	AlertWebhookURL string
	AlertEmailHost  string
	AlertEmailPort  int
	AlertEmailUser  string
	AlertEmailPass  string
	AlertEmailTo    string
}

func (c *Config) IsProduction() bool { return c.NodeEnv == "production" }

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func getEnvNum(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

// parseEnvBool 解析布尔型环境变量，兼容 true/1/yes/on/false/0/no/off。
func parseEnvBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	}
	if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
		return b
	}
	return def
}

// parseList 解析逗号分隔列表，去除空白与空项。
func parseList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseWebhookVerifyMode(v string) string {
	if v == "" {
		return "warn"
	}
	n := strings.ToLower(strings.TrimSpace(v))
	switch n {
	case "false", "0", "off", "no":
		return "false"
	case "warn":
		return "warn"
	default:
		return "true"
	}
}

// LoadDotEnv 简易 .env 文件加载(等价 dotenv.config())
func LoadDotEnv(file ...string) {
	paths := []string{".env", "../.env", "../../.env"}
	if len(file) > 0 {
		paths = []string{file[0]}
	}
	var data []byte
	var found bool
	for _, p := range paths {
		var err error
		data, err = os.ReadFile(p)
		if err == nil {
			found = true
			break
		}
	}
	if !found {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

var placeholderSecrets = map[string]bool{
	"itops-agent-platform-secret-key-change-in-production": true,
	"your-production-secret-key-change-me-32-chars":        true,
	"please_change_this_to_a_random_64_char_string":        true,
	"change-me":  true,
	"secret":     true,
	"jwt-secret": true,
}

// Load 加载并校验环境配置，等价 validateEnv()
func Load() (*Config, error) {
	isProd := os.Getenv("NODE_ENV") == "production"
	jwtSecret := os.Getenv("JWT_SECRET")

	if isProd && jwtSecret == "" {
		return nil, fmt.Errorf("missing required environment variable: JWT_SECRET (must be set in production)")
	}
	if jwtSecret == "" {
		fmt.Println("⚠️ Missing recommended environment variable: JWT_SECRET (using insecure default, DO NOT use in production)")
	}

	trimmedSecret := strings.ToLower(strings.TrimSpace(jwtSecret))
	if jwtSecret != "" && (placeholderSecrets[trimmedSecret] || strings.HasPrefix(trimmedSecret, "itops-agent-platform-secret")) {
		if isProd {
			return nil, fmt.Errorf("cannot use placeholder JWT_SECRET in production! Please set a secure random secret (openssl rand -hex 32)")
		}
		fmt.Println("⚠️ WARNING: Using placeholder JWT secret. This is INSECURE and should ONLY be used for development!")
	}

	if isProd && jwtSecret != "" && len(jwtSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters in production. Generate with: openssl rand -hex 32")
	}

	if jwtSecret == "" {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		jwtSecret = hex.EncodeToString(b)
		fmt.Println("⚠️ WARNING: Using a randomly generated JWT secret for this session.")
		fmt.Println("   This means all tokens will be invalidated on server restart.")
		fmt.Println("   Set JWT_SECRET in your .env file for persistent sessions.")
	}

	return &Config{
		NodeEnv:             getEnv("NODE_ENV", "development"),
		Port:                getEnvNum("PORT", 8088),
		DatabasePath:        paths.DatabasePath(),
		MetricsDatabasePath: paths.MetricsDatabasePath(),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		JWTSecret:           jwtSecret,
		JWTExpiresIn:        getEnv("JWT_EXPIRES_IN", "24h"),
		AllowedOrigins:      parseList(getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:8088")),
		EncryptionKey:       os.Getenv("ENCRYPTION_KEY"),
		AuthDevBypass:       parseEnvBool("AUTH_DEV_BYPASS", false),
		TrustedProxies:      parseList(getEnv("TRUSTED_PROXIES", "")),

		// MySQL 可选后端（业务与常用元数据存储）
		MySQLEnabled: parseEnvBool("MYSQL_ENABLED", false),
		MySQLDSN:     os.Getenv("MYSQL_DSN"),

		// InfluxDB 可选时序库后端（网络设备与指标时序数据存储）
		InfluxdbEnabled:           parseEnvBool("INFLUXDB_ENABLED", false),
		InfluxdbVersion:           getEnv("INFLUXDB_VERSION", "v2"),
		InfluxdbHost:              getEnv("INFLUXDB_HOST", "127.0.0.1"),
		InfluxdbPort:              getEnvNum("INFLUXDB_PORT", 8086),
		InfluxdbToken:             os.Getenv("INFLUXDB_TOKEN"),
		InfluxdbOrg:               getEnv("INFLUXDB_ORG", "itops"),
		InfluxdbBucket:            getEnv("INFLUXDB_BUCKET", "netmon_metrics"),
		InfluxdbUsername:          getEnv("INFLUXDB_USERNAME", "root"),
		InfluxdbPassword:          os.Getenv("INFLUXDB_PASSWORD"),
		InfluxdbDatabase:          getEnv("INFLUXDB_DATABASE", "netmon_metrics"),
		InfluxdbRetentionPolicy:   getEnv("INFLUXDB_RETENTION_POLICY", "autogen"),
		InfluxdbMeasurementPrefix: getEnv("INFLUXDB_MEASUREMENT_PREFIX", "snmp"),
		InfluxdbWriteMode:         getEnv("INFLUXDB_WRITE_MODE", "async"),
		InfluxdbBatchSize:         getEnvNum("INFLUXDB_BATCH_SIZE", 100),
		InfluxdbFlushIntervalMs:   getEnvNum("INFLUXDB_FLUSH_INTERVAL_MS", 3000),

		DoubaoAPIKey:   os.Getenv("DOUBAO_API_KEY"),
		DoubaoAPIBase:  os.Getenv("DOUBAO_API_BASE"),
		DoubaoModel:    os.Getenv("DOUBAO_MODEL"),
		OpenAIAPIKey:   os.Getenv("OPENAI_API_KEY"),
		OpenAIAPIBase:  os.Getenv("OPENAI_API_BASE"),
		OpenAIModel:    os.Getenv("OPENAI_MODEL"),
		LocalAIAPIKey:  os.Getenv("LOCAL_AI_API_KEY"),
		LocalAIAPIBase: os.Getenv("LOCAL_AI_API_BASE"),
		LocalAIModel:   os.Getenv("LOCAL_AI_MODEL"),

		WebhookVerifyEnabled: parseWebhookVerifyMode(os.Getenv("WEBHOOK_VERIFY_ENABLED")),
		WebhookSecret:        os.Getenv("WEBHOOK_SECRET"),
		WebhookIPWhitelist:   os.Getenv("WEBHOOK_IP_WHITELIST"),

		AlertWebhookURL: os.Getenv("ALERT_WEBHOOK_URL"),
		AlertEmailHost:  os.Getenv("ALERT_EMAIL_HOST"),
		AlertEmailPort:  getEnvNum("ALERT_EMAIL_PORT", 587),
		AlertEmailUser:  os.Getenv("ALERT_EMAIL_USER"),
		AlertEmailPass:  os.Getenv("ALERT_EMAIL_PASS"),
		AlertEmailTo:    os.Getenv("ALERT_EMAIL_TO"),
	}, nil
}

// Env 全局配置单例
var Env *Config

// ParseDuration 解析 JWT 过期时间(支持 24h/7d/30m 等)
func ParseDuration(s string) (dur int64, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		// 尝试 Go 原生格式
		d, e2 := time.ParseDuration(s)
		if e2 != nil {
			return 0, fmt.Errorf("invalid duration: %s", s)
		}
		return int64(d.Seconds()), nil
	}
	switch unit {
	case 's':
		return n, nil
	case 'm':
		return n * 60, nil
	case 'h':
		return n * 3600, nil
	case 'd':
		return n * 86400, nil
	case 'w':
		return n * 7 * 86400, nil
	default:
		// 整体当作秒
		if n2, e := strconv.ParseInt(s, 10, 64); e == nil {
			return n2, nil
		}
		return 0, fmt.Errorf("invalid duration: %s", s)
	}
}
