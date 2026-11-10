// WebSocket 终端服务 — 网络设备与服务器交互式 Shell
// 前端通过原生 WebSocket 连接，协议为 JSON 消息
package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"

	"itops-backend/internal/config"
	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
)

// ───────────────────────────────────────────────
// WebSocket 消息协议
// ───────────────────────────────────────────────

// WSMessage 客户端→服务端消息格式
type WSMessage struct {
	Type       string `json:"type"`                 // open, data, resize, close
	DeviceID   string `json:"deviceId,omitempty"`   // open 时必填
	Cols       int    `json:"cols,omitempty"`       // open/resize 时必填
	Rows       int    `json:"rows,omitempty"`       // open/resize 时必填
	Data       string `json:"data,omitempty"`       // data 时必填
	SessionID  string `json:"sessionId,omitempty"`  // data/resize/close 时必填
	DeviceType string `json:"deviceType,omitempty"` // "server" | "network-device"
}

// WSResponse 服务端→客户端消息格式
type WSResponse struct {
	Type      string `json:"type"`                // session, data, error, closed
	SessionID string `json:"sessionId,omitempty"` // session/closed 时有
	Data      string `json:"data,omitempty"`      // data 时有
	Error     string `json:"error,omitempty"`     // error 时有
}

// ───────────────────────────────────────────────
// 终端会话
// ───────────────────────────────────────────────

type TerminalSession struct {
	ID        string
	DeviceID  string
	Conn      *websocket.Conn
	CreatedAt time.Time
	WriteData func(data []byte) error
	Resize    func(cols, rows int)
	Cancel    func()
}

var (
	terminalSessions   = make(map[string]*TerminalSession)
	terminalSessionsMu sync.Mutex
)

// ───────────────────────────────────────────────
// WebSocket 升级器
// ───────────────────────────────────────────────

var TerminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// connWriteLocks 为每个 WebSocket 连接维护写锁，保证同一连接上的 WriteMessage 串行执行
// （gorilla/websocket 不允许并发写，否则会触发 panic 并可能导致进程退出）
var (
	connWriteLocksMu sync.Mutex
	connWriteLocks   = make(map[*websocket.Conn]*sync.Mutex)
)

// wsWriteMutex 获取（或创建）某连接的写锁
func wsWriteMutex(conn *websocket.Conn) *sync.Mutex {
	connWriteLocksMu.Lock()
	defer connWriteLocksMu.Unlock()
	mu, ok := connWriteLocks[conn]
	if !ok {
		mu = &sync.Mutex{}
		connWriteLocks[conn] = mu
	}
	return mu
}

// releaseWSWriteMutex 连接关闭后清理写锁
func releaseWSWriteMutex(conn *websocket.Conn) {
	connWriteLocksMu.Lock()
	delete(connWriteLocks, conn)
	connWriteLocksMu.Unlock()
}

// wsWrite 串行写入一条 WebSocket 消息
func wsWrite(conn *websocket.Conn, messageType int, data []byte) error {
	mu := wsWriteMutex(conn)
	mu.Lock()
	defer mu.Unlock()
	return conn.WriteMessage(messageType, data)
}

// HandleTerminalWebSocket 处理 WebSocket 终端连接
func HandleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	authUser := middleware.VerifyToken(token)
	if authUser == nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	// 终端具备设备/服务器命令执行能力，仅允许管理员与运维角色
	if authUser.Role != "admin" && authUser.Role != "operator" {
		http.Error(w, "forbidden: terminal requires admin or operator role", http.StatusForbidden)
		return
	}

	conn, err := TerminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorf("WebSocket upgrade error: %v", err)
		return
	}

	logger.Infof("Terminal WebSocket connected: %s (user: %s)", conn.RemoteAddr(), authUser.Username)
	go handleTerminalMessages(conn)
}

// handleTerminalMessages 处理 WebSocket 消息循环
func handleTerminalMessages(conn *websocket.Conn) {
	defer func() {
		conn.Close()
		cleanupSessionsByConn(conn)
		releaseWSWriteMutex(conn)
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.Errorf("Terminal WebSocket read error: %v", err)
			}
			return
		}

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			sendWSError(conn, "invalid message format")
			continue
		}

		switch msg.Type {
		case "open":
			handleTerminalOpen(conn, msg)
		case "data":
			handleTerminalData(msg)
		case "resize":
			handleTerminalResize(msg)
		case "close":
			handleTerminalClose(msg)
		default:
			sendWSError(conn, fmt.Sprintf("unknown message type: %s", msg.Type))
		}
	}
}

// handleTerminalOpen 处理终端打开请求
func handleTerminalOpen(conn *websocket.Conn, msg WSMessage) {
	if msg.DeviceID == "" {
		sendWSError(conn, "deviceId is required")
		return
	}
	if msg.Cols <= 0 || msg.Rows <= 0 {
		msg.Cols = 80
		msg.Rows = 24
	}

	isNetworkDevice := msg.DeviceType == "network-device" || msg.DeviceType == "network_device"
	logger.Infof("Terminal open: device=%s, type=%s, cols=%d, rows=%d", msg.DeviceID, msg.DeviceType, msg.Cols, msg.Rows)

	var sessionID string
	var shellData <-chan []byte
	var writeData func(data []byte) error
	var resizeFunc func(cols, rows int)
	var closeFunc func()
	var openErr error

	if isNetworkDevice {
		sessionID, shellData, writeData, resizeFunc, closeFunc, openErr = createDeviceTerminal(msg.DeviceID, msg.Cols, msg.Rows)
	} else {
		sessionID, shellData, writeData, resizeFunc, closeFunc, openErr = createServerTerminal(msg.DeviceID, msg.Cols, msg.Rows)
	}

	if sessionID == "" || openErr != nil {
		errMsg := "创建终端会话失败，请检查设备凭据和网络连接"
		if openErr != nil {
			errMsg = fmt.Sprintf("连接失败: %v", openErr)
		}
		sendWSError(conn, errMsg)
		return
	}

	session := &TerminalSession{
		ID:        sessionID,
		DeviceID:  msg.DeviceID,
		Conn:      conn,
		CreatedAt: time.Now(),
		WriteData: writeData,
		Resize:    resizeFunc,
		Cancel:    closeFunc,
	}
	terminalSessionsMu.Lock()
	terminalSessions[sessionID] = session
	terminalSessionsMu.Unlock()

	// 发送 session ID
	sendWSMessage(conn, WSResponse{
		Type:      "session",
		SessionID: sessionID,
	})

	// 启动 shell 输出读取循环
	go func() {
		for data := range shellData {
			err := sendWSMessage(conn, WSResponse{
				Type:      "data",
				SessionID: sessionID,
				Data:      string(data),
			})
			if err != nil {
				logger.Errorf("Failed to send terminal data: %v", err)
				break
			}
		}
		logger.Infof("Terminal session %s shell closed", sessionID)
		sendWSMessage(conn, WSResponse{
			Type:      "closed",
			SessionID: sessionID,
		})
		closeTerminalSession(sessionID)
	}()
}

// handleTerminalData 处理终端输入数据
func handleTerminalData(msg WSMessage) {
	if msg.SessionID == "" {
		return
	}
	terminalSessionsMu.Lock()
	session, ok := terminalSessions[msg.SessionID]
	terminalSessionsMu.Unlock()

	if !ok || session == nil || session.WriteData == nil {
		return
	}
	if err := session.WriteData([]byte(msg.Data)); err != nil {
		logger.Errorf("Write to terminal session %s failed: %v", msg.SessionID, err)
	}
}

// handleTerminalResize 处理终端大小变更
func handleTerminalResize(msg WSMessage) {
	if msg.SessionID == "" {
		return
	}
	terminalSessionsMu.Lock()
	session, ok := terminalSessions[msg.SessionID]
	terminalSessionsMu.Unlock()

	if !ok || session == nil || session.Resize == nil {
		return
	}
	session.Resize(msg.Cols, msg.Rows)
	logger.Debugf("Terminal resize: session=%s, cols=%d, rows=%d", msg.SessionID, msg.Cols, msg.Rows)
}

// handleTerminalClose 处理终端关闭
func handleTerminalClose(msg WSMessage) {
	if msg.SessionID != "" {
		closeTerminalSession(msg.SessionID)
	}
}

// closeTerminalSession 关闭终端会话
func closeTerminalSession(sessionID string) {
	terminalSessionsMu.Lock()
	session, ok := terminalSessions[sessionID]
	if ok {
		delete(terminalSessions, sessionID)
	}
	terminalSessionsMu.Unlock()

	if ok && session != nil && session.Cancel != nil {
		session.Cancel()
		logger.Infof("Terminal session %s closed", sessionID)
	}
}

// cleanupSessionsByConn 清理某个 WebSocket 连接关联的所有会话
func cleanupSessionsByConn(conn *websocket.Conn) {
	terminalSessionsMu.Lock()
	defer terminalSessionsMu.Unlock()

	for id, session := range terminalSessions {
		if session.Conn == conn {
			if session.Cancel != nil {
				session.Cancel()
			}
			delete(terminalSessions, id)
			logger.Infof("Cleaned up terminal session %s (connection closed)", id)
		}
	}
}

// ───────────────────────────────────────────────
// 创建设备终端会话（网络设备）
// ───────────────────────────────────────────────

func createDeviceTerminal(deviceID string, cols, rows int) (sessionID string, shellData <-chan []byte, writeData func(data []byte) error, resizeFunc func(cols, rows int), closeFunc func(), err error) {
	device, err := NetworkDeviceServiceInstance.GetFullCredentials(deviceID)
	if err != nil || device == nil {
		logger.Errorf("createDeviceTerminal: device not found: %s, err=%v", deviceID, err)
		return "", nil, nil, nil, nil, fmt.Errorf("设备不存在或无法获取凭据: %s", deviceID)
	}

	sshPort := device.SSHPort
	if sshPort <= 0 {
		sshPort = 22
	}

	// 若端口明确指定为 23 (Telnet)，直接建立 Telnet 连接
	if sshPort == 23 {
		return createTelnetDeviceTerminal(deviceID, device.Name, device.IPAddress, 23, device.Username, device.Password, device.EnablePassword, cols, rows)
	}

	sshDevice := SSHDeviceConfig{
		Vendor:         device.Vendor,
		Host:           device.IPAddress,
		Port:           sshPort,
		Username:       device.Username,
		Password:       device.Password,
		EnablePassword: device.EnablePassword,
		PrivateKey:     device.PrivateKey,
		AuthType:       device.AuthType,
	}

	sshClient, err := SSHDial(sshDevice)
	if err != nil {
		logger.Warnf("createDeviceTerminal: SSH connect failed for %s (%s:%d): %v, testing Telnet fallback...", device.Name, device.IPAddress, sshPort, err)
		// 如果 SSH 连接失败，自动尝试 Telnet 23 端口回退
		tSession, tCh, tWrite, tResize, tClose, tErr := createTelnetDeviceTerminal(deviceID, device.Name, device.IPAddress, 23, device.Username, device.Password, device.EnablePassword, cols, rows)
		if tErr == nil {
			logger.Infof("createDeviceTerminal: Successfully fell back to Telnet for %s (%s:23)", device.Name, device.IPAddress)
			return tSession, tCh, tWrite, tResize, tClose, nil
		}
		return "", nil, nil, nil, nil, fmt.Errorf("SSH 连接失败: %v", err)
	}

	sshSession, err := sshClient.client.NewSession()
	if err != nil {
		sshClient.Close()
		logger.Errorf("createDeviceTerminal: create session failed: %v", err)
		return "", nil, nil, nil, nil, fmt.Errorf("创建 SSH 会话失败: %w", err)
	}

	// 请求 PTY（针对锐捷 S2952G-E、思科、华为等老设备，依次尝试降级终端模式：vt100 -> xterm -> ansi -> xterm-256color -> dumb）
	ptyCandidates := []string{"vt100", "xterm", "ansi", "xterm-256color", "dumb"}
	var ptySuccess bool
	var lastPtyErr error

	for _, pty := range ptyCandidates {
		termModes := ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}
		if ptyErr := sshSession.RequestPty(pty, rows, cols, termModes); ptyErr == nil {
			ptySuccess = true
			logger.Debugf("createDeviceTerminal: RequestPty '%s' succeeded for %s", pty, device.Name)
			break
		} else {
			lastPtyErr = ptyErr
		}
	}

	if !ptySuccess {
		// 尝试不带高级终端参数的纯 vt100
		if ptyErr := sshSession.RequestPty("vt100", rows, cols, ssh.TerminalModes{}); ptyErr == nil {
			ptySuccess = true
		} else {
			logger.Warnf("createDeviceTerminal: all RequestPty attempts failed (last: %v), trying direct shell...", lastPtyErr)
		}
	}

	// 获取 stdin/stdout/stderr
	stdin, err := sshSession.StdinPipe()
	if err != nil {
		sshSession.Close()
		sshClient.Close()
		return "", nil, nil, nil, nil, fmt.Errorf("获取输入流失败: %w", err)
	}
	stdout, err := sshSession.StdoutPipe()
	if err != nil {
		sshSession.Close()
		sshClient.Close()
		return "", nil, nil, nil, nil, fmt.Errorf("获取输出流失败: %w", err)
	}
	stderr, err := sshSession.StderrPipe()
	if err != nil {
		sshSession.Close()
		sshClient.Close()
		return "", nil, nil, nil, nil, fmt.Errorf("获取错误流失败: %w", err)
	}

	// 启动 shell
	if err := sshSession.Shell(); err != nil {
		sshSession.Close()
		sshClient.Close()
		logger.Errorf("createDeviceTerminal: shell start failed: %v", err)
		return "", nil, nil, nil, nil, fmt.Errorf("启动 Shell 失败: %w", err)
	}

	sessionID = "dev-" + deviceID[:8] + "-" + uuid.New().String()[:8]
	dataCh := make(chan []byte, 512)

	// 读取 stdout
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case dataCh <- data:
				default:
				}
			}
			if err != nil {
				close(dataCh)
				return
			}
		}
	}()

	// 读取 stderr
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case dataCh <- data:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()

	writeData = func(data []byte) error {
		_, err := stdin.Write(data)
		return err
	}

	resizeFunc = func(c, r int) {
		if err := sshSession.WindowChange(r, c); err != nil {
			logger.Debugf("WindowChange failed: %v", err)
		}
	}

	closeFunc = func() {
		sshSession.Close()
		sshClient.Close()
	}

	logger.Infof("Device terminal session %s created for %s (%s:%d)", sessionID, device.Name, device.IPAddress, sshPort)
	return sessionID, dataCh, writeData, resizeFunc, closeFunc, nil
}

// ───────────────────────────────────────────────
// 创建服务器终端会话（普通服务器，通过 SSH 连接）
// ───────────────────────────────────────────────

func createServerTerminal(serverID string, cols, rows int) (sessionID string, shellData <-chan []byte, writeData func(data []byte) error, resizeFunc func(cols, rows int), closeFunc func(), err error) {
	srv, err := repos.Servers.GetByID(serverID)
	if err != nil || srv == nil {
		logger.Errorf("createServerTerminal: server not found: %s, err=%v", serverID, err)
		return "", nil, nil, nil, nil, fmt.Errorf("服务器不存在: %s", serverID)
	}

	// 获取解密后的凭据
	username, password, privateKey, useSSHKey, err := repos.Servers.GetCredentialFields(serverID)
	if err != nil {
		logger.Errorf("createServerTerminal: get credentials failed for %s: %v", serverID, err)
		return "", nil, nil, nil, nil, fmt.Errorf("获取服务器凭据失败: %w", err)
	}

	host := srv.Hostname
	port := srv.Port
	if port <= 0 {
		port = 22
	}

	sshDevice := SSHDeviceConfig{
		Host:       host,
		Port:       port,
		Username:   username,
		Password:   password,
		PrivateKey: privateKey,
		AuthType:   "password",
		OSType:     "linux",
	}
	if srv.OSType != nil {
		sshDevice.OSType = *srv.OSType
	}
	if useSSHKey == 1 && privateKey != "" {
		sshDevice.AuthType = "publickey"
	}

	sshClient, err := SSHDial(sshDevice)
	if err != nil {
		logger.Errorf("createServerTerminal: SSH connect failed for %s (%s:%d): %v", srv.Name, host, port, err)
		return "", nil, nil, nil, nil, fmt.Errorf("SSH 连接失败: %w", err)
	}

	sshSession, err := sshClient.client.NewSession()
	if err != nil {
		sshClient.Close()
		logger.Errorf("createServerTerminal: create session failed: %v", err)
		return "", nil, nil, nil, nil, fmt.Errorf("创建会话失败: %w", err)
	}

	// 请求 PTY
	termCandidates := []string{"xterm-256color", "xterm", "vt100", "ansi", "dumb"}
	for _, pty := range termCandidates {
		termModes := ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}
		if ptyErr := sshSession.RequestPty(pty, rows, cols, termModes); ptyErr == nil {
			break
		}
	}

	stdin, err := sshSession.StdinPipe()
	if err != nil {
		sshSession.Close()
		sshClient.Close()
		return "", nil, nil, nil, nil, fmt.Errorf("获取输入流失败: %w", err)
	}
	stdout, err := sshSession.StdoutPipe()
	if err != nil {
		sshSession.Close()
		sshClient.Close()
		return "", nil, nil, nil, nil, fmt.Errorf("获取输出流失败: %w", err)
	}
	stderr, err := sshSession.StderrPipe()
	if err != nil {
		sshSession.Close()
		sshClient.Close()
		return "", nil, nil, nil, nil, fmt.Errorf("获取错误流失败: %w", err)
	}

	if err := sshSession.Shell(); err != nil {
		sshSession.Close()
		sshClient.Close()
		logger.Errorf("createServerTerminal: shell start failed: %v", err)
		return "", nil, nil, nil, nil, fmt.Errorf("启动 Shell 失败: %w", err)
	}

	sessionID = "srv-" + serverID[:8] + "-" + uuid.New().String()[:8]
	dataCh := make(chan []byte, 512)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case dataCh <- data:
				default:
				}
			}
			if err != nil {
				close(dataCh)
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case dataCh <- data:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()

	writeData = func(data []byte) error {
		_, err := stdin.Write(data)
		return err
	}

	resizeFunc = func(c, r int) {
		if err := sshSession.WindowChange(r, c); err != nil {
			logger.Debugf("WindowChange failed: %v", err)
		}
	}

	closeFunc = func() {
		sshSession.Close()
		sshClient.Close()
	}

	logger.Infof("Server terminal session %s created for %s (%s:%d)", sessionID, srv.Name, host, port)
	return sessionID, dataCh, writeData, resizeFunc, closeFunc, nil
}

// ───────────────────────────────────────────────
// 辅助函数
// ───────────────────────────────────────────────

func sendWSMessage(conn *websocket.Conn, resp WSResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return wsWrite(conn, websocket.TextMessage, data)
}

func sendWSError(conn *websocket.Conn, errMsg string) {
	sendWSMessage(conn, WSResponse{
		Type:  "error",
		Error: errMsg,
	})
}

// validateWSToken 验证 WebSocket token
func validateWSToken(token string) (string, error) {
	claims := jwt.MapClaims{}
	tk, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(config.Env.JWTSecret), nil
	})
	if err != nil || !tk.Valid {
		return "", fmt.Errorf("invalid token")
	}
	id, _ := claims["id"].(string)
	return id, nil
}
