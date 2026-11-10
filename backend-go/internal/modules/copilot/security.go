// Copilot 安全审查器 — 严格只读权限硬隔离（Read-Only Guardrail）
// 确保 AI 仅具备只读查询权限，绝对禁止任何配置下发、状态修改或破坏性操作
package copilot

import (
	"fmt"
	"regexp"
	"strings"
)

// shellMetaBlocks 命令拼接/注入元字符黑名单（命中即整条拒绝，绝不做子命令拆分递归）
var shellMetaBlocks = []struct {
	token  string
	reason string
}{
	{"`", "禁止使用反引号进行命令替换"},
	{"$(", "禁止使用 $(...) 进行命令替换"},
	{"${", "禁止使用 ${...} 变量展开"},
	{"&&", "禁止使用 && 串联多条命令"},
	{"||", "禁止使用 || 串联多条命令"},
	{"|", "禁止使用管道符 | 串联命令"},
	{"&", "禁止使用后台执行符 &"},
	{";", "禁止使用分号串联多条命令"},
	{"<", "禁止使用输入重定向 <"},
	{">", "禁止使用输出重定向或文件写入 >"},
	{"\n", "禁止使用换行符串联多条命令"},
	{"\r", "禁止使用回车符串联多条命令"},
	{"\"", "禁止使用双引号包裹参数"},
	{"'", "禁止使用单引号包裹参数"},
}

// universalBlocklist 通用破坏性/提权黑名单（整串小写匹配）
var universalBlocklist = []struct {
	pattern string
	reason  string
}{
	{"system-view", "禁止进入网络设备配置模式 (system-view)"},
	{"configure terminal", "禁止进入全局配置模式 (configure terminal)"},
	{"config t", "禁止进入配置模式 (config t)"},
	{"conf t", "禁止进入配置模式 (conf t)"},
	{"undo ", "禁止执行配置撤销/删除指令 (undo)"},
	{"no ", "禁止执行反转/取消配置指令 (no ...)"},
	{"shutdown", "禁止执行端口或系统关闭指令 (shutdown)"},
	{"reboot", "禁止执行设备或系统重启指令 (reboot)"},
	{"reload", "禁止执行设备重新加载指令 (reload)"},
	{"poweroff", "禁止执行系统关机指令 (poweroff)"},
	{"halt", "禁止执行停机指令 (halt)"},
	{"reset ", "禁止执行重置/清空指令 (reset)"},
	{"clear ", "禁止执行清理/清空状态指令 (clear)"},
	{"delete ", "禁止执行文件或配置删除指令 (delete)"},
	{"erase ", "禁止执行存储抹除指令 (erase)"},
	{"format ", "禁止执行格式化指令 (format)"},
	{"save", "禁止执行配置保存/持久化指令 (save)"},
	{"write", "禁止执行配置写入操作 (write)"},
	{"commit", "禁止执行配置提交指令 (commit)"},
	{"patch ", "禁止执行补丁更新操作 (patch)"},
	{"upgrade ", "禁止执行系统升级操作 (upgrade)"},
	{"boot-loader", "禁止修改启动项引导文件 (boot-loader)"},
	{"vlan ", "禁止下发 VLAN 划分与修改配置"},
	{"ip address ", "禁止修改或配置 IP 地址"},
	{"ospf ", "禁止修改动态路由 OSPF 配置"},
	{"bgp ", "禁止修改动态路由 BGP 配置"},
	{"acl ", "禁止修改访问控制列表 ACL"},
	{"user-interface", "禁止修改用户接口安全策略"},
	{"local-user", "禁止修改本地账号或密码"},
	{"password", "禁止执行密码与身份凭证修改"},
	{"snmp-agent community", "禁止修改 SNMP 团体字"},
	{"rm ", "禁止执行文件删除指令 (rm)"},
	{"rmdir", "禁止执行目录删除指令 (rmdir)"},
	{"truncate", "禁止执行文件截断清空 (truncate)"},
	{"chmod", "禁止修改文件权限属性 (chmod)"},
	{"chown", "禁止修改文件所属用户 (chown)"},
	{"kill", "禁止终止系统或服务进程 (kill)"},
	{"pkill", "禁止批量终止服务进程 (pkill)"},
	{"systemctl restart", "禁止重启系统服务 (systemctl restart)"},
	{"systemctl stop", "禁止停止系统服务 (systemctl stop)"},
	{"systemctl disable", "禁止禁用系统服务 (systemctl disable)"},
	{"service ", "禁止管理系统后台服务 (service ...)"},
	{"mkfs", "禁止格式化文件系统 (mkfs)"},
	{"fdisk", "禁止操作磁盘分区表 (fdisk)"},
	{"dd ", "禁止执行底层块写入指令 (dd)"},
	{"sudo ", "禁止使用超级管理员提权指令 (sudo)"},
	{"su ", "禁止切换超级管理员用户 (su)"},
	{"passwd", "禁止修改系统密码 (passwd)"},
	{"useradd", "禁止创建新系统用户 (useradd)"},
	{"userdel", "禁止删除系统用户 (userdel)"},
	{"iptables -f", "禁止清空防火墙规则 (iptables -F)"},
	{"sed -i", "禁止就地修改文件内容 (sed -i)"},
	{"apt install", "禁止安装软件包 (apt install)"},
	{"apt remove", "禁止卸载软件包 (apt remove)"},
	{"yum install", "禁止安装软件包 (yum install)"},
	{"yum remove", "禁止卸载软件包 (yum remove)"},
	{"docker rm", "禁止删除 Docker 容器"},
	{"docker stop", "禁止停止 Docker 容器"},
	{"docker restart", "禁止重启 Docker 容器"},
}

// networkAllowedPrefixes 网络设备（交换机/路由器/防火墙）只读指令白名单
var networkAllowedPrefixes = []string{
	"display", "show", "dir", "more",
	"get", "diagnose",
	"ping", "tracert", "traceroute",
	"screen-length 0 temporary",
	"terminal length 0",
}

// serverAllowedPrefixes 主机服务器只读指令白名单
// 注意：awk/grep 可执行子命令或读取任意文件，已移出白名单
var serverAllowedPrefixes = []string{
	"df", "free", "uptime", "uname", "top -b -n 1", "ps", "cat", "head", "tail",
	"ls", "dir", "ip", "ifconfig", "netstat", "ss", "lsof", "systemctl status",
	"journalctl", "dmesg", "docker ps", "docker stats --no-stream", "docker inspect",
	"docker logs", "vmstat", "iostat", "w", "who", "last", "hostname",
}

// allowedArgPattern 只读命令参数允许出现的字符集合（命令 + 安全参数字符整串校验）
const allowedArgPattern = `[A-Za-z0-9 _./:=@,+-]*`

// buildAllowedCommandRegex 构造“白名单命令前缀 + 仅安全参数字符”的整串正则
func buildAllowedCommandRegex(prefixes []string) *regexp.Regexp {
	escaped := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		escaped = append(escaped, regexp.QuoteMeta(p))
	}
	return regexp.MustCompile(`^(?:` + strings.Join(escaped, "|") + `)(?:\s` + allowedArgPattern + `)?$`)
}

var (
	networkAllowedRe = buildAllowedCommandRegex(networkAllowedPrefixes)
	serverAllowedRe  = buildAllowedCommandRegex(serverAllowedPrefixes)
	anyAllowedRe     = buildAllowedCommandRegex(append(append([]string{}, networkAllowedPrefixes...), serverAllowedPrefixes...))
)

// hasReadOnlyVerbPrefix 判断命令是否以纯查询动词开头
func hasReadOnlyVerbPrefix(lowerCmd string) bool {
	for _, verb := range []string{"display ", "display", "show ", "show", "dir", "more ", "get ", "diagnose ", "ping ", "tracert ", "traceroute "} {
		if lowerCmd == verb || strings.HasPrefix(lowerCmd, verb) {
			return true
		}
	}
	return false
}

// ValidateReadOnlyCommand 校验是否为只读命令
// 返回 (allowed, reason)。命中任何注入元字符直接整体拒绝，绝不拆分执行。
func ValidateReadOnlyCommand(deviceCategory string, rawCmd string) (bool, string) {
	cmd := strings.TrimSpace(rawCmd)
	if cmd == "" {
		return false, "指令不能为空"
	}

	// 1. 命令拼接/注入元字符硬阻断（不递归拆分，避免绕过与死循环）
	for _, m := range shellMetaBlocks {
		if strings.Contains(cmd, m.token) {
			return false, m.reason + "（指令 [" + cmd + "] 已被整体拒绝）"
		}
	}

	// 2. 通用破坏性/提权黑名单
	// 读类动词（display/show/dir/more/get/diagnose/ping/tracert）后接的参数只是查询对象，
	// 白名单已保证不存在拼接，故跳过配置黑名单，避免 display vlan / display interface brief 被误杀。
	lower := strings.ToLower(cmd)
	if !hasReadOnlyVerbPrefix(lower) {
		for _, b := range universalBlocklist {
			if strings.Contains(lower, b.pattern) {
				return false, b.reason
			}
		}
	}

	// 3. 白名单整串正则校验：命令前缀 + 仅安全参数字符
	switch strings.ToLower(strings.TrimSpace(deviceCategory)) {
	case "network", "switch", "router", "firewall":
		if networkAllowedRe.MatchString(lower) {
			return true, ""
		}
		return false, fmt.Sprintf("网络设备仅支持只读查询命令（如 display ... 或 show ...），指令 [%s] 不在只读白名单内或包含非法参数字符", cmd)
	case "server", "host":
		if serverAllowedRe.MatchString(lower) {
			return true, ""
		}
		return false, fmt.Sprintf("主机仅支持只读监控诊断命令（如 df, free, ps, uptime, docker ps 等），指令 [%s] 不在只读白名单内或包含非法参数字符", cmd)
	}

	if anyAllowedRe.MatchString(lower) {
		return true, ""
	}
	return false, fmt.Sprintf("指令 [%s] 不符合只读查询规范。AI 仅开放设备配置与状态查询（如 display ...、show ...、df -h、free 等），禁止任何修改、下发或命令拼接操作", cmd)
}

// GenerateSecurityBlockNotice 生成规范的安全审计拦截报告
func GenerateSecurityBlockNotice(cmd string, reason string) string {
	// 压平空白并转义会破坏 Markdown 表格/代码块的字符
	now := strings.Join(strings.Fields(cmd), " ")
	now = strings.ReplaceAll(now, "`", "'")
	now = strings.ReplaceAll(now, "|", "\\|")
	now = strings.ReplaceAll(now, "\n", " ")
	reason = strings.ReplaceAll(reason, "`", "'")
	reason = strings.ReplaceAll(reason, "|", "\\|")

	var sb strings.Builder
	sb.WriteString("### 🛑 安全审计拦截：只读权限硬隔离已触发\n\n")
	sb.WriteString("> **安全策略声明**：NetMon AI 助手被配置为 **只读查询模式 (Read-Only Enforced)**，**绝对禁止** 下发配置、修改运行状态或执行具有系统破坏性的指令。\n\n")
	sb.WriteString("| 审计项 | 详情 |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **拦截目标指令** | `%s` |\n", now))
	sb.WriteString(fmt.Sprintf("| **拦截阻断原因** | `%s` |\n", reason))
	sb.WriteString("| **防护级别** | `High Security (零信任只读沙箱)` |\n")
	sb.WriteString("| **设备状态** | `未向任何设备下发指令，网络与系统配置保持完好` |\n\n")
	sb.WriteString("#### 💡 允许执行的只读指令推荐\n\n")
	sb.WriteString("* **网络交换机/路由器**：\n")
	sb.WriteString("  * `display current-configuration`（查询当前全量配置）\n")
	sb.WriteString("  * `display version`（查询系统版本与硬件型号）\n")
	sb.WriteString("  * `display ip routing-table`（查询 IP 路由表）\n")
	sb.WriteString("  * `display interface brief`（查询端口运行简要信息）\n")
	sb.WriteString("  * `display vlan`（查询 VLAN 配置情况）\n")
	sb.WriteString("* **Linux/Windows 主机**：\n")
	sb.WriteString("  * `df -h`（查询磁盘挂载与空间使用率）\n")
	sb.WriteString("  * `free -m`（查询系统内存与 Swap 占用）\n")
	sb.WriteString("  * `docker ps`（查询运行中的容器清单）\n")
	sb.WriteString("  * `uptime`（查询系统运行时长与平均负载）\n\n")
	sb.WriteString("*如需调整网络设备配置，请在授权运维窗口期通过人工控制台或经过严格审批的配置变更流程进行操作。*")

	return sb.String()
}
