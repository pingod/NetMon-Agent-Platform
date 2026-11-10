// 路由注册 — 等价于 TS modules/_registry.ts
// 所有模块路由在此统一注册，构成完整的 HTTP 路由树
package modules

import (
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"

	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
	authmod "itops-backend/internal/modules/auth"
	dbroutes "itops-backend/internal/modules/database/routes"
	dcroutes "itops-backend/internal/modules/dc/routes"
	"itops-backend/internal/modules/inspection"
	"itops-backend/internal/modules/network/routes"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/modules/ops"
	srvroutes "itops-backend/internal/modules/server/routes"
	"itops-backend/internal/modules/copilot"
)

// NewRouter 构建完整的 HTTP 路由
func NewRouter() chi.Router {
	r := chi.NewRouter()

	// ── 全局中间件 ──
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Compress(5))

	// CORS
	corsOpts := cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Refresh-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}
	r.Use(cors.New(corsOpts).Handler)

	// ── 健康检查（无需认证） ──
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// ── WebSocket 终端（token 通过 URL query 参数验证） ──
	// 必须在 SPA 回退之前注册，否则会被 /* 拦截
	r.Get("/ws/terminal", services.HandleTerminalWebSocket)
	r.Get("/ws/vnc", services.HandleVncWebSocket)

	// ── 公开路由（无需认证） ──
	r.Group(func(r chi.Router) {
		r.Mount("/api/v1/auth", authmod.Routes())
	})

	// ── 认证中间件之后的受保护路由 ──
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate)

		// ── 网络设备模块 ──
		r.Mount("/api/v1/network-devices", routes.NetworkDeviceRoutes())
		r.Mount("/api/v1/snmp", routes.SnmpRoutes())
		r.Mount("/api/v1/snmp-collect", routes.SnmpCollectRoutes())
		r.Mount("/api/v1/config-backup", routes.ConfigBackupRoutes())
		r.Mount("/api/v1/network-advanced", routes.NetworkAdvancedRoutes())
		r.Mount("/api/v1/vnc", routes.VncRoutes())
		r.Mount("/api/v1/topology", routes.TopologyRoutes())
		r.Mount("/api/v1/network-subnets", routes.NetworkSubnetRoutes())
		r.Mount("/api/v1/network-discovery", routes.NetworkDiscoveryRoutes())

		// ── 服务器模块（SSH密钥/凭证管理） ──
		r.Mount("/api/v1/ssh-keys", routes.SshKeyRoutes())

		// ── 服务器模块（Beszel 监控：服务器/分组/命令/管理/导入导出） ──
		r.Mount("/api/v1/servers", srvroutes.ServerRoutes())
		r.Mount("/api/v1/server-groups", srvroutes.ServerGroupRoutes())
		r.Mount("/api/v1/server-commands", srvroutes.ServerCommandRoutes())
		r.Mount("/api/v1/server-management", srvroutes.ServerManagementRoutes())
		r.Mount("/api/v1/import-export", srvroutes.ImportExportRoutes())
		r.Mount("/api/v1/agents", srvroutes.AgentRoutes())
		r.Mount("/api/v1/agent", srvroutes.AgentCollectorRoutes())
		r.Mount("/api/v1/ai-models", srvroutes.AIModelRoutes())

		// ── 分析聚合模块（仪表盘/巡检中心/趋势）+ 告警模块 ──
		r.Mount("/api/v1", routes.AnalyticsRoutes())

		// ── 数据库连接 + InfluxDB 写入 + 数据库模式切换与迁移 ──
		r.Mount("/api/v1/db-connections", dbroutes.DbConnectionRoutes())
		r.Mount("/api/v1/influxdb-write", dbroutes.InfluxdbWriteRoutes())
		r.Mount("/api/v1/database-mode", dbroutes.DatabaseModeRoutes())

		// ── 数据中心模块（机房/机柜/机位/PDU/NetBox） ──
		r.Mount("/api/v1/dc", dcroutes.Routes())

		// ── 巡检模版管理 ──
		r.Mount("/api/v1/inspection-templates", inspection.Routes())

		// ── 运维与治理（工作流/任务/容器/K8s/虚拟机/知识库/审计/工具/审批/自愈） ──
		r.Mount("/api/v1/workflows", ops.WorkflowRoutes())
		r.Mount("/api/v1/tasks", ops.TaskRoutes())
		r.Mount("/api/v1/scheduled-tasks", ops.ScheduledTaskRoutes())
		r.Mount("/api/v1/reports", ops.ReportRoutes())
		r.Mount("/api/v1/containers", ops.ContainerRoutes())
		r.Mount("/api/v1/docker-monitor", ops.DockerMonitorRoutes())
		r.Get("/api/v1/docker-hosts", ops.DockerHostsRoute)
		r.Mount("/api/v1/images", ops.ImagesRoutes())
		r.Mount("/api/v1/volumes", ops.VolumesRoutes())
		r.Mount("/api/v1/kubernetes", ops.KubernetesRoutes())
		r.Mount("/api/v1/virtual-machines", ops.VirtualMachineRoutes())
		r.Mount("/api/v1/knowledge", ops.KnowledgeRoutes())
		r.Mount("/api/v1/audit", ops.AuditRoutes())
		r.Mount("/api/v1/audit-logs", ops.AuditRoutes())
		r.Mount("/api/v1/tool-links", ops.ToolLinkRoutes())
		r.Mount("/api/v1/approvals", ops.ApprovalRoutes())
		r.Mount("/api/v1/remediations", ops.RemediationRoutes())
		r.Mount("/api/v1/ai-remediations", ops.AiRemediationRoutes())
		// ── 通知配置与通知中心 ──
		r.Mount("/api/v1/notification-config", ops.NotificationConfigRoutes())
		r.Mount("/api/v1/notifications", ops.NotificationRoutes())

		// ── Copilot 智能运维助手 ──
		r.Mount("/api/v1/copilot", copilot.Routes())
	})

	// ── SPA 回退（前端静态文件） ──
	// 优先使用 STATIC_DIR 环境变量指定的外部目录（开发模式热更新）
	// 否则使用嵌入的前端文件（单文件部署）
	distDir := os.Getenv("STATIC_DIR")
	if distDir != "" {
		if fi, err := os.Stat(distDir); err == nil && fi.IsDir() {
			r.HandleFunc("/*", makeSpaHandler(http.Dir(distDir), nil))
			logger.Infof("Serving frontend from external dir: %s", distDir)
		}
	} else if embeddedFS := FrontendFS(); embeddedFS != nil {
		r.HandleFunc("/*", makeSpaHandler(http.FS(embeddedFS), embeddedFS))
		logger.Infof("Serving frontend from embedded FS")
	}

	logger.Infof("Router initialized with network module (129 endpoints)")
	return r
}

// makeSpaHandler 创建 SPA 回退处理器：精确匹配静态文件（带长期缓存），否则回退到 index.html（无缓存）
func makeSpaHandler(fileSystem http.FileSystem, fallbackFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(fileSystem)
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// 尝试作为静态文件提供
		f, err := fileSystem.Open(path)
		if err == nil {
			f.Close()
			// 如果是 /assets/ 下的哈希产物，配置 1 年强缓存，实现 0ms 浏览器极速加载
			if strings.HasPrefix(path, "/assets/") || strings.HasSuffix(path, ".woff2") || strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".svg") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA 回退：返回 index.html（禁用缓存，确保版本更新立即可见）
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		if fallbackFS != nil {
			// 嵌入文件系统
			data, err := fs.ReadFile(fallbackFS, "index.html")
			if err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(data)
				return
			}
		} else {
			// 外部文件系统
			http.ServeFile(w, r, "index.html")
		}
	}
}
