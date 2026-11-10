// 路由层共享工具函数
package routes

import (
	"encoding/json"
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
