// Telnet 终端服务 — 支持传统网络设备交互式 WebShell
package services

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"itops-backend/internal/logger"
)

// Telnet 协议控制常量 (RFC 854)
const (
	telnetSE   = 240
	telnetNOP  = 241
	telnetDM   = 242
	telnetBRK  = 243
	telnetIP   = 244
	telnetAO   = 245
	telnetAYT  = 246
	telnetEC   = 247
	telnetEL   = 248
	telnetGA   = 249
	telnetSB   = 250
	telnetWILL = 251
	telnetWONT = 252
	telnetDO   = 253
	telnetDONT = 254
	telnetIAC  = 255

	telnetOptEcho     = 1
	telnetOptSGA      = 3
	telnetOptTerminal = 24
	telnetOptNAWS     = 31
)

// createTelnetDeviceTerminal 创建基于 Telnet 的网络设备交互终端
func createTelnetDeviceTerminal(deviceID, name, host string, port int, username, password, enablePassword string, cols, rows int) (sessionID string, shellData <-chan []byte, writeData func(data []byte) error, resizeFunc func(cols, rows int), closeFunc func(), err error) {
	if port <= 0 {
		port = 23
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, dialErr := (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).Dial("tcp", addr)
	if dialErr != nil {
		return "", nil, nil, nil, nil, fmt.Errorf("Telnet 连接到 %s:%d 失败: %w", host, port, dialErr)
	}

	sessionID = "telnet-" + deviceID[:8] + "-" + uuid.New().String()[:8]
	dataCh := make(chan []byte, 512)

	// 发送初始终端大小 NAWS (RFC 1073)
	sendNAWS := func(c, r int) {
		naws := []byte{
			telnetIAC, telnetSB, telnetOptNAWS,
			byte(c >> 8), byte(c & 0xFF),
			byte(r >> 8), byte(r & 0xFF),
			telnetIAC, telnetSE,
		}
		_, _ = conn.Write(naws)
	}
	sendNAWS(cols, rows)

	// 异步读取循环与 Telnet 协商处理
	go func() {
		defer close(dataCh)
		defer conn.Close()

		buf := make([]byte, 4096)
		var userSent, passSent bool
		for {
			n, rerr := conn.Read(buf)
			if n > 0 {
				raw := buf[:n]
				clean, responses := processTelnetIAC(raw, cols, rows)
				if len(responses) > 0 {
					_, _ = conn.Write(responses)
				}
				if len(clean) > 0 {
					text := string(clean)
					lower := strings.ToLower(text)
					// 自动应答 Username/Password 提示
					if !userSent && username != "" && (strings.Contains(lower, "username:") || strings.Contains(lower, "login:") || strings.Contains(lower, "user name:") || strings.Contains(lower, "user:")) {
						time.Sleep(100 * time.Millisecond)
						_, _ = conn.Write([]byte(username + "\r\n"))
						userSent = true
					} else if !passSent && password != "" && (strings.Contains(lower, "password:") || strings.Contains(lower, "passwd:")) {
						time.Sleep(100 * time.Millisecond)
						_, _ = conn.Write([]byte(password + "\r\n"))
						passSent = true
					}

					chunk := make([]byte, len(clean))
					copy(chunk, clean)
					select {
					case dataCh <- chunk:
					default:
					}
				}
			}
			if rerr != nil {
				return
			}
		}
	}()

	writeData = func(data []byte) error {
		_, werr := conn.Write(data)
		return werr
	}

	resizeFunc = func(c, r int) {
		sendNAWS(c, r)
	}

	closeFunc = func() {
		_ = conn.Close()
	}

	logger.Infof("Telnet terminal session %s created for %s (%s:%d)", sessionID, name, host, port)
	return sessionID, dataCh, writeData, resizeFunc, closeFunc, nil
}

// processTelnetIAC 解析并过滤 Telnet 命令序列，生成应答字节流
func processTelnetIAC(raw []byte, cols, rows int) (cleanData []byte, response []byte) {
	var clean bytes.Buffer
	var resp bytes.Buffer

	i := 0
	for i < len(raw) {
		if raw[i] == telnetIAC {
			if i+1 >= len(raw) {
				break
			}
			cmd := raw[i+1]
			switch cmd {
			case telnetIAC: // 转义 0xFF
				clean.WriteByte(0xFF)
				i += 2
			case telnetWILL, telnetWONT, telnetDO, telnetDONT:
				if i+2 < len(raw) {
					opt := raw[i+2]
					switch cmd {
					case telnetDO:
						if opt == telnetOptNAWS {
							resp.Write([]byte{telnetIAC, telnetWILL, telnetOptNAWS})
						} else if opt == telnetOptTerminal {
							resp.Write([]byte{telnetIAC, telnetWILL, telnetOptTerminal})
						} else {
							resp.Write([]byte{telnetIAC, telnetWONT, opt})
						}
					case telnetDONT:
						resp.Write([]byte{telnetIAC, telnetWONT, opt})
					case telnetWILL:
						if opt == telnetOptEcho || opt == telnetOptSGA {
							resp.Write([]byte{telnetIAC, telnetDO, opt})
						} else {
							resp.Write([]byte{telnetIAC, telnetDONT, opt})
						}
					case telnetWONT:
						resp.Write([]byte{telnetIAC, telnetDONT, opt})
					}
					i += 3
				} else {
					i = len(raw)
				}
			case telnetSB:
				// 子协商直到 IAC SE
				end := bytes.Index(raw[i:], []byte{telnetIAC, telnetSE})
				if end != -1 {
					subData := raw[i : i+end+2]
					if len(subData) > 3 && subData[2] == telnetOptTerminal && subData[3] == 1 { // SEND
						// 回复 "VT100"
						termResp := append([]byte{telnetIAC, telnetSB, telnetOptTerminal, 0}, []byte("VT100")...)
						termResp = append(termResp, telnetIAC, telnetSE)
						resp.Write(termResp)
					}
					i += end + 2
				} else {
					i = len(raw)
				}
			default:
				i += 2
			}
		} else {
			clean.WriteByte(raw[i])
			i++
		}
	}
	return clean.Bytes(), resp.Bytes()
}
