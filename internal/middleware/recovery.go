package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recovery Panic 恢复中间件
// 捕获 handler 中未处理的 panic，记录堆栈信息，返回 500 错误
// 防止单个请求的 panic 导致整个 HTTP 服务崩溃
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// 记录完整的错误信息和调用栈
					logger.Error("panic recovered",
						"error", rec,
						"stack", string(debug.Stack()),
					)
					// 返回统一的错误 JSON（不使用 writeError 因为可能 handler 尚未初始化）
					http.Error(w, `{"code":50000,"message":"内部服务器错误","data":null}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
