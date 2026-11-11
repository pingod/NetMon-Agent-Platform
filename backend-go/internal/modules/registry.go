// 路由注册 — 等价于 TS modules/_registry.ts
// 所有模块路由在此统一注册，构成完整的 HTTP 路由树
package modules

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"

	"itops-backend/internal/config"
	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
	authmod "itops-backend/internal/modules/auth"
	"itops-backend/internal/modules/configmgmt"
	"itops-backend/internal/modules/copilot"
	dbroutes "itops-backend/internal/modules/database/routes"
	dcroutes "itops-backend/internal/modules/dc/routes"
	"itops-backend/internal/modules/inspection"
	"itops-backend/internal/modules/network/routes"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/modules/ops"
	"itops-backend/internal/modules/planning"
	"itops-backend/internal/modules/registries"
	srvroutes "itops-backend/internal/modules/server/routes"
)

// NewRouter 构建完整的 HTTP 路由
func NewRouter() chi.Router {
	// 打破 ops → server/routes 循环依赖：SSH 执行能力注入给工作流执行器
	ops.SSHExecHook = srvroutes.ExecOnServer
	// 打破 ops → copilot 循环依赖（copilot → network/services → ops）：agent 节点执行能力注入
	ops.AgentRunHook = func(ctx context.Context, prompt string) string {
		return copilot.ProcessMessage(ctx, prompt, "admin")
	}
	// RCA 根因推断的大模型通道（未配置模型时 hook 返回错误，RCA 回退规则推断）
	ops.LLMHook = copilot.CallLLM

	r := chi.NewRouter()

	// ── 全局中间件 ──
	r.Use(chimw.RequestID)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Compress(5))
	// CORS：仅允许配置的来源，不使用通配符，也不启用凭据
	allowedOrigins := []string{"http://localhost:3000", "http://localhost:8088"}
	if config.Env != nil && len(config.Env.AllowedOrigins) > 0 {
		allowedOrigins = config.Env.AllowedOrigins
	}
	corsOpts := cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Refresh-Token", "X-Agent-Token", "X-Trace-Id"},
		ExposedHeaders:   []string{"Link", "X-Trace-Id", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"},
		AllowCredentials: false,
		MaxAge:           300,
	}
	r.Use(cors.New(corsOpts).Handler)
	r.Use(middleware.Trace)
	r.Use(middleware.RateLimiter)
	r.Use(middleware.WebhookIPFilter)

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
		// Agent 指标上报使用 X-Agent-Token 由处理器自行校验，其余请求强制 JWT
		r.Use(middleware.AuthenticateOrAgentReport)
		// 初始密码未修改时仅允许 auth 模块的豁免端点（这些端点不在本组内）
		r.Use(middleware.RequirePasswordChange)

		// ── 用户管理（模块内再限 admin） ──
		r.Mount("/api/v1/users", authmod.UserRoutes())

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

		// ── 配置模板中心 / 镜像仓库 / Compose 编排 ──
		r.Mount("/api/v1/config-templates", configmgmt.Routes())
		r.Mount("/api/v1/registries", registries.Routes())
		r.Mount("/api/v1/compose", ops.ComposeRoutes())
		r.Mount("/api/v1/snapshot-policies", ops.SnapshotPolicyRoutes())

		// ── IDC 上架规划与布线引擎（原 IDCBoxLine） ──
		r.Mount("/api/v1/planning", planning.Routes())
		r.Get("/api/v1/planning-status", planning.StatusHandler)

		// ── 巡检模版管理 ──
		r.Mount("/api/v1/inspection-templates", inspection.Routes())

		// ── 运维与治理（工作流/任务/容器/K8s/虚拟机/知识库/审计/工具/审批/自愈） ──
		r.Mount("/api/v1/workflows", ops.WorkflowRoutes())
	r.Mount("/api/v1/scripts", srvroutes.ScriptRoutes())
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
		r.Mount("/api/v1/remediation-policies", ops.RemediationPolicyRoutes())
		r.Mount("/api/v1/remediation-executions", ops.RemediationExecutionRoutes())
		r.Mount("/api/v1/remediation-audits", ops.RemediationAuditRoutes())
		r.Mount("/api/v1/ai-remediations", ops.AiRemediationRoutes())
		r.Mount("/api/v1/auto-scale", ops.AutoScaleRoutes())
		r.Mount("/api/v1/root-cause-analysis", ops.RcaRoutes())
		r.Mount("/api/v1/backups", ops.BackupRoutes())
		r.Mount("/api/v1/cost-analysis", ops.CostAnalysisRoutes())
		// ── 通知配置与通知中心 ──
		r.Mount("/api/v1/notification-config", ops.NotificationConfigRoutes())
		r.Mount("/api/v1/notifications", ops.NotificationRoutes())

		// ── Copilot 智能运维助手 ──
		r.Mount("/api/v1/copilot", copilot.Routes())
	})

	// ── 规划前端静态资源（需在 SPA 回退之前注册） ──
	r.HandleFunc("/planning-app", planning.StaticHandler())
	r.HandleFunc("/planning-app/*", planning.StaticHandler())

	// ── SPA 回退（前端静态文件） ──
	// 优先使用 STATIC_DIR 环境变量指定的外部目录（开发模式热更新）
	// 否则使用嵌入的前端文件（单文件部署）
	distDir := os.Getenv("STATIC_DIR")
	if distDir != "" {
		if fi, err := os.Stat(distDir); err == nil && fi.IsDir() {
			r.HandleFunc("/*", makeSpaHandler(http.Dir(distDir), nil, filepath.Join(distDir, "index.html")))
			logger.Infof("Serving frontend from external dir: %s", distDir)
		}
	} else if embeddedFS := FrontendFS(); embeddedFS != nil {
		r.HandleFunc("/*", makeSpaHandler(http.FS(embeddedFS), embeddedFS, ""))
		logger.Infof("Serving frontend from embedded FS")
	}

	logger.Infof("Router initialized with network module (129 endpoints)")
	return r
}

// makeSpaHandler 创建 SPA 回退处理器：精确匹配静态文件（带长期缓存），否则回退到 index.html（无缓存）。
// indexFile 为空时从 fallbackFS 读取嵌入式 index.html；否则读取外部文件系统上的 index.html 绝对路径。
func makeSpaHandler(fileSystem http.FileSystem, fallbackFS fs.FS, indexFile string) http.HandlerFunc {
	fileServer := http.FileServer(fileSystem)
	return func(w http.ResponseWriter, r *http.Request) {
		reqPath := r.URL.Path

		// 尝试作为静态文件提供
		f, err := fileSystem.Open(reqPath)
		if err == nil {
			f.Close()
			// 如果是 /assets/ 下的哈希产物，配置 1 年强缓存，实现 0ms 浏览器极速加载
			if strings.HasPrefix(reqPath, "/assets/") || strings.HasSuffix(reqPath, ".woff2") || strings.HasSuffix(reqPath, ".png") || strings.HasSuffix(reqPath, ".svg") {
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
		} else if indexFile != "" {
			// 外部文件系统：使用 STATIC_DIR 下的 index.html 绝对路径
			http.ServeFile(w, r, indexFile)
			return
		}
		http.NotFound(w, r)
	}
}
