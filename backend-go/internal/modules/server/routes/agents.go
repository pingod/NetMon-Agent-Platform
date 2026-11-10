// AI Agent 路由 — 对应 TS agentRoutes.ts
// 挂载路径：/api/v1/agents
// 提供：Agent 列表、Agent 测试（生成运维命令）
package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/repos"
)

// AgentRoutes AI Agent 路由
func AgentRoutes() chi.Router {
	r := chi.NewRouter()

	// GET / — Agent 列表
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		agents, err := repos.Agents.List()
		if err != nil {
			httpx.RespondError(w, "获取 Agent 列表失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, agents)
	})

	// POST /{id}/test — 测试 Agent（根据输入生成运维命令）
	r.Post("/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var req struct {
			Input     string   `json:"input"`
			ServerIDs []string `json:"serverIds"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		// 记录使用次数
		repos.Agents.IncrementUsage(id)

		output := generateCommandFromInput(req.Input)
		httpx.Respond(w, map[string]interface{}{
			"output": output,
			"agent_id": id,
		})
	})

	return r
}

// generateCommandFromInput 根据用户需求生成运维命令（模板规则，无需外部 AI）
// 前端会从 output 中提取 {...} JSON 解析出 command 和 explanation
func generateCommandFromInput(input string) string {
	lower := strings.ToLower(input)

	// 系统信息/版本类
	if containsAny(lower, "版本", "版本信息", "os", "系统信息", "uname") {
		return mustJSON("uname -a && cat /etc/os-release", "查看系统内核与操作系统版本信息")
	}
	// CPU 类
	if containsAny(lower, "cpu", "处理器", "负载", "核心") {
		if containsAny(lower, "负载", "load") {
			return mustJSON("uptime && top -bn1 | head -5", "查看系统负载与 CPU 使用情况")
		}
		return mustJSON("lscpu && nproc", "查看 CPU 型号与核心数")
	}
	// 内存类
	if containsAny(lower, "内存", "memory", "mem") {
		return mustJSON("free -m", "查看内存使用情况（单位 MB）")
	}
	// 磁盘类
	if containsAny(lower, "磁盘", "硬盘", "disk", "df", "空间") {
		return mustJSON("df -h", "查看磁盘分区使用情况")
	}
	// 网络类
	if containsAny(lower, "网络", "端口", "网卡", "network", "netstat", "ss ") {
		if containsAny(lower, "端口", "port") {
			return mustJSON("ss -tlnp", "查看系统监听端口")
		}
		return mustJSON("ip addr show && ip route show", "查看网卡地址与路由信息")
	}
	// 进程类
	if containsAny(lower, "进程", "进程数", "process", "ps ") {
		return mustJSON("ps aux --sort=-%mem | head -20", "查看占用内存最高的 20 个进程")
	}
	// 服务类
	if containsAny(lower, "服务", "systemctl", "service", "启动", "停止", "重启") {
		if containsAny(lower, "重启", "restart") {
			return mustJSON("systemctl list-units --type=service --state=running | head -20", "查看运行中的服务列表，确认需要重启的服务")
		}
		return mustJSON("systemctl list-units --type=service --state=running | head -30", "查看运行中的系统服务")
	}
	// 日志类
	if containsAny(lower, "日志", "log") {
		return mustJSON("journalctl -n 50 --no-pager", "查看最近 50 条系统日志")
	}
	// 安全/登录类
	if containsAny(lower, "登录", "安全", "ssh", "失败") {
		return mustJSON("last -n 20 && grep -E 'Failed|Invalid' /var/log/auth.log 2>/dev/null | tail -20", "查看最近登录记录与认证失败日志")
	}
	// 默认
	return mustJSON("uname -a && uptime && free -m && df -h", "采集服务器基础信息（系统、负载、内存、磁盘）")
}

// containsAny 判断字符串是否包含任意关键词
func containsAny(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// mustJSON 生成 {command, explanation} JSON 字符串
func mustJSON(command, explanation string) string {
	obj := map[string]string{
		"command":     command,
		"explanation": explanation,
	}
	buf, err := json.Marshal(obj)
	if err != nil {
		return fmt.Sprintf(`{"command":"%s","explanation":"%s"}`, command, explanation)
	}
	return string(buf)
}
