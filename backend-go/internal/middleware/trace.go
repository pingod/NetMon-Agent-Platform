package middleware

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/logger"
)

// ───────────────────────────────────────────────
// Trace 与请求日志（等价 TS middleware/trace.ts）
// ───────────────────────────────────────────────

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// Hijack 透传（WebSocket 升级需要）
func (sr *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := sr.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
}

// Flush 透传（SSE/流式响应需要）
func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Trace 生成/透传 X-Trace-Id 并记录请求日志
func Trace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get("X-Trace-Id")
		if traceID == "" {
			traceID = uuid.NewString()
		}
		w.Header().Set("X-Trace-Id", traceID)

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		duration := time.Since(start).Milliseconds()
		url := r.URL.RequestURI()
		line := r.Method + " " + url + " - " + strconv.Itoa(rec.status) + " (" + strconv.FormatInt(duration, 10) + "ms)"
		switch {
		case rec.status >= 500:
			logger.Errorf("%s [trace=%s ip=%s]", line, traceID, ClientIP(r))
		case rec.status >= 400:
			logger.Warnf("%s [trace=%s ip=%s]", line, traceID, ClientIP(r))
		default:
			logger.Infof("%s [trace=%s ip=%s]", line, traceID, ClientIP(r))
		}
	})
}
