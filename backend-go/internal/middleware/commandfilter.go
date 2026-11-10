package middleware

import (
	"regexp"
	"strings"
)

// ───────────────────────────────────────────────
// 命令安全过滤（等价 TS middleware/commandFilter.ts）
// ───────────────────────────────────────────────

// CommandPolicy 命令策略
type CommandPolicy struct {
	Name         string
	Description  string
	Action       string // block | warn | allow
	BlockedRoles []string
}

// SafetyResult 命令安全检查结果
type SafetyResult struct {
	Allowed  bool   `json:"allowed"`
	Severity string `json:"severity"` // blocked | warning | safe
	Reason   string `json:"reason,omitempty"`
	Policy   string `json:"policy,omitempty"`
}

var dangerousCommandNames = map[string]CommandPolicy{
	"rm":         {Name: "filesystem_destructive", Description: "破坏性文件系统操作", Action: "block", BlockedRoles: []string{"viewer", "operator"}},
	"dd":         {Name: "filesystem_destructive", Description: "破坏性文件系统操作", Action: "block", BlockedRoles: []string{"viewer", "operator"}},
	"shred":      {Name: "filesystem_destructive", Description: "破坏性文件系统操作", Action: "block", BlockedRoles: []string{"viewer", "operator"}},
	"mkfs":       {Name: "system_critical", Description: "系统关键操作", Action: "block", BlockedRoles: []string{"viewer", "operator", "admin"}},
	"fdisk":      {Name: "system_critical", Description: "系统关键操作", Action: "block", BlockedRoles: []string{"viewer", "operator", "admin"}},
	"parted":     {Name: "system_critical", Description: "系统关键操作", Action: "block", BlockedRoles: []string{"viewer", "operator", "admin"}},
	"cryptsetup": {Name: "system_critical", Description: "系统关键操作", Action: "block", BlockedRoles: []string{"viewer", "operator", "admin"}},
	"lvremove":   {Name: "system_critical", Description: "系统关键操作", Action: "block", BlockedRoles: []string{"viewer", "operator", "admin"}},
	"vgremove":   {Name: "system_critical", Description: "系统关键操作", Action: "block", BlockedRoles: []string{"viewer", "operator", "admin"}},
	"iptables":   {Name: "network_destructive", Description: "网络破坏性操作", Action: "block", BlockedRoles: []string{"viewer", "operator"}},
	"ip":         {Name: "network_destructive", Description: "网络破坏性操作", Action: "block", BlockedRoles: []string{"viewer", "operator"}},
	"tc":         {Name: "network_destructive", Description: "网络破坏性操作", Action: "block", BlockedRoles: []string{"viewer", "operator"}},
	"kill":       {Name: "process_kill", Description: "批量终止进程", Action: "block", BlockedRoles: []string{"viewer", "operator"}},
	"killall":    {Name: "process_kill", Description: "批量终止进程", Action: "block", BlockedRoles: []string{"viewer", "operator"}},
	"pkill":      {Name: "process_kill", Description: "批量终止进程", Action: "block", BlockedRoles: []string{"viewer", "operator"}},
	"reboot":     {Name: "system_critical", Description: "系统关键操作", Action: "block", BlockedRoles: []string{"viewer", "operator", "admin"}},
	"shutdown":   {Name: "system_critical", Description: "系统关键操作", Action: "block", BlockedRoles: []string{"viewer", "operator", "admin"}},
	"halt":       {Name: "system_critical", Description: "系统关键操作", Action: "block", BlockedRoles: []string{"viewer", "operator", "admin"}},
	"init":       {Name: "system_critical", Description: "系统关键操作", Action: "block", BlockedRoles: []string{"viewer", "operator", "admin"}},
}

var credentialPatterns = []struct {
	re     *regexp.Regexp
	policy CommandPolicy
}{
	{regexp.MustCompile(`/etc/shadow`), CommandPolicy{Name: "credential_access", Description: "凭据访问尝试", Action: "warn", BlockedRoles: []string{"viewer"}}},
	{regexp.MustCompile(`/etc/passwd`), CommandPolicy{Name: "credential_access", Description: "凭据访问尝试", Action: "warn", BlockedRoles: []string{"viewer"}}},
	{regexp.MustCompile(`\.pem\b`), CommandPolicy{Name: "credential_access", Description: "凭据访问尝试", Action: "warn", BlockedRoles: []string{"viewer"}}},
	{regexp.MustCompile(`id_rsa`), CommandPolicy{Name: "credential_access", Description: "凭据访问尝试", Action: "warn", BlockedRoles: []string{"viewer"}}},
	{regexp.MustCompile(`\.key\b`), CommandPolicy{Name: "credential_access", Description: "凭据访问尝试", Action: "warn", BlockedRoles: []string{"viewer"}}},
	{regexp.MustCompile(`(?i)\bPASSWORD\b`), CommandPolicy{Name: "credential_access", Description: "凭据访问尝试", Action: "warn", BlockedRoles: []string{"viewer"}}},
	{regexp.MustCompile(`(?i)\bSECRET\b`), CommandPolicy{Name: "credential_access", Description: "凭据访问尝试", Action: "warn", BlockedRoles: []string{"viewer"}}},
}

var (
	privilegeEscalation = map[string]bool{"su": true, "sudo": true}
	chainOperators      = map[string]bool{";": true, "&&": true, "||": true, "|": true}

	reSubShellDollar = regexp.MustCompile(`\$\(([^)]+)\)`)
	reSubShellTick   = regexp.MustCompile("`([^`]+)`")
	reBase64Pipe     = regexp.MustCompile(`base64\s+-d\s*\|`)
	reHexPipe        = regexp.MustCompile(`xxd\s+-r\s*-p\s*\|`)
	reCurlPipeSh     = regexp.MustCompile(`(?:curl|wget)\s+.*\|\s*(?:ba)?sh`)
	reSudoPrefix     = regexp.MustCompile(`^(?:sudo(?:\s+-\w+(?:\s+\S+)?)?\s+)+`)
	reWhitespace     = regexp.MustCompile(`\s+`)
)

// tokenizeShell 简化版 shell-quote 分词：处理单/双引号与链式操作符
func tokenizeShell(command string) []string {
	var tokens []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(command); i++ {
		ch := command[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case !inSingle && !inDouble && (ch == ' ' || ch == '\t' || ch == '\n'):
			flush()
		case !inSingle && !inDouble && ch == ';':
			flush()
			tokens = append(tokens, ";")
		case !inSingle && !inDouble && ch == '|':
			flush()
			if i+1 < len(command) && command[i+1] == '|' {
				tokens = append(tokens, "||")
				i++
			} else {
				tokens = append(tokens, "|")
			}
		case !inSingle && !inDouble && ch == '&':
			if i+1 < len(command) && command[i+1] == '&' {
				flush()
				tokens = append(tokens, "&&")
				i++
			} else {
				cur.WriteByte(ch)
			}
		default:
			cur.WriteByte(ch)
		}
	}
	flush()
	return tokens
}

// extractCommandSegments 将命令按链式操作符分段，返回每段命令名（第一个 token）
func extractCommandSegments(command string) []string {
	tokens := tokenizeShell(command)
	var segments []string
	currentCmd := ""
	for _, tok := range tokens {
		if chainOperators[tok] {
			if currentCmd != "" {
				segments = append(segments, currentCmd)
			}
			currentCmd = ""
		} else if currentCmd == "" {
			currentCmd = tok
		}
	}
	if currentCmd != "" {
		segments = append(segments, currentCmd)
	}
	if len(segments) == 0 {
		// 回退到简单空格分割
		for _, p := range reWhitespace.Split(strings.TrimSpace(command), -1) {
			if p != "" {
				segments = append(segments, p)
			}
		}
	}
	return segments
}

// stripSudo 剥离 sudo 前缀，返回实际命令
func stripSudo(cmd string) string {
	return strings.TrimSpace(reSudoPrefix.ReplaceAllString(cmd, ""))
}

func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// CheckCommandSafety 检查命令安全性（等价 checkCommandSafety）
func CheckCommandSafety(command, userRole string) SafetyResult {
	if strings.TrimSpace(command) == "" {
		return SafetyResult{Allowed: true, Severity: "safe"}
	}
	trimmed := strings.TrimSpace(command)

	// ── 1. 检测子 shell 执行 $(...) 和反引号 ──
	if strings.Contains(trimmed, "$(") || strings.Contains(trimmed, "`") {
		subs := reSubShellDollar.FindAllStringSubmatch(trimmed, -1)
		if len(subs) == 0 {
			subs = reSubShellTick.FindAllStringSubmatch(trimmed, -1)
		}
		for _, m := range subs {
			inner := strings.TrimSpace(m[1])
			innerResult := CheckCommandSafety(inner, userRole)
			if innerResult.Severity == "blocked" {
				return SafetyResult{
					Allowed:  false,
					Severity: "blocked",
					Reason:   "子 shell 中包含禁止操作: " + innerResult.Reason,
					Policy:   "subshell_injection",
				}
			}
		}
	}

	// ── 2. 检测 base64/hex 解码管道 ──
	if reBase64Pipe.MatchString(trimmed) || reHexPipe.MatchString(trimmed) {
		return SafetyResult{
			Allowed:  false,
			Severity: "blocked",
			Reason:   "禁止使用 base64/hex 解码管道执行命令",
			Policy:   "encoding_bypass",
		}
	}

	// ── 3. 检测管道到 shell 的操作（curl/wget | sh）──
	if reCurlPipeSh.MatchString(trimmed) {
		return SafetyResult{
			Allowed:  false,
			Severity: "blocked",
			Reason:   "禁止从网络下载脚本直接执行",
			Policy:   "remote_script_execution",
		}
	}

	// ── 4. 按操作符分割，对每段独立检查命令名 ──
	for _, segment := range extractCommandSegments(trimmed) {
		parts := reWhitespace.Split(stripSudo(segment), -1)
		cmdName := ""
		if len(parts) > 0 {
			cmdName = parts[0]
		}
		if cmdName == "" {
			continue
		}

		if policy, ok := dangerousCommandNames[cmdName]; ok && containsRole(policy.BlockedRoles, userRole) {
			if policy.Action == "block" {
				return SafetyResult{
					Allowed:  false,
					Severity: "blocked",
					Reason:   "禁止操作: " + policy.Description + " (" + cmdName + ")",
					Policy:   policy.Name,
				}
			}
		}

		if privilegeEscalation[cmdName] && userRole == "viewer" {
			return SafetyResult{
				Allowed:  true,
				Severity: "warning",
				Reason:   "警告: 权限提升尝试 (" + cmdName + ")",
				Policy:   "privilege_escalation",
			}
		}
	}

	// ── 5. 检查凭据访问模式（对完整命令字符串）──
	for _, cp := range credentialPatterns {
		if cp.re.MatchString(trimmed) && containsRole(cp.policy.BlockedRoles, userRole) {
			return SafetyResult{
				Allowed:  true,
				Severity: "warning",
				Reason:   "警告: " + cp.policy.Description,
				Policy:   cp.policy.Name,
			}
		}
	}

	return SafetyResult{Allowed: true, Severity: "safe"}
}
