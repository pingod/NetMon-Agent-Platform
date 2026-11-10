// 服务器模块路由层共享工具函数
package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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

// parseInt 安全解析整数
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// parseTags 将逗号分隔的标签字符串解析为 []string
func parseTags(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
