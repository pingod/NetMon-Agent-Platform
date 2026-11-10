// Package paths 提供生产环境感知的路径解析，等价于 TS 版 utils/paths.ts
package paths

import (
	"os"
	"path/filepath"
)

// IsPackaged 等价于 TS 的 NETMON_PACKAGED === 'true'
var IsPackaged = os.Getenv("NETMON_PACKAGED") == "true"

// execDir 返回可执行文件所在目录
func execDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// AppRoot 应用根目录
// - 打包模式: 可执行文件所在目录
// - 开发模式: backend-go 的上级目录(项目根)
func AppRoot() string {
	if IsPackaged {
		return execDir()
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	// 若 cwd 是 backend-go 本身，上移一级到项目根
	if filepath.Base(wd) == "backend-go" {
		return filepath.Dir(wd)
	}
	return wd
}

// FrontendDist 前端 dist 目录
func FrontendDist() string {
	if IsPackaged {
		return filepath.Join(AppRoot(), "public")
	}
	return filepath.Join(AppRoot(), "frontend", "dist")
}

// DatabasePath 数据库文件路径
func DatabasePath() string {
	if v := os.Getenv("DATABASE_PATH"); v != "" {
		// 相对路径解析
		if !filepath.IsAbs(v) {
			// .env 的 DATABASE_PATH=./data/app.db 是相对于 TS backend/ 目录的
			// Go 后端需要正确映射到共享数据库
			projectRoot := detectProjectRoot()
			if projectRoot != "" {
				// 尝试相对于项目根目录
				p1 := filepath.Join(projectRoot, v)
				if _, err := os.Stat(p1); err == nil {
					return p1
				}
				// 尝试相对于 backend/ 目录（TS 的数据库名义路径）
				p2 := filepath.Join(projectRoot, "backend", v)
				if _, err := os.Stat(p2); err == nil {
					return p2
				}
				// 文件都不存在（全新安装）：按 env 值落位创建，不再回退 backend/data/app.db
				// .env 的相对路径以 backend/ 目录为基准（与 TS 后端一致）
				backendRoot := filepath.Join(projectRoot, "backend")
				if fi, err := os.Stat(backendRoot); err == nil && fi.IsDir() {
					return filepath.Join(backendRoot, v)
				}
				return p1
			}
			return filepath.Join(AppRoot(), v)
		}
		return v
	}
	if IsPackaged {
		return filepath.Join(AppRoot(), "data", "app.db")
	}
	// 开发模式与 TS 一致: 复用 backend/data/app.db 以共享数据
	projectRoot := detectProjectRoot()
	if projectRoot != "" {
		return filepath.Join(projectRoot, "backend", "data", "app.db")
	}
	return "./data/app.db"
}

// detectProjectRoot 检测项目根目录
// 支持从任意子目录向上遍历查找项目根（标记为包含 backend-go/ 子目录或 .git/ 目录）
func detectProjectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// 若当前工作目录就是 backend-go，先上移一级
	if filepath.Base(wd) == "backend-go" {
		wd = filepath.Dir(wd)
	}

	// 从当前目录开始，逐级向上查找项目根
	for dir := wd; dir != "" && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		// 标记1: 包含 backend-go/ 子目录
		if fi, err := os.Stat(filepath.Join(dir, "backend-go")); err == nil && fi.IsDir() {
			return dir
		}
		// 标记2: 包含 .git/ 目录（git 仓库根）
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil && fi.IsDir() {
			// 确认是本项目（包含 .env 或 backend-go）
			if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
				return dir
			}
			if fi2, err := os.Stat(filepath.Join(dir, "backend-go")); err == nil && fi2.IsDir() {
				return dir
			}
		}
		// 标记3: 包含 .env 文件且有 backend/data/app.db
		if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "backend", "data", "app.db")); err == nil {
				return dir
			}
		}
	}

	return ""
}

// MetricsDatabasePath 指标数据库文件路径（metrics.db，与 app.db 同目录）
func MetricsDatabasePath() string {
	appPath := DatabasePath()
	dir := filepath.Dir(appPath)
	return filepath.Join(dir, "metrics.db")
}

// LogsDir 日志目录
func LogsDir() string {
	if IsPackaged {
		return filepath.Join(AppRoot(), "logs")
	}
	return filepath.Join(func() string {
		wd, _ := os.Getwd()
		return wd
	}(), "logs")
}

// AssetsDir 资源目录(字体等)
func AssetsDir() string {
	if IsPackaged {
		return filepath.Join(AppRoot(), "assets")
	}
	return filepath.Join(AppRoot(), "backend", "src", "assets")
}

// UploadsDir 上传目录
func UploadsDir(subdir ...string) string {
	base := os.Getenv("UPLOAD_DIR")
	if base == "" {
		if IsPackaged {
			base = filepath.Join(AppRoot(), "data", "uploads")
		} else {
			base = filepath.Join(AppRoot(), "backend", "data", "uploads")
		}
	}
	if len(subdir) > 0 && subdir[0] != "" {
		return filepath.Join(base, subdir[0])
	}
	return base
}

// BackendDataDir 后端数据目录(备份文件等)，等价于 TS 的 backend/data 或打包 data/
func BackendDataDir() string {
	if IsPackaged {
		return filepath.Join(AppRoot(), "data")
	}
	if wd, err := os.Getwd(); err == nil && filepath.Base(wd) == "backend-go" {
		return filepath.Join(filepath.Dir(wd), "backend", "data")
	}
	return "./data"
}

// StaticDir 前端静态资源目录（SPA 回退使用）：优先 STATIC_DIR，其次 FrontendDist
func StaticDir() string {
	if v := os.Getenv("STATIC_DIR"); v != "" {
		if filepath.IsAbs(v) {
			return v
		}
		return filepath.Join(AppRoot(), v)
	}
	return FrontendDist()
}
