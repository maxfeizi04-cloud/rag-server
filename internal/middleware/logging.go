package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// responseWriter 包装 http.ResponseWriter,捕获状态码和响应大小
// Go 的 http.ResponseWriter 接口不直接暴露状态码,必须用包装器
type responseWriter struct {
	http.ResponseWriter
	status int   // HTTP 状态码
	size   int64 // 响应体大小
}

// WriteHeader 重写以捕获状态码
func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

// Write 重写以捕获响应大小
func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += int64(size)
	return size, err
}

// Logging 请求日志中间件
// 记录每个 HTTP 请求的：方法、路径、状态码、响应大小、耗时
// 使用 slog 结构化日志，方便后续接入日志分析系统
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			// 执行后续处理器
			next.ServeHTTP(rw, r)

			// 记录请求日志
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"size", rw.size,
				"duration", time.Since(start).String(),
			)
		})
	}
}
