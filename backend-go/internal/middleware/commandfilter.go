package middleware

import (
	"path"
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

const maxCommandDepth = 16

var (
	privilegeEscalation = map[string]bool{"su": true, "sudo": true}
	chainOperators      = map[string]bool{";": true, "&&": true, "||": true, "|": true, "&": true}

	reBase64Pipe = regexp.MustCompile(`base64\s+-d\s*\|`)
	reHexPipe    = regexp.MustCompile(`xxd\s+-r\s*-p\s*\|`)
	reCurlPipeSh = regexp.MustCompile(`(?:curl|wget)\s+.*\|\s*(?:ba)?sh`)
	reSudoPrefix = regexp.MustCompile(`^(?:sudo(?:\s+-\w+(?:\s+\S+)?)?\s+)+`)
)

// tokenizeShell 简化版 shell-quote 分词：处理单/双引号与链式操作符。
// 裸 &、换行、回车、括号都视为命令分隔符，避免 `cmd1\ncmd2`、`(cmd)` 等绕过。
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
		case inSingle:
			if ch == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(ch)
			}
		case inDouble:
			if ch == '"' {
				inDouble = false
			} else {
				cur.WriteByte(ch)
			}
		case ch == '\'':
			inSingle = true
		case ch == '"':
			inDouble = true
		case ch == '\\' && i+1 < len(command):
			cur.WriteByte(command[i+1])
			i++
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '(' || ch == ')':
			flush()
		case ch == ';':
			flush()
			tokens = append(tokens, ";")
		case ch == '|':
			flush()
			if i+1 < len(command) && command[i+1] == '|' {
				tokens = append(tokens, "||")
				i++
			} else {
				tokens = append(tokens, "|")
			}
		case ch == '&':
			flush()
			if i+1 < len(command) && command[i+1] == '&' {
				tokens = append(tokens, "&&")
				i++
			} else {
				tokens = append(tokens, "&")
			}
		default:
			cur.WriteByte(ch)
		}
	}
	flush()
	return tokens
}

// extractCommandSegments 将命令按链式操作符分段，返回每段的 token 序列
func extractCommandSegments(command string) [][]string {
	tokens := tokenizeShell(command)
	var segments [][]string
	var current []string
	for _, tok := range tokens {
		if chainOperators[tok] {
			if len(current) > 0 {
				segments = append(segments, current)
				current = nil
			}
			continue
		}
		current = append(current, tok)
	}
	if len(current) > 0 {
		segments = append(segments, current)
	}
	return segments
}

// commandName 返回命令名（去除路径前缀，如 /bin/rm → rm）
func commandName(tok string) string {
	if tok == "" {
		return ""
	}
	return path.Base(tok)
}

// stripSudo 剥离 sudo 前缀，返回实际命令
func stripSudo(tokens []string) []string {
	for len(tokens) > 0 && commandName(tokens[0]) == "sudo" {
		tokens = tokens[1:]
		// 跳过 sudo 选项及其参数（-u user / -g group / -p prompt）
		for len(tokens) > 0 && strings.HasPrefix(tokens[0], "-") {
			opt := tokens[0]
			tokens = tokens[1:]
			if len(tokens) > 0 && (opt == "-u" || opt == "-g" || opt == "-p" || opt == "-C") {
				tokens = tokens[1:]
			}
		}
	}
	return tokens
}

// isAssignment 判断 token 是否为 VAR=value 形式的环境变量前缀
func isAssignment(tok string) bool {
	idx := strings.Index(tok, "=")
	if idx <= 0 {
		return false
	}
	for i := 0; i < idx; i++ {
		c := tok[i]
		if !(c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// extractSubshells 提取 $(...)（支持嵌套）与反引号中的命令体
func extractSubshells(s string) []string {
	var out []string
	for i := 0; i < len(s); i++ {
		if s[i] == '`' {
			j := i + 1
			for j < len(s) && s[j] != '`' {
				j++
			}
			if j < len(s) {
				out = append(out, s[i+1:j])
				i = j
			}
			continue
		}
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '(' {
			depth := 0
			j := i + 1
			for ; j < len(s); j++ {
				switch s[j] {
				case '(':
					depth++
				case ')':
					depth--
				}
				if depth == 0 {
					break
				}
			}
			if j < len(s) && j > i+2 {
				out = append(out, s[i+2:j])
			}
			i = j
		}
	}
	return out
}

func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// checkSegment 检查单段命令的 token 序列（含 sh -c / eval / env / xargs / find -exec 载荷）
func checkSegment(tokens []string, userRole string, depth int) *SafetyResult {
	// 反复剥离环境变量前缀与 sudo，防止 `FOO=1 sudo rm` 之类的组合绕过
	for {
		changed := false
		for len(tokens) > 0 && (isAssignment(tokens[0]) || tokens[0] == "!") {
			tokens = tokens[1:]
			changed = true
		}
		stripped := stripSudo(tokens)
		if len(stripped) != len(tokens) {
			tokens = stripped
			changed = true
		}
		if !changed {
			break
		}
	}
	if len(tokens) == 0 {
		return nil
	}

	cmdName := commandName(tokens[0])
	args := tokens[1:]

	if policy, ok := dangerousCommandNames[cmdName]; ok && containsRole(policy.BlockedRoles, userRole) {
		if policy.Action == "block" {
			return &SafetyResult{
				Allowed:  false,
				Severity: "blocked",
				Reason:   "禁止操作: " + policy.Description + " (" + cmdName + ")",
				Policy:   policy.Name,
			}
		}
	}
	if privilegeEscalation[cmdName] && userRole == "viewer" {
		return &SafetyResult{
			Allowed:  true,
			Severity: "warning",
			Reason:   "警告: 权限提升尝试 (" + cmdName + ")",
			Policy:   "privilege_escalation",
		}
	}

	// 提取需要递归检查的内层命令载荷
	var payload string
	switch cmdName {
	case "sh", "bash", "zsh", "dash", "ksh", "ash":
		for i, a := range args {
			// 支持 -c / -lc / -ic 等组合短选项
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "c") && i+1 < len(args) {
				payload = strings.Join(args[i+1:], " ")
				break
			}
		}
	case "eval":
		payload = strings.Join(args, " ")
	case "env":
		// 跳过 env 选项与 VAR=value 前缀
		for len(args) > 0 && (strings.HasPrefix(args[0], "-") || isAssignment(args[0])) {
			if args[0] == "-u" && len(args) > 1 {
				args = args[2:]
			} else {
				args = args[1:]
			}
		}
		payload = strings.Join(args, " ")
	case "xargs":
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			opt := args[0]
			args = args[1:]
			if (opt == "-I" || opt == "-n" || opt == "-P" || opt == "-L" || opt == "-s" || opt == "-d" || opt == "-a") && len(args) > 0 {
				args = args[1:]
			}
		}
		payload = strings.Join(args, " ")
	case "find":
		for i, a := range args {
			if a == "-exec" || a == "-execdir" || a == "-ok" || a == "-okdir" {
				payload = strings.Join(args[i+1:], " ")
				break
			}
		}
	}

	if strings.TrimSpace(payload) != "" {
		inner := checkCommandSafetyDepth(payload, userRole, depth+1)
		if !inner.Allowed || inner.Severity == "blocked" {
			return &SafetyResult{
				Allowed:  false,
				Severity: "blocked",
				Reason:   "嵌套命令中包含禁止操作: " + inner.Reason,
				Policy:   "nested_command",
			}
		}
		if inner.Severity == "warning" {
			return &inner
		}
	}
	return nil
}

func checkCommandSafetyDepth(command, userRole string, depth int) SafetyResult {
	if depth > maxCommandDepth {
		return SafetyResult{
			Allowed:  false,
			Severity: "blocked",
			Reason:   "命令嵌套层级过深",
			Policy:   "nesting_limit",
		}
	}
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return SafetyResult{Allowed: true, Severity: "safe"}
	}

	// ── 1. 检测子 shell 执行 $(...) 和反引号（支持嵌套） ──
	for _, inner := range extractSubshells(trimmed) {
		innerResult := checkCommandSafetyDepth(inner, userRole, depth+1)
		if !innerResult.Allowed || innerResult.Severity == "blocked" {
			return SafetyResult{
				Allowed:  false,
				Severity: "blocked",
				Reason:   "子 shell 中包含禁止操作: " + innerResult.Reason,
				Policy:   "subshell_injection",
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

	// ── 4. 按操作符分割，对每段独立检查 ──
	segments := extractCommandSegments(trimmed)
	if len(segments) == 0 {
		segments = [][]string{strings.Fields(trimmed)}
	}
	var warning *SafetyResult
	for _, tokens := range segments {
		res := checkSegment(tokens, userRole, depth)
		if res == nil {
			continue
		}
		if !res.Allowed || res.Severity == "blocked" {
			return *res
		}
		if warning == nil {
			w := *res
			warning = &w
		}
	}
	if warning != nil {
		return *warning
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

// CheckCommandSafety 检查命令安全性（等价 checkCommandSafety）
func CheckCommandSafety(command, userRole string) SafetyResult {
	return checkCommandSafetyDepth(command, userRole, 0)
}
