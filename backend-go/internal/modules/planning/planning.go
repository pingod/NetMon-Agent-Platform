// Package planning 将 IDC 上架布线引擎（原 IDCBoxLine）作为受管子进程接入 NetMon。
//
// 设计取舍：引擎的域层（25k 行 Python + 1328 个用例 + 导出逐格铁线）是
// 经真实客户数据反解并验证过的资产，重写成 Go 会同时丢掉验证与公式口径。
// 因此这里只做三件事：进程监管、认证反向代理、静态资源嵌入，
// 让规划能力成为 NetMon 的一等模块，而不是第二个需要用户单独启动的应用。
package planning

import (
	"bufio"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
)

const (
	defaultPort    = 18001
	readyTimeout   = 90 * time.Second
	restartMinWait = 1 * time.Second
	restartMaxWait = 30 * time.Second
)

//go:embed dist
var distFS embed.FS

// Config 规划引擎运行参数（全部可由环境变量覆盖）。
type Config struct {
	Enabled   bool
	Root      string // 规划模块根目录（含 backend/ data/ projects/）
	EngineDir string // Python 包所在目录（默认 Root/backend）
	Python    string
	Port      int
	DBPath    string
	StaticDir string
	// Bundled 表示使用绿色包内含的独立 Python 运行时（靠 PYTHONPATH 指向 vendor）。
	Bundled bool
}

// Manager 管理引擎子进程、代理与状态。
type Manager struct {
	cfg       Config
	proxy     *httputil.ReverseProxy
	static    http.Handler
	transport *http.Transport

	mu      sync.RWMutex
	cmd     *exec.Cmd
	state   string // disabled | starting | up | restarting | stopped
	lastErr string

	stopping atomic.Bool
	stopCh   chan struct{}
	doneCh   chan struct{}
}

var defaultManager *Manager

// Start 解析配置并在后台拉起规划引擎；失败不会阻塞主服务启动。
func Start() {
	cfg, err := loadConfig()
	if err != nil {
		logger.Warnf("Planning module disabled: %v", err)
		return
	}
	m := newManager(cfg)
	defaultManager = m
	if !cfg.Enabled {
		m.setState("disabled")
		logger.Infof("Planning module disabled by PLANNING_ENABLED")
		return
	}
	logger.Infof("Planning engine starting (root=%s, port=%d)", cfg.Root, cfg.Port)
	go m.supervise()
}

// Stop 终止子进程并等待退出。
func Stop() {
	if defaultManager == nil {
		return
	}
	defaultManager.shutdown()
}

func newManager(cfg Config) *Manager {
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Port))}
	m := &Manager{
		cfg:    cfg,
		state:  "starting",
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
		transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          32,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: 120 * time.Second,
		},
	}
	m.proxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// chi.Mount 保留 r.URL.Path 原值，只改 RoutePath；这里显式剥离前缀，
			// 保证 /api/v1/planning/api/meta → 引擎 /api/meta。
			req.URL.Path = enginePath(req.URL.Path)
			req.URL.RawPath = ""
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			// 引擎无会话概念，认证在 NetMon 侧完成；隔离掉外部伪造头。
			req.Header.Del("X-NetMon-User")
		},
		Transport:     m.transport,
		FlushInterval: 100 * time.Millisecond,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			m.setError(err.Error())
			httpx.RespondError(w, "规划引擎请求失败: "+err.Error(), http.StatusBadGateway)
		},
	}
	m.static = buildStaticHandler(cfg.StaticDir)
	return m
}

// Routes 返回挂在 /api/v1/planning 下的代理子路由。
//
// 读接口对所有已登录用户开放；写接口与主平台一致，仅 admin/operator 可执行。
func Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/*", proxyHandler)
	r.Get("/", proxyHandler)
	write := middleware.RequireRole("admin", "operator")
	r.With(write).Post("/*", proxyHandler)
	r.With(write).Put("/*", proxyHandler)
	r.With(write).Patch("/*", proxyHandler)
	r.With(write).Delete("/*", proxyHandler)
	return r
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	m := defaultManager
	if m == nil {
		httpx.RespondError(w, "规划模块未启用：未找到 planning 引擎目录", http.StatusServiceUnavailable)
		return
	}
	m.mu.RLock()
	state := m.state
	lastErr := m.lastErr
	m.mu.RUnlock()
	if state != "up" {
		msg := "规划引擎正在启动"
		if state == "stopped" || state == "disabled" {
			msg = "规划引擎未运行"
		}
		if lastErr != "" {
			msg += "：" + lastErr
		}
		httpx.RespondError(w, msg, http.StatusServiceUnavailable)
		return
	}
	m.proxy.ServeHTTP(w, r)
}

// StaticHandler 提供嵌入的规划前端；资源无需鉴权（数据接口仍受 JWT 保护）。
func StaticHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := defaultManager
		var h http.Handler
		if m != nil {
			h = m.static
		} else {
			h = buildStaticHandler("")
		}
		if r.URL.Path == "/planning-app" {
			http.Redirect(w, r, "/planning-app/", http.StatusMovedPermanently)
			return
		}
		h.ServeHTTP(w, r)
	}
}

// StatusHandler 供门户探活。
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	if defaultManager == nil {
		httpx.Respond(w, map[string]any{
			"enabled": false,
			"state":   "disabled",
			"error":   "未找到 planning 引擎目录",
		})
		return
	}
	m := defaultManager
	m.mu.RLock()
	defer m.mu.RUnlock()
	httpx.Respond(w, map[string]any{
		"enabled": m.cfg.Enabled,
		"state":   m.state,
		"error":   m.lastErr,
		"port":    m.cfg.Port,
		"root":    m.cfg.Root,
	})
}

func buildStaticHandler(dir string) http.Handler {
	var fileSystem http.FileSystem
	if dir != "" {
		fileSystem = http.Dir(dir)
	} else {
		sub, err := fs.Sub(distFS, "dist")
		if err == nil {
			if _, err := fs.Stat(sub, "index.html"); err == nil {
				fileSystem = http.FS(sub)
			}
		}
	}
	if fileSystem == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `<!doctype html><meta charset="utf-8"><title>规划前端未构建</title>
<body style="font-family:system-ui;padding:48px">
<h1>规划前端未构建</h1>
<p>请执行 <code>build_release.sh</code>（或在 <code>planning/web</code> 执行 <code>pnpm build</code> 并同步 dist）后重新编译，或把 <code>PLANNING_STATIC_DIR</code> 指向 <code>planning/web/dist</code>。</p>
</body>`)
		})
	}
	fileServer := http.FileServer(fileSystem)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/planning-app")
		if path == "" {
			path = "/"
		}
		if fi, err := fileSystem.Open(path); err == nil {
			_ = fi.Close()
			if strings.HasPrefix(path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = path
			fileServer.ServeHTTP(w, r2)
			return
		}
		// 缺失的带扩展名资源必须 404，不能回退成 index.html（浏览器会得到 HTML 当 JS 解析）
		if strings.Contains(filepath.Base(path), ".") {
			http.NotFound(w, r)
			return
		}
		// SPA 回退：规划应用使用 hash 路由，仅需 index.html
		w.Header().Set("Cache-Control", "no-cache")
		if f, err := fileSystem.Open("/index.html"); err == nil {
			defer f.Close()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.Copy(w, f)
			return
		}
		http.NotFound(w, r)
	})
}

// ---------------------------------------------------------------- 配置

func loadConfig() (Config, error) {
	cfg := Config{
		Enabled: envBool("PLANNING_ENABLED", true),
		Port:    envInt("PLANNING_PORT", defaultPort),
		DBPath:  os.Getenv("PLANNING_DB"),
	}
	if !cfg.Enabled {
		// 显式关闭时不再探测目录/解释器，避免把「用户主动关闭」误报成配置错误。
		return cfg, nil
	}
	cfg.Root = os.Getenv("PLANNING_ROOT")
	if cfg.Root == "" {
		root, err := findRoot()
		if err != nil {
			return cfg, err
		}
		cfg.Root = root
	}
	cfg.EngineDir = os.Getenv("PLANNING_ENGINE_DIR")
	if cfg.EngineDir == "" {
		cfg.EngineDir = filepath.Join(cfg.Root, "backend")
	}
	if _, err := os.Stat(filepath.Join(cfg.EngineDir, "idcbox", "api", "main.py")); err != nil {
		return cfg, fmt.Errorf("未找到规划引擎源码: %s", filepath.Join(cfg.EngineDir, "idcbox"))
	}
	cfg.Python = os.Getenv("PLANNING_PYTHON")
	if cfg.Python == "" {
		var bundled bool
		cfg.Python, bundled = findPython(cfg.Root, cfg.EngineDir)
		cfg.Bundled = bundled
	}
	if cfg.Python == "" {
		return cfg, errors.New("未找到 Python 解释器（可用 PLANNING_PYTHON 指定）")
	}
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.Root, "data", "idcbox.db")
	}
	if err := ensureDatabase(cfg); err != nil {
		return cfg, err
	}
	cfg.StaticDir = os.Getenv("PLANNING_STATIC_DIR")
	cfg.Port = pickPort(cfg.Port)
	return cfg, nil
}

func findRoot() (string, error) {
	candidates := []string{}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "planning"), filepath.Join(wd, "..", "planning"))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, "planning"), filepath.Join(dir, "..", "planning"))
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			if st, err := os.Stat(filepath.Join(abs, "backend", "idcbox")); err == nil && st.IsDir() {
				return abs, nil
			}
		}
	}
	return "", errors.New("未找到 planning/ 目录（可用 PLANNING_ROOT 指定）")
}

func findPython(root, engineDir string) (string, bool) {
	// 绿色包优先：自带解释器 + vendor 依赖，现场无需安装 Python。
	bundled := []string{
		filepath.Join(root, "runtime", "linux", "python", "bin", "python3"),
		filepath.Join(root, "runtime", "windows", "python", "python.exe"),
	}
	for _, c := range bundled {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, true
		}
	}
	candidates := []string{
		filepath.Join(engineDir, ".venv", "bin", "python"),
		filepath.Join(engineDir, ".venv", "Scripts", "python.exe"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, false
		}
	}
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, false
		}
	}
	return "", false
}

// ensureDatabase 首次运行时从模块内的种子库复制出独立运行库，避免直接改动随包资产。
func ensureDatabase(cfg Config) error {
	if _, err := os.Stat(cfg.DBPath); err == nil {
		return nil
	}
	seed := filepath.Join(cfg.Root, "data", "idcbox.db")
	if cfg.DBPath == seed {
		return nil
	}
	if _, err := os.Stat(seed); err != nil {
		return nil // 没有种子也能启动；引擎会建空库
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return fmt.Errorf("创建规划数据目录失败: %w", err)
	}
	src, err := os.Open(seed)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(cfg.DBPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("初始化规划数据库失败: %w", err)
	}
	logger.Infof("Planning database seeded: %s", cfg.DBPath)
	return nil
}

func pickPort(preferred int) int {
	for p := preferred; p < preferred+20; p++ {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
		if err == nil {
			_ = ln.Close()
			return p
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err == nil {
		defer ln.Close()
		if addr, ok := ln.Addr().(*net.TCPAddr); ok {
			return addr.Port
		}
	}
	return preferred
}

// ---------------------------------------------------------------- 进程监管

func (m *Manager) supervise() {
	defer close(m.doneCh)
	wait := restartMinWait
	for {
		select {
		case <-m.stopCh:
			return
		default:
		}
		err := m.runOnce()
		if m.stopping.Load() {
			return
		}
		if err != nil {
			m.setError(err.Error())
			logger.Warnf("Planning engine exited: %v (retry in %s)", err, wait)
		} else {
			logger.Warnf("Planning engine exited unexpectedly (retry in %s)", wait)
		}
		m.setState("restarting")
		select {
		case <-m.stopCh:
			return
		case <-time.After(wait):
		}
		if wait < restartMaxWait {
			wait *= 2
		}
	}
}

func (m *Manager) runOnce() error {
	cmd := exec.Command(m.cfg.Python,
		"-m", "uvicorn", "idcbox.api.main:create_app",
		"--factory",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(m.cfg.Port),
		"--log-level", "warning",
	)
	cmd.Dir = m.cfg.EngineDir
	cmd.Env = engineEnv(m.cfg)
	cmd.Stdout = &logWriter{prefix: "[planning] "}
	cmd.Stderr = &logWriter{prefix: "[planning] "}
	m.mu.Lock()
	m.cmd = cmd
	m.state = "starting"
	m.lastErr = ""
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动失败: %w", err)
	}
	if err := m.waitReady(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	m.setState("up")
	logger.Infof("Planning engine ready at http://127.0.0.1:%d", m.cfg.Port)

	err := cmd.Wait()
	m.mu.Lock()
	m.cmd = nil
	m.mu.Unlock()
	return err
}

func (m *Manager) waitReady() error {
	deadline := time.Now().Add(readyTimeout)
	client := &http.Client{Timeout: 2 * time.Second}
	health := fmt.Sprintf("http://127.0.0.1:%d/api/health", m.cfg.Port)
	var last error
	for time.Now().Before(deadline) {
		select {
		case <-m.stopCh:
			return errors.New("已停止")
		case <-time.After(300 * time.Millisecond):
		}
		resp, err := client.Get(health)
		if err != nil {
			last = err
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
		last = fmt.Errorf("健康检查返回 %d", resp.StatusCode)
	}
	if last == nil {
		last = errors.New("启动超时")
	}
	return fmt.Errorf("规划引擎未就绪: %w", last)
}

func (m *Manager) setState(s string) {
	m.mu.Lock()
	m.state = s
	m.mu.Unlock()
}

func (m *Manager) setError(msg string) {
	m.mu.Lock()
	m.lastErr = msg
	if m.state == "up" {
		m.state = "restarting"
	}
	m.mu.Unlock()
}

func (m *Manager) shutdown() {
	if m.stopping.Swap(true) {
		return
	}
	close(m.stopCh)
	m.mu.Lock()
	cmd := m.cmd
	m.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
		time.Sleep(500 * time.Millisecond)
		_ = cmd.Process.Kill()
	}
	select {
	case <-m.doneCh:
	case <-time.After(5 * time.Second):
	}
	m.transport.CloseIdleConnections()
	m.setState("stopped")
}

func engineEnv(cfg Config) []string {
	env := make([]string, 0, len(os.Environ())+6)
	for _, kv := range os.Environ() {
		key := strings.SplitN(kv, "=", 2)[0]
		switch key {
		case "PYTHONHOME", "PYTHONPATH":
			continue // 与 IDCBox run.sh 一致：污染的解释器路径必须剔除
		}
		env = append(env, kv)
	}
	env = append(env,
		"PYTHONUNBUFFERED=1",
		"IDCBOX_DB="+cfg.DBPath,
		"IDCBOX_HOST=127.0.0.1",
		"IDCBOX_PORT="+strconv.Itoa(cfg.Port),
	)
	if cfg.Bundled {
		// 绿色包布局：解释器在 runtime/，依赖在 vendor/python，源码在 backend/。
		pythonPath := filepath.Join(cfg.Root, "vendor", "python") + string(os.PathListSeparator) + cfg.EngineDir
		env = append(env, "PYTHONHOME=", "PYTHONPATH="+pythonPath)
	}
	return env
}

type logWriter struct{ prefix string }

func (w *logWriter) Write(p []byte) (int, error) {
	s := bufio.NewScanner(strings.NewReader(string(p)))
	for s.Scan() {
		if line := strings.TrimRight(s.Text(), "\r\n"); line != "" {
			logger.Infof("%s%s", w.prefix, line)
		}
	}
	return len(p), nil
}

// enginePath 把 NetMon 的挂载前缀剥掉，得到规划引擎的真实路径。
func enginePath(p string) string {
	out := strings.TrimPrefix(p, "/api/v1/planning")
	if out == "" {
		return "/"
	}
	if !strings.HasPrefix(out, "/") {
		return "/" + out
	}
	return out
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

// MarshalStatus 便于测试/诊断。
func MarshalStatus() []byte {
	b, _ := json.Marshal(map[string]any{"manager": defaultManager != nil})
	return b
}
