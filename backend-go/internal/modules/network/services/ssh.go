// SSH 服务层 — 网络设备 SSH 连接、Shell 交互、命令执行
// 等价于 TS networkInspectionService.ts 的 SSH 部分 + sshTypes.ts 的 SSH_ALGORITHMS
package services

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"itops-backend/internal/logger"
)

// ───────────────────────────────────────────────
// SSH 算法配置（兼容旧设备）
// ───────────────────────────────────────────────

// sshKexAlgorithms 密钥交换算法（包含 diffie-hellman-group-exchange-sha1, diffie-hellman-group14-sha1 等旧网络设备协议）
var sshKexAlgorithms = []string{
	"curve25519-sha256",
	"curve25519-sha256@libssh.org",
	"ecdh-sha2-nistp256",
	"ecdh-sha2-nistp384",
	"ecdh-sha2-nistp521",
	"diffie-hellman-group-exchange-sha256",
	"diffie-hellman-group14-sha256",
	"diffie-hellman-group15-sha512",
	"diffie-hellman-group16-sha512",
	// 旧设备兼容（华为 VRP 5.x / H3C Comware 5/7 / Cisco IOS 12/15 / 中兴 / 锐捷等）
	"diffie-hellman-group-exchange-sha1",
	"diffie-hellman-group14-sha1",
	"diffie-hellman-group1-sha1", // 极旧设备（Cisco IOS 12.2 以下、旧 H3C/锐捷）
}

// sshCiphers 加密算法（含 3DES-CBC / arcfour / blowfish 旧设备兼容）
var sshCiphers = []string{
	"chacha20-poly1305@openssh.com",
	"aes128-ctr",
	"aes192-ctr",
	"aes256-ctr",
	"aes128-gcm@openssh.com",
	"aes256-gcm@openssh.com",
	"aes128-cbc",
	"aes192-cbc",
	"aes256-cbc",
	"3des-cbc",       // 极旧 Cisco / 旧 H3C
	"arcfour128",     // 旧设备流密码
	"arcfour256",
	"arcfour",        // 极旧设备
	"blowfish-cbc",   // 部分旧 Juniper / Foundry / 旧 Linux sshd
	"cast128-cbc",    // 部分旧设备
}

// sshMACs HMAC 算法（含 SHA1/MD5 旧设备兼容）
var sshMACs = []string{
	"hmac-sha2-256",
	"hmac-sha2-512",
	"hmac-sha1",
	"hmac-md5",
	"hmac-sha1-96",
	"hmac-md5-96",
	"hmac-sha2-256-etm@openssh.com",
	"hmac-sha2-512-etm@openssh.com",
	"hmac-sha1-etm@openssh.com",
}

// sshHostKeyAlgorithms 主机密钥算法（含 ssh-rsa / ssh-dss 旧设备兼容）
var sshHostKeyAlgorithms = []string{
	"ssh-ed25519",
	"ecdsa-sha2-nistp256",
	"ecdsa-sha2-nistp384",
	"ecdsa-sha2-nistp521",
	"rsa-sha2-256",
	"rsa-sha2-512",
	"ssh-rsa",
	"ssh-dss",
}

// ───────────────────────────────────────────────
// 正则表达式（编译一次复用）
// ───────────────────────────────────────────────

var (
	// ANSI 转义码：颜色、光标移动、清屏等
	ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[()][AB0]|\r`)
	// 分页提示：---- More ---- 或 == more == 等
	moreRegex = regexp.MustCompile(`[-=]{2,}\s*(More|more)\s*[-=]{0,}`)
	// Y/N 确认提示
	ynRegex = regexp.MustCompile(`\[Y/N\]`)
	// enable 密码提示
	enablePasswordRegex = regexp.MustCompile(`(?i)(Password|Passwd)\s*:`)
	// Shell 提示符：Huawei/H3C <DeviceName> 或 《DeviceName》— 允许任意非空白/非括号字符
	anglePromptRegex = regexp.MustCompile(`[<\x{3008}][^\s<>\x{3008}\x{3009}\[\]]+[>\x{3009}]\s*$`)
	// Shell 提示符：Cisco [hostname#] / Huawei sys-view [hostname]
	bracketPromptRegex = regexp.MustCompile(`\[[A-Za-z0-9_\-.]+[#>?]?\]\s*$`)
	// Shell 提示符：hostname# 或 hostname>
	hashPromptRegex = regexp.MustCompile(`^[A-Za-z0-9_\-.]+[#>]\s*$`)
)

// stripANSI 清除 ANSI 转义码（颜色、光标移动等）和 \r 字符
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// ───────────────────────────────────────────────
// 类型定义
// ───────────────────────────────────────────────

// SSHDeviceConfig 设备 SSH 连接参数
type SSHDeviceConfig struct {
	Vendor         string `json:"vendor"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	EnablePassword string `json:"enable_password"`
	PrivateKey     string `json:"private_key"` // PEM 格式私钥（已解密）
	AuthType       string `json:"auth_type"`   // "password" 或 "key"
	OSType         string `json:"os_type"`     // "linux" / "windows"（服务器采集用）
}

// OSTypeHintWindows 判断是否为 Windows 服务器
func (c SSHDeviceConfig) OSTypeHintWindows() bool {
	return strings.EqualFold(c.OSType, "windows")
}

// SSHClient 封装 SSH 连接
type SSHClient struct {
	client *ssh.Client
}

// ───────────────────────────────────────────────
// 连接管理
// ───────────────────────────────────────────────

// buildAuthMethods 根据 SSHDeviceConfig 构建 SSH 认证方法列表
// 支持密钥认证（含密码保护的密钥）、密码认证、keyboard-interactive 认证
// 如果同时提供密钥和密码，两种方法都会添加，SSH 客户端会依次尝试
func buildAuthMethods(device SSHDeviceConfig) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	// 密钥认证
	if device.PrivateKey != "" {
		var signer ssh.Signer
		var err error

		// 如果密码非空，先尝试用密码解密私钥（适用于加密的私钥）
		if device.Password != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(device.PrivateKey), []byte(device.Password))
		}

		// 如果加密密钥解析失败或密码为空，尝试无密码解析
		if signer == nil {
			signer, err = ssh.ParsePrivateKey([]byte(device.PrivateKey))
			if err != nil {
				return nil, fmt.Errorf("parse private key failed: %w", err)
			}
		}
		methods = append(methods, ssh.PublicKeys(signer))
		logger.Debugf("SSH auth: added public key method")
	}

	// 密码认证（作为密钥的 fallback，或密钥认证时也可尝试）
	if device.Password != "" {
		// ssh.Password — 标准 password 认证
		methods = append(methods, ssh.Password(device.Password))
		logger.Debugf("SSH auth: added password method")

		// keyboard-interactive — 部分设备（如华为 V5、某些 FortiGate、
		// 旧 Juniper ScreenOS）仅支持 keyboard-interactive 而非 password
		// golang.org/x/crypto/ssh 的 KeyboardInteractive 回调会被调用一次，
		// 我们直接返回密码即可覆盖大多数单轮挑战场景
		kbdPassword := device.Password
		methods = append(methods, ssh.KeyboardInteractive(
			func(name, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = kbdPassword
				}
				return answers, nil
			},
		))
		logger.Debugf("SSH auth: added keyboard-interactive method")
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no authentication method available (no password or private key)")
	}

	return methods, nil
}

// SSHDialTimeout 连接到网络设备（支持自定义超时时间，含旧设备加密算法兼容）
func SSHDialTimeout(device SSHDeviceConfig, timeout time.Duration) (*SSHClient, error) {
	if device.Port == 0 {
		device.Port = 22
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	authMethods, err := buildAuthMethods(device)
	if err != nil {
		return nil, err
	}

	// 强制 TCP 连接超时：确保不可达 IP 也能在超时后快速失败
	addr := net.JoinHostPort(device.Host, strconv.Itoa(device.Port))
	conn, err := (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接错误: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            device.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		HostKeyAlgorithms: sshHostKeyAlgorithms,
		Timeout:           timeout,
		Config: ssh.Config{
			Ciphers:       sshCiphers,
			KeyExchanges:  sshKexAlgorithms,
			MACs:          sshMACs,
		},
	}

	// 在已建立的 TCP 连接上完成 SSH 握手（设置截止时间防悬挂）
	_ = conn.SetDeadline(time.Now().Add(config.Timeout))
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SSH 连接错误: %w", err)
	}
	_ = conn.SetDeadline(time.Time{})
	client := ssh.NewClient(sshConn, chans, reqs)

	logger.Debugf("SSH connected to %s:%d", device.Host, device.Port)
	return &SSHClient{client: client}, nil
}

// SSHDial 连接到网络设备（默认 10 秒超时）
func SSHDial(device SSHDeviceConfig) (*SSHClient, error) {
	return SSHDialTimeout(device, 10*time.Second)
}

// Close 关闭 SSH 连接
func (c *SSHClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// ───────────────────────────────────────────────
// Exec 模式（单命令执行）
// ───────────────────────────────────────────────

// ExecCommand 通过 exec 通道执行单条命令
func (c *SSHClient) ExecCommand(cmd string, timeout time.Duration) (stdout, stderr string, err error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case err := <-done:
		return stdoutBuf.String(), stderrBuf.String(), err
	case <-time.After(timeout):
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("command timeout after %s", timeout)
	}
}

// ConnectAndExec 连接设备，执行单条命令，断开
func ConnectAndExec(device SSHDeviceConfig, cmd string) (string, error) {
	client, err := SSHDial(device)
	if err != nil {
		return "", err
	}
	defer client.Close()

	stdout, _, err := client.ExecCommand(cmd, 30*time.Second)
	return stdout, err
}

// ───────────────────────────────────────────────
// Shell 模式（交互式命令执行）
// ───────────────────────────────────────────────

// RunShellCommands 通过交互式 Shell 执行多条命令
// 模拟 TS runCommandsViaShell 的完整行为：
//   - vt100 终端，512 列 x 100 行
//   - 厂商分页禁用（Huawei: screen-length 0 temporary, H3C: screen-length disable）
//   - ---- More ---- 自动翻页（最多 1000 次）
//   - Y/N 确认自动回复 N
//   - enable 密码自动输入（stderr 检测）
//   - Shell 提示符检测（<Name> / [Name#] / Name#）
//   - 180s 整体会话超时
func (c *SSHClient) RunShellCommands(device SSHDeviceConfig, commands []string) (string, error) {
	return c.RunShellCommandsWithTimeout(device, commands, 180*time.Second)
}

// RunShellCommandsWithTimeout 与 RunShellCommands 相同，但使用自定义超时
func (c *SSHClient) RunShellCommandsWithTimeout(device SSHDeviceConfig, commands []string, timeout time.Duration) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	// 请求 PTY
	if err := session.RequestPty("vt100", 100, 512, ssh.TerminalModes{}); err != nil {
		return "", fmt.Errorf("request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return "", err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderrPipe, err := session.StderrPipe()
	if err != nil {
		return "", err
	}

	if err := session.Shell(); err != nil {
		return "", fmt.Errorf("start shell: %w", err)
	}

	var output strings.Builder
	var stderrOutput strings.Builder

	// stderr goroutine：累积输出 + 检测 enable 密码提示
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				stderrOutput.WriteString(chunk)
				if enablePasswordRegex.MatchString(chunk) && device.EnablePassword != "" {
					logger.Debugf("Detected enable password prompt, sending enable password")
					stdin.Write([]byte(device.EnablePassword + "\n"))
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// stdout 读取 goroutine
	type chunkMsg struct {
		data string
	}
	chunkCh := make(chan chunkMsg, 100)
	stdoutErrCh := make(chan error, 1)

	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				chunkCh <- chunkMsg{data: string(buf[:n])}
			}
			if err != nil {
				stdoutErrCh <- err
				return
			}
		}
	}()

	// 命令队列：分页禁用命令 + 实际命令
	initCmds := getShellInitCommands(device.Vendor)
	cmdQueue := make([]string, 0, len(initCmds)+len(commands))
	cmdQueue = append(cmdQueue, initCmds...)
	cmdQueue = append(cmdQueue, commands...)

	totalCount := len(cmdQueue)
	// initPhase: 等待初始提示符后再发第一条命令（等待设备 shell 就绪）
	initPhase := len(initCmds) > 0

	cmdIndex := 0
	paginationCount := 0
	allDone := false

	sendNext := func() {
		if len(cmdQueue) == 0 {
			logger.Debugf("Shell: all commands done, sending quit")
			stdin.Write([]byte("quit\n"))
			return
		}
		cmd := cmdQueue[0]
		cmdQueue = cmdQueue[1:]
		cmdIndex++
		logger.Debugf("Shell: sending command (%d/%d): %s", cmdIndex, totalCount, cmd)
		stdin.Write([]byte(cmd + "\n"))
	}

	// 有 init 命令时等待设备 shell 就绪（提示符出现后发送第一条）
	// 无 init 命令时直接发送第一条命令
	if !initPhase {
		sendNext()
	}

	timeoutCh := time.After(timeout)

	for !allDone {
		select {
		case <-timeoutCh:
			finalOutput := output.String()
			if stderrOutput.Len() > 0 {
				finalOutput += "\n[stderr] " + stderrOutput.String()
			}
			return finalOutput, fmt.Errorf("Shell 会话超时(%ds), 已收到 %.1fKB", int(timeout.Seconds()), float64(output.Len())/1024)

		case <-stdoutErrCh:
			// stdout 已关闭，shell 会话结束
			allDone = true

		case msg := <-chunkCh:
			chunk := msg.data
			output.WriteString(chunk)

			// 剥离 ANSI 转义码后再做提示符/分页检测
			cleanChunk := stripANSI(chunk)

			// 检查分页提示
			tail := ""
			cleanOutput := stripANSI(output.String())
			if len(cleanOutput) > 300 {
				tail = cleanOutput[len(cleanOutput)-300:]
			} else {
				tail = cleanOutput
			}

			if moreRegex.MatchString(tail) || moreRegex.MatchString(cleanChunk) {
				paginationCount++
				if paginationCount > 1000 {
					return output.String(), fmt.Errorf("Shell 分页次数过多(%d)，疑似死循环", paginationCount)
				}
				stdin.Write([]byte(" "))
				continue
			}

			// 检查 Y/N 确认
			if ynRegex.MatchString(cleanChunk) {
				logger.Debugf("Shell: detected Y/N prompt, sending N")
				stdin.Write([]byte("N\n"))
				continue
			}

			// 检查 Shell 提示符（在 cleanChunk 上检测）
			if hasShellPrompt(cleanChunk) {
				if initPhase {
					// init 命令完成后，发送第一条实际命令
					initPhase = false
					sendNext()
				} else if len(cmdQueue) == 0 && cmdIndex > 0 {
					// 所有命令执行完毕
					stdin.Write([]byte("quit\n"))
					allDone = true
				} else {
					sendNext()
				}
			}
		}
	}

	// 等待会话关闭
	session.Wait()

	// 追加 stderr
	if stderrOutput.Len() > 0 {
		output.WriteString("\n[stderr] " + stderrOutput.String())
	}

	logger.Debugf("Shell session closed, received %.1fKB output", float64(output.Len())/1024)
	return output.String(), nil
}

// ConnectAndRunShell 连接设备，通过 Shell 执行多条命令，断开
func ConnectAndRunShell(device SSHDeviceConfig, commands []string) (string, error) {
	client, err := SSHDial(device)
	if err != nil {
		return "", err
	}
	defer client.Close()

	return client.RunShellCommands(device, commands)
}

// ConnectAndRunShellWithTimeout 连接设备，通过 Shell 执行命令（自定义超时），断开
func ConnectAndRunShellWithTimeout(device SSHDeviceConfig, commands []string, timeout time.Duration) (string, error) {
	client, err := SSHDial(device)
	if err != nil {
		return "", err
	}
	defer client.Close()

	return client.RunShellCommandsWithTimeout(device, commands, timeout)
}

// ConnectAndExecCommands 连接设备，通过 exec 模式逐条执行命令（fallback）
// exec 模式更简单可靠，无需提示符检测，但不支持分页禁用和 enable 模式
// 适用于 shell 模式失败时的降级方案
// 注意：每条命令重新 dial，避免 session 超时后连接污染
func ConnectAndExecCommands(device SSHDeviceConfig, commands []string) (string, error) {
	var output strings.Builder
	successCount := 0
	for i, cmd := range commands {
		logger.Debugf("Exec: running command (%d/%d): %s", i+1, len(commands), cmd)

		// 每条命令重新建立连接
		client, err := SSHDial(device)
		if err != nil {
			output.WriteString(fmt.Sprintf("\n--- Command %d: connection failed: %s ---\n", i+1, err.Error()))
			continue
		}

		session, err := client.client.NewSession()
		if err != nil {
			client.Close()
			output.WriteString(fmt.Sprintf("\n--- Command %d: create session failed: %s ---\n", i+1, err.Error()))
			continue
		}

		var stdoutBuf, stderrBuf bytes.Buffer
		session.Stdout = &stdoutBuf
		session.Stderr = &stderrBuf

		done := make(chan error, 1)
		go func(s *ssh.Session) {
			done <- s.Run(cmd)
		}(session)

		select {
		case err := <-done:
			session.Close()
			client.Close()
			if err != nil && stdoutBuf.Len() == 0 {
				output.WriteString(fmt.Sprintf("\n--- Command %d failed: %s ---\n", i+1, err.Error()))
				continue
			}
			successCount++
			output.WriteString(stdoutBuf.String())
			if stderrBuf.Len() > 0 {
				output.WriteString("\n[stderr] " + stderrBuf.String())
			}
			output.WriteString("\n")
		case <-time.After(60 * time.Second):
			// 超时：强制关闭 session 和连接
			session.Close()
			client.Close()
			<-done // 等待 goroutine 结束
			if stdoutBuf.Len() > 0 {
				successCount++
				output.WriteString(stdoutBuf.String())
				output.WriteString(fmt.Sprintf("\n--- Command %d: partial output (%d bytes, 60s timeout) ---\n", i+1, stdoutBuf.Len()))
			} else {
				output.WriteString(fmt.Sprintf("\n--- Command %d: timed out after 60s with no output ---\n", i+1))
			}
		}
	}
	if successCount == 0 {
		return output.String(), fmt.Errorf("all %d commands failed in exec mode", len(commands))
	}
	return output.String(), nil
}

// ───────────────────────────────────────────────
// 连接测试
// ───────────────────────────────────────────────

// TestSSHConnection 测试 SSH 连接是否可用，返回延迟（毫秒）
// 兼容旧调用方（仅密码认证）
func TestSSHConnection(host string, port int, username, password string) (int64, error) {
	return TestSSHConnectionWithConfig(SSHDeviceConfig{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
	})
}

// TestSSHConnectionWithConfig 测试 SSH 连接（支持密钥和密码认证）
func TestSSHConnectionWithConfig(device SSHDeviceConfig) (int64, error) {
	start := time.Now()

	client, err := SSHDialTimeout(device, 3500*time.Millisecond)
	if err != nil {
		return 0, fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	// 使用 shell 模式发送探测命令（与 TS 一致，比 exec 更可靠）
	session, err := client.client.NewSession()
	if err != nil {
		return 0, fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	if err := session.RequestPty("vt100", 80, 200, ssh.TerminalModes{}); err != nil {
		return 0, fmt.Errorf("request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return 0, err
	}
	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return 0, err
	}

	if err := session.Shell(); err != nil {
		return 0, fmt.Errorf("start shell: %w", err)
	}

	// 发送探测命令 — 使用通用回车而非厂商特定命令
	// 发送一个空行即可触发提示符返回，无需厂商特定命令
	// 这样对任何厂商都能正确检测连接是否成功
	stdin.Write([]byte("\n"))

	// 读取输出，等待提示符或超时
	buf := make([]byte, 8192)
	probeDone := make(chan bool, 1)
	go func() {
		var sb strings.Builder
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
				s := sb.String()
				// 检测到提示符或有数据，说明命令已返回
				if hasShellPrompt(s) || len(s) > 0 {
					probeDone <- true
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					probeDone <- true
				}
				return
			}
		}
	}()

	select {
	case <-probeDone:
		// 成功
	case <-time.After(2 * time.Second):
		// 超时也认为连接成功（设备可能正在响应）
	}

	stdin.Write([]byte("quit\n"))
	session.Close()

	latency := time.Since(start).Milliseconds()
	logger.Debugf("SSH test connection to %s:%d succeeded, latency=%dms", device.Host, device.Port, latency)
	return latency, nil
}

// ───────────────────────────────────────────────
// 辅助函数
// ───────────────────────────────────────────────

// getShellInitCommand 返回厂商对应的分页禁用命令（已弃用，用 getShellInitCommands）
func getShellInitCommand(vendor string) string {
	switch strings.ToLower(vendor) {
	case "huawei":
		return "screen-length 0 temporary"
	case "h3c":
		return "screen-length disable"
	default:
		return ""
	}
}

// getShellInitCommands 返回分页禁用命令列表
// 对所有厂商设备发送多种分页禁用命令，避免厂商标记错误导致分页未禁用
// 匹配的命令会成功，不匹配的命令会报错（无害）
func getShellInitCommands(vendor string) []string {
	// 覆盖所有常见厂商的分页禁用命令，不依赖 vendor 标记准确性
	return []string{
		"screen-length 0 temporary",      // 华为
		"screen-length disable",          // H3C
		"terminal length 0",              // Cisco（执行模式）
		"terminal pager 0",               // Cisco ASA
		"set cli screen-length 0",        // Juniper JunOS
		"set cli screen-width 1024",      // Juniper JunOS（宽度也设宽）
		"config system console",          // FortiGate（进入 console 配置视图）
		"set output standard",            // FortiGate（标准输出不分页）
		"end",                            // FortiGate（退出配置视图）
		"no paging",                      // 部分旧设备 / D-Link / TP-Link CLI
		"terminal datadump",              // Arista / 部分 Cisco（彻底禁用分页+格式化）
		"skip-page-display",              // 锐捷 RGOS
	}
}

// hasShellPrompt 检测输出中是否包含 Shell 提示符
// 支持 Huawei/H3C <DeviceName> / 《DeviceName》
// 支持 Cisco/其他 [hostname#] / hostname#
func hasShellPrompt(output string) bool {
	lines := strings.Split(output, "\n")
	// 检查最后几行是否有提示符
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-3; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if anglePromptRegex.MatchString(line) {
			return true
		}
		if bracketPromptRegex.MatchString(line) {
			return true
		}
		if hashPromptRegex.MatchString(line) {
			return true
		}
	}
	return false
}

// ExtractCommandOutput 从 Shell 组合输出中提取单条命令的输出
// 等价于 TS extractCommandOutput
func ExtractCommandOutput(shellOutput string, command string) string {
	lines := strings.Split(shellOutput, "\n")
	var result []string
	found := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !found {
			// 找到命令回显行
			if trimmed == command || strings.HasSuffix(trimmed, command) {
				found = true
			}
			continue
		}

		// 检测下一个提示符，结束提取
		if hasShellPrompt(line) {
			break
		}
		// 检测 quit/exit 行
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "quit") || strings.HasPrefix(lower, "exit") || strings.HasPrefix(lower, "logout") {
			break
		}
		// 检测 Y/N 确认行
		if ynRegex.MatchString(line) {
			break
		}

		result = append(result, line)
	}

	if !found {
		// 命令未找到，返回前200字符
		if len(shellOutput) > 200 {
			return shellOutput[:200]
		}
		return shellOutput
	}

	return strings.Join(result, "\n")
}
