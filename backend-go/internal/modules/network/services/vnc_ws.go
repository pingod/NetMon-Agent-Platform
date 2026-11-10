// VNC WebSocket 代理 — 将浏览器 WebSocket 连接桥接到 VNC 服务器的 TCP 连接
// 前端使用标准 WebSocket（noVNC 兼容），后端透传 RFB 协议数据
package services

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
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
type VncConnectRequest struct {
	ServerID string `json:"serverId"`
	VncHost   string `json:"vncHost"`
	VncPort   int    `json:"vncPort"`
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

	// 升级 WebSocket
	wsConn, err := VncUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorf("VNC WebSocket upgrade error: %v", err)
		return
	}
	defer wsConn.Close()

	// 等待前端发送连接请求
	_, msgData, err := wsConn.ReadMessage()
	if err != nil {
		logger.Errorf("VNC WebSocket read error: %v", err)
		return
	}

	var req VncConnectRequest
	if err := json.Unmarshal(msgData, &req); err != nil {
		wsConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"invalid connect request"}`))
		return
	}

	// 如果前端没有指定 vncHost/vncPort，从数据库读取
	vncHost := req.VncHost
	vncPort := req.VncPort
	if vncHost == "" || vncPort == 0 {
		if req.ServerID == "" {
			wsConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"serverId or vncHost/vncPort required"}`))
			return
		}
		srv, err := repos.Servers.GetByID(req.ServerID)
		if err != nil || srv == nil {
			wsConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"server not found"}`))
			return
		}
		if vncHost == "" {
			vncHost = srv.Hostname
		}
		if vncPort == 0 {
			if srv.VncPort != nil && *srv.VncPort > 0 {
				vncPort = *srv.VncPort
			} else {
				vncPort = 5900
			}
		}
	}

	// 连接 VNC 服务器
	addr := net.JoinHostPort(vncHost, fmt.Sprintf("%d", vncPort))
	dialer := net.Dialer{Timeout: 10 * time.Second}
	vncConn, err := dialer.Dial("tcp", addr)
	if err != nil {
		errMsg := fmt.Sprintf("连接 VNC 服务器 %s 失败: %v", addr, err)
		wsConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"`+errMsg+`"}`))
		logger.Errorf("VNC connect to %s failed: %v", addr, err)
		return
	}
	defer vncConn.Close()

	logger.Infof("VNC WebSocket proxy established: %s -> %s (user: %s)", wsConn.RemoteAddr(), addr, authUser.Username)

	// 通知前端连接成功
	wsConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"connected"}`))

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
				if err := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
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
