// 路由层共享工具函数
package routes

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"itops-backend/internal/httpx"
)

// parseJSON 解析请求 JSON body，失败时自动返回 400 响应
func parseJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		httpx.RespondError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// contains 判断字符串是否包含子串
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// timeNowLocal 返回与 TS datetime('now','localtime') 一致的本地时间字符串
func timeNowLocal() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// validSnmpHost 校验 SNMP 目标主机：仅允许 IP 或主机名，拒绝 URL/路径等畸形输入
func validSnmpHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	// IP 字面量优先放行（IPv6 含冒号，不能被下面的字符黑名单误杀）
	if net.ParseIP(host) != nil {
		return true
	}
	if strings.ContainsAny(host, "/\\:@?# \t\r\n") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

// validSnmpPort 校验端口范围
func validSnmpPort(port int) bool {
	return port >= 1 && port <= 65535
}

// checkSnmpTarget 校验主机+端口，失败时自动返回 400
func checkSnmpTarget(w http.ResponseWriter, host string, port int) bool {
	if !validSnmpHost(host) {
		httpx.RespondError(w, "无效的目标主机（仅支持 IP 或主机名）", http.StatusBadRequest)
		return false
	}
	if !validSnmpPort(port) {
		httpx.RespondError(w, "无效的端口（1-65535）", http.StatusBadRequest)
		return false
	}
	return true
}

// validOID 校验数字 OID（如 1.3.6.1.2.1.1.1），允许前导点
func validOID(oid string) bool {
	oid = strings.TrimPrefix(oid, ".")
	if oid == "" || len(oid) > 512 {
		return false
	}
	for _, part := range strings.Split(oid, ".") {
		if part == "" {
			return false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}
