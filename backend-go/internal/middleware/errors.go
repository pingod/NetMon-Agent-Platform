package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"itops-backend/internal/config"
	"itops-backend/internal/httpx"
	"itops-backend/internal/logger"
)

// ───────────────────────────────────────────────
// 错误处理（等价 TS middleware/errorHandler.ts）
// ───────────────────────────────────────────────

// AppError 业务错误（等价 TS types/errors AppError）
type AppError struct {
	StatusCode int
	Code       string
	Message    string
	Details    interface{}
}

func (e *AppError) Error() string { return e.Message }

// NewAppError 创建业务错误
func NewAppError(code string, message string, statusCode int) *AppError {
	return &AppError{StatusCode: statusCode, Code: code, Message: message}
}

// ErrHandler 自定义错误处理器（由路由层注册，用于统一渲染业务错误）
var ErrHandler func(w http.ResponseWriter, r *http.Request, err error)

// Recover panic 恢复中间件
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Errorf("Unexpected panic on %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				if ErrHandler != nil {
					ErrHandler(w, r, fmt.Errorf("%v", rec))
					return
				}
				httpx.RespondError(w, "服务器内部错误", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// NotFound 404 处理（等价 notFoundHandler）
func NotFound(w http.ResponseWriter, r *http.Request) {
	msg := fmt.Sprintf("Route %s %s not found", r.Method, r.URL.Path)
	if ErrHandler != nil {
		ErrHandler(w, r, NewAppError("NOT_FOUND", msg, http.StatusNotFound))
		return
	}
	httpx.RespondError(w, msg, http.StatusNotFound)
}

// HandleError 供 handler 调用的统一错误渲染入口
func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	appErr, ok := err.(*AppError)
	if !ok {
		logger.Errorf("Unexpected error on %s %s: %v", r.Method, r.URL.Path, err)
		httpx.RespondRaw(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"code":    "SERVER_ERROR",
			"message": "服务器内部错误",
		})
		return
	}
	status := appErr.StatusCode
	if status == 0 {
		status = http.StatusInternalServerError
	}
	logger.Warnf("[%s] %s (%s %s)", appErr.Code, appErr.Message, r.Method, r.URL.Path)
	body := map[string]interface{}{
		"success": false,
		"code":    appErr.Code,
		"message": appErr.Message,
	}
	if appErr.Details != nil {
		body["details"] = appErr.Details
	}
	if config.Env != nil && !config.Env.IsProduction() {
		body["stack"] = fmt.Sprintf("%+v", err)
	}
	httpx.RespondRaw(w, status, body)
}
