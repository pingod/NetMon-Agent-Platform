// VNC WebSocket 代理 — 将浏览器 WebSocket 连接桥接到 VNC 服务器的 TCP 连接
// 前端使用标准 WebSocket（noVNC 兼容），后端透传 RFB 协议数据
package services

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
)

// ───────────────────────────────────────────────
// VNC WebSocket 升级器
// ───────────────────────────────────────────────

var VncUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// VncConnectRequest 前端发送的连接请求
// 出于 SSRF 防护考虑，VNC 目标主机/端口只能来自 servers 表，客户端传入的 vncHost/vncPort 会被忽略。
type VncConnectRequest struct {
	ServerID string `json:"serverId"`
	VncHost  string `json:"vncHost"`
	VncPort  int    `json:"vncPort"`
}

// HandleVncWebSocket 处理 VNC WebSocket 连接
// 路由: GET /ws/vnc?token=xxx&serverId=xxx
func HandleVncWebSocket(w http.ResponseWriter, r *http.Request) {
	// 验证 token
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
	// VNC 可远程控制服务器，仅允许管理员与运维角色
	if authUser.Role != "admin" && authUser.Role != "operator" {
		http.Error(w, "forbidden: vnc requires admin or operator role", http.StatusForbidden)
		return
	}

	// 升级 WebSocket
	wsConn, err := VncUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorf("VNC WebSocket upgrade error: %v", err)
		return
	}
	defer func() {
		wsConn.Close()
		releaseWSWriteMutex(wsConn)
	}()

	// 等待前端发送连接请求
	_, msgData, err := wsConn.ReadMessage()
	if err != nil {
		logger.Errorf("VNC WebSocket read error: %v", err)
		return
	}

	var req VncConnectRequest
	if err := json.Unmarshal(msgData, &req); err != nil {
		wsWrite(wsConn, websocket.TextMessage, []byte(`{"type":"error","message":"invalid connect request"}`))
		return
	}

	if req.ServerID == "" {
		wsWrite(wsConn, websocket.TextMessage, []byte(`{"type":"error","message":"serverId is required"}`))
		return
	}

	// 目标地址只从 servers 表读取，禁止客户端直传 host/port（防止任意内网地址访问）
	// VNC 密码在仓库中加密存储，由 GetVncConfig 负责解密；代理层只需主机与端口。
	vncHost, vncPort, _, cfgErr := repos.Servers.GetVncConfig(req.ServerID)
	if cfgErr != nil {
		wsWrite(wsConn, websocket.TextMessage, []byte(`{"type":"error","message":"server not found"}`))
		return
	}
	vncHost = strings.TrimSpace(vncHost)
	if vncHost == "" {
		wsWrite(wsConn, websocket.TextMessage, []byte(`{"type":"error","message":"server has no vnc host configured"}`))
		return
	}
	if vncPort <= 0 {
		vncPort = 5900
	}

	// 连接 VNC 服务器
	addr := net.JoinHostPort(vncHost, fmt.Sprintf("%d", vncPort))
	dialer := net.Dialer{Timeout: 10 * time.Second}
	vncConn, err := dialer.Dial("tcp", addr)
	if err != nil {
		errMsg := fmt.Sprintf("连接 VNC 服务器 %s 失败: %v", addr, err)
		payload, _ := json.Marshal(map[string]string{"type": "error", "message": errMsg})
		wsWrite(wsConn, websocket.TextMessage, payload)
		logger.Errorf("VNC connect to %s failed: %v", addr, err)
		return
	}
	defer vncConn.Close()

	logger.Infof("VNC WebSocket proxy established: %s -> %s (user: %s)", wsConn.RemoteAddr(), addr, authUser.Username)

	// 通知前端连接成功
	wsWrite(wsConn, websocket.TextMessage, []byte(`{"type":"connected"}`))

	// 双向数据透传
	proxyVncConnection(wsConn, vncConn)
	logger.Infof("VNC WebSocket proxy closed: %s -> %s", wsConn.RemoteAddr(), addr)
}

// proxyVncConnection 双向透传 WebSocket↔TCP
func proxyVncConnection(wsConn *websocket.Conn, vncConn net.Conn) {
	var once sync.Once
	closeAll := func() {
		vncConn.Close()
		wsConn.Close()
	}

	// WS → VNC（读取浏览器发来的数据，写入 VNC TCP）
	go func() {
		defer once.Do(closeAll)
		for {
			_, data, err := wsConn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					logger.Debugf("VNC WS read error: %v", err)
				}
				return
			}
			if _, err := vncConn.Write(data); err != nil {
				logger.Debugf("VNC TCP write error: %v", err)
				return
			}
		}
	}()

	// VNC → WS（读取 VNC TCP 数据，写入浏览器）
	func() {
		defer once.Do(closeAll)
		buf := make([]byte, 8192)
		for {
			n, err := vncConn.Read(buf)
			if n > 0 {
				if err := wsWrite(wsConn, websocket.BinaryMessage, buf[:n]); err != nil {
					logger.Debugf("VNC WS write error: %v", err)
					return
				}
			}
			if err != nil {
				logger.Debugf("VNC TCP read error: %v", err)
				return
			}
		}
	}()
}
