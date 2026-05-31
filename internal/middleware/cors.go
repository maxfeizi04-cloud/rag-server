package middleware

import "net/http"

// CORS 跨域资源共享中间件
// V1 阶段开放所有来源，方便前后端分离开发
// 生产环境应限制 Access-Control-Allow-Origin 为具体域名
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// 预检请求（OPTIONS）直接返回 204，不需要进业务处理
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
