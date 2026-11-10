// Package httpx 统一 API 响应格式，等价于 TS shared/utils/response.ts
package httpx

import (
	"encoding/json"
	"net/http"
	"reflect"
)

// ApiResponse 统一响应结构
type ApiResponse struct {
	Success    bool        `json:"success"`
	Data       interface{} `json:"data,omitempty"`
	Error      string      `json:"error,omitempty"`
	Message    string      `json:"message,omitempty"`
	Pagination *Pagination `json:"pagination,omitempty"`
	Details    interface{} `json:"details,omitempty"`
}

// Pagination 分页信息
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ensureNonNilSlice 将 nil slice 转为空 slice，避免 JSON 序列化成 null
// 前端期望列表端点返回 [] 而非 null，否则读取 .length 会崩溃
func ensureNonNilSlice(data interface{}) interface{} {
	if data == nil {
		return data
	}
	v := reflect.ValueOf(data)
	// 指针类型：解引用后检查
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return data
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Slice && v.IsNil() {
		return reflect.MakeSlice(v.Type(), 0, 0).Interface()
	}
	return data
}

// Respond 成功响应 {success:true, data}
func Respond(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, ApiResponse{Success: true, Data: ensureNonNilSlice(data)})
}

// RespondStatus 成功响应，自定义状态码
func RespondStatus(w http.ResponseWriter, status int, data interface{}) {
	writeJSON(w, status, ApiResponse{Success: true, Data: ensureNonNilSlice(data)})
}

// RespondMessage 成功响应 {success:true, message, data:null}
func RespondMessage(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusOK, ApiResponse{Success: true, Message: message, Data: nil})
}

// RespondMessageStatus 成功响应，自定义状态码
func RespondMessageStatus(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ApiResponse{Success: true, Message: message, Data: nil})
}

// RespondError 错误响应 {success:false, error}
func RespondError(w http.ResponseWriter, errMsg string, status int) {
	writeJSON(w, status, ApiResponse{Success: false, Error: errMsg})
}

// RespondPagination 分页响应
func RespondPagination(w http.ResponseWriter, data interface{}, total, page, pageSize int) {
	writeJSON(w, http.StatusOK, ApiResponse{
		Success:    true,
		Data:       ensureNonNilSlice(data),
		Pagination: &Pagination{Page: page, PageSize: pageSize, Total: total},
	})
}

// RespondRaw 直接写入任意 JSON 值（用于登录等需自定义顶层结构的端点）
func RespondRaw(w http.ResponseWriter, status int, body interface{}) {
	writeJSON(w, status, body)
}

// FailResponse 失败响应结构 {success:false, message, code?}
type FailResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// RespondFailStatus 失败响应（自定义状态码）
func RespondFailStatus(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, FailResponse{Success: false, Message: message})
}

// RespondFailCode 失败响应（自定义状态码 + code 字段，如 PASSWORD_MUST_CHANGE）
func RespondFailCode(w http.ResponseWriter, status int, message, code string) {
	writeJSON(w, status, FailResponse{Success: false, Message: message, Code: code})
}
