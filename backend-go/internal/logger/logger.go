// Package logger 等价于 TS utils/logger.ts 的核心功能
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"itops-backend/internal/config"
	"itops-backend/internal/paths"
)

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

func (l Level) String() string {
	switch l {
	case Debug:
		return "debug"
	case Info:
		return "info"
	case Warn:
		return "warn"
	default:
		return "error"
	}
}

func parseLevel(s string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return Debug, true
	case "info":
		return Info, true
	case "warn", "warning":
		return Warn, true
	case "error":
		return Error, true
	}
	return Info, false
}

// Entry 日志条目(供 /logs 端点读取)
type Entry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Service   string `json:"service"`
}

// Stats 日志统计
type Stats struct {
	Total    int64            `json:"total"`
	ByLevel  map[string]int64 `json:"byLevel"`
	Errors   int64            `json:"errors"`
	Warnings int64            `json:"warnings"`
	LastHour int64            `json:"lastHour"`
	LastDay  int64            `json:"lastDay"`
}

type Logger struct {
	mu      sync.Mutex
	level   Level
	service string
	stats   Stats
	times   []time.Time
	recent  []Entry
	logFile *os.File
	errFile *os.File
	logDate string
	onError []func(Entry)
}

// Init 在配置加载完成后初始化日志（打开当日文件，跨天自动轮转重开）。
// cfg 可为空（仅按环境变量清理），兼容 main 中的 logger.Init(cfg) 与 logger.Init()。
func Init(cfg ...*config.Config) {
	if len(cfg) > 0 && cfg[0] != nil {
		if lv, ok := parseLevel(cfg[0].LogLevel); ok {
			std.level = lv
		} else if cfg[0].NodeEnv == "production" {
			std.level = Info
		}
	}
	std.mu.Lock()
	defer std.mu.Unlock()
	std.openFilesLocked(true)
}

// setupFiles 打开当前日期的日志文件
func (l *Logger) setupFiles() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.openFilesLocked(true)
}

// openFilesLocked 按当前日期打开日志文件；force=false 且日期未变时复用已有文件句柄
func (l *Logger) openFilesLocked(force bool) {
	date := time.Now().Format("2006-01-02")
	if !force && l.logDate == date && l.logFile != nil {
		return
	}
	dir := paths.LogsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	if l.logFile != nil {
		_ = l.logFile.Close()
		l.logFile = nil
	}
	if l.errFile != nil {
		_ = l.errFile.Close()
		l.errFile = nil
	}
	l.logDate = date
	if f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%s-%s.log", l.service, date)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		l.logFile = f
	}
	if f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%s-error-%s.log", l.service, date)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		l.errFile = f
	}
}

var std = newLogger("itops-agent")

func newLogger(service string) *Logger {
	l := &Logger{
		service: service,
		level:   Debug,
		stats: Stats{
			ByLevel: map[string]int64{"debug": 0, "info": 0, "warn": 0, "error": 0},
		},
	}
	if os.Getenv("NODE_ENV") != "production" {
		l.level = Debug
	} else {
		l.level = Info
	}
	if lv, ok := parseLevel(os.Getenv("LOG_LEVEL")); ok {
		l.level = lv
	}
	if os.Getenv("NODE_ENV") == "production" {
		l.setupFiles()
	}
	return l
}

// OnError 注册错误日志监听器(自监控服务使用)
func OnError(fn func(Entry)) {
	std.mu.Lock()
	defer std.mu.Unlock()
	std.onError = append(std.onError, fn)
}

func (l *Logger) log(lv Level, msg string, args ...any) {
	if lv < l.level {
		return
	}
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	now := time.Now()
	ts := now.Format("2006-01-02T15:04:05.000Z07:00")

	entry := Entry{Timestamp: ts, Level: lv.String(), Message: msg, Service: l.service}

	l.mu.Lock()
	l.stats.Total++
	l.stats.ByLevel[lv.String()]++
	if lv == Error {
		l.stats.Errors++
	}
	if lv == Warn {
		l.stats.Warnings++
	}
	l.times = append(l.times, now)
	if len(l.times) > 20000 {
		l.times = l.times[len(l.times)-20000:]
	}
	l.recent = append(l.recent, entry)
	if len(l.recent) > 1000 {
		l.recent = l.recent[len(l.recent)-1000:]
	}
	l.openFilesLocked(false)
	logFile, errFile := l.logFile, l.errFile
	errListeners := make([]func(Entry), len(l.onError))
	copy(errListeners, l.onError)
	l.mu.Unlock()

	line := fmt.Sprintf("[%s] [%s] [%s] %s\n", ts, strings.ToUpper(lv.String()), l.service, msg)
	switch lv {
	case Error:
		fmt.Fprint(os.Stderr, line)
	default:
		fmt.Fprint(os.Stdout, line)
	}
	if logFile != nil {
		_, _ = logFile.WriteString(line)
	}
	if lv == Error && errFile != nil {
		_, _ = errFile.WriteString(line)
	}
	if lv == Error {
		for _, fn := range errListeners {
			fn(entry)
		}
	}
}

func Debugf(msg string, args ...any) { std.log(Debug, msg, args...) }
func Infof(msg string, args ...any)  { std.log(Info, msg, args...) }
func Warnf(msg string, args ...any)  { std.log(Warn, msg, args...) }
func Errorf(msg string, args ...any) { std.log(Error, msg, args...) }

// GetStats 返回日志统计
func GetStats() Stats {
	std.mu.Lock()
	defer std.mu.Unlock()
	hourAgo := time.Now().Add(-time.Hour)
	dayAgo := time.Now().Add(-24 * time.Hour)
	var lastHour, lastDay int64
	for _, t := range std.times {
		if t.After(hourAgo) {
			lastHour++
		}
		if t.After(dayAgo) {
			lastDay++
		}
	}
	return Stats{
		Total:    std.stats.Total,
		ByLevel:  std.stats.ByLevel,
		Errors:   std.stats.Errors,
		Warnings: std.stats.Warnings,
		LastHour: lastHour,
		LastDay:  lastDay,
	}
}

// Recent 返回最近 n 条日志
func Recent(n int) []Entry {
	std.mu.Lock()
	defer std.mu.Unlock()
	if n <= 0 || n > len(std.recent) {
		n = len(std.recent)
	}
	out := make([]Entry, n)
	copy(out, std.recent[len(std.recent)-n:])
	return out
}

// SetLevel 动态设置日志级别
func SetLevel(s string) bool {
	if lv, ok := parseLevel(s); ok {
		std.mu.Lock()
		std.level = lv
		std.mu.Unlock()
		return true
	}
	return false
}

// GetLevel 当前日志级别
func GetLevel() string {
	std.mu.Lock()
	defer std.mu.Unlock()
	return std.level.String()
}
