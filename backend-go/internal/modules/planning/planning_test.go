package planning

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnginePath(t *testing.T) {
	cases := map[string]string{
		"/api/v1/planning/api/meta":         "/api/meta",
		"/api/v1/planning/api/cables":       "/api/cables",
		"/api/v1/planning":                  "/",
		"/api/v1/planning/":                 "/",
		"/api/v1/planning/api/intent/check": "/api/intent/check",
		"/api/meta":                         "/api/meta", // 已剥离时保持幂等
	}
	for in, want := range cases {
		if got := enginePath(in); got != want {
			t.Errorf("enginePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEngineEnvBundledUsesVendor(t *testing.T) {
	t.Setenv("PYTHONHOME", "/bad/home")
	t.Setenv("PYTHONPATH", "/bad/path")
	cfg := Config{Root: "/opt/planning", EngineDir: "/opt/planning/backend", Bundled: true}
	env := engineEnv(cfg)
	if hasEnv(env, "PYTHONHOME=/bad/home") || hasEnv(env, "PYTHONPATH=/bad/path") {
		t.Fatal("污染的解释器路径必须被剔除")
	}
	want := "PYTHONPATH=/opt/planning/vendor/python" + string(os.PathListSeparator) + "/opt/planning/backend"
	if !hasEnv(env, want) {
		t.Fatalf("bundled PYTHONPATH 缺失: %q", want)
	}
}

func TestEngineEnvVenvKeepsPythonPathClean(t *testing.T) {
	t.Setenv("PYTHONPATH", "/bad/path")
	cfg := Config{Root: "/opt/planning", EngineDir: "/opt/planning/backend"}
	env := engineEnv(cfg)
	if hasEnv(env, "PYTHONPATH=/bad/path") {
		t.Fatal("venv 模式不应继承 PYTHONPATH")
	}
	if hasEnvPrefix(env, "PYTHONPATH=") {
		t.Fatal("venv 模式不应设置 PYTHONPATH")
	}
}

func TestStaticHandlerServesIndexAndAssets(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("INDEX"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("APP"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := buildStaticHandler(dir)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/planning-app/assets/app.js", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "APP" {
		t.Fatalf("asset: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("asset 应强缓存，got %q", got)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/planning-app/design", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "INDEX" {
		t.Fatalf("SPA fallback: code=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/planning-app/assets/missing.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("缺失资源应 404，got %d", rec.Code)
	}
}

func TestProxyReturns503WhenEngineDisabled(t *testing.T) {
	old := defaultManager
	defaultManager = nil
	defer func() { defaultManager = old }()

	rec := httptest.NewRecorder()
	proxyHandler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/planning/api/meta", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "规划模块未启用") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func hasEnv(env []string, target string) bool {
	for _, kv := range env {
		if kv == target {
			return true
		}
	}
	return false
}

func hasEnvPrefix(env []string, prefix string) bool {
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}
	return false
}
