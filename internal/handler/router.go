package handler

import (
	"log/slog"
	"net/http"

	"rag-server/internal/middleware"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware" // chi 内置中间件
)

// NewRouter 创建并配置 chi 路由器
// 注册所有中间件和路由
// 返回的 chi.Router 可以直接传给 http.Server
func NewRouter(
	kbHandler *KnowledgeBaseHandler,
	docHandler *DocumentHandler,
	convHandler *ConversationHandler,
	healthHandler *HealthHandler,
	logger *slog.Logger,
) chi.Router {
	r := chi.NewRouter()

	// ============================================================
	// 全局中间件（按顺序执行）
	// ============================================================

	// 1. RequestID：为每个请求生成唯一 ID，方便追踪
	r.Use(chimw.RequestID)

	// 2. Recovery：捕获 panic，防止整个服务挂掉
	r.Use(middleware.Recovery(logger))

	// 3. CORS：跨域配置
	r.Use(middleware.CORS)

	// 4. Logging：记录每个请求的日志
	r.Use(middleware.Logging(logger))

	// 注意：不设置全局超时（chi 的 Timeout 中间件会中断 SSE 长连接）
	// SSE 的超时由 http.Server.WriteTimeout 控制（120s）

	// ============================================================
	// API v1 路由组
	// ============================================================
	r.Route("/api/v1", func(r chi.Router) {
		// --- 健康检查 ---
		r.Get("/health", healthHandler.Check)

		// --- 知识库管理 ---
		r.Route("/knowledge-bases", func(r chi.Router) {
			r.Post("/", kbHandler.Create)       // 创建知识库
			r.Get("/", kbHandler.List)          // 知识库列表
			r.Get("/{id}", kbHandler.Get)       // 知识库详情
			r.Put("/{id}", kbHandler.Update)    // 更新知识库
			r.Delete("/{id}", kbHandler.Delete) // 删除知识库

			// 文档管理（嵌套在知识库路径下）
			r.Post("/{id}/documents", docHandler.Upload) // 上传文档
			r.Get("/{id}/documents", docHandler.List)    // 文档列表
		})

		// --- 文档管理（独立路由，不与知识库嵌套） ---
		r.Get("/documents/{id}", docHandler.Get)       // 文档详情
		r.Delete("/documents/{id}", docHandler.Delete) // 删除文档

		// --- 对话和问答 ---
		r.Post("/conversations", convHandler.Create)                   // 创建对话
		r.Get("/conversations", convHandler.List)                      // 对话列表
		r.Get("/conversations/{id}/messages", convHandler.GetMessages) // 消息历史
		r.Post("/conversations/{id}/chat", convHandler.Chat)           // SSE 流式问答（核心）
	})

	// ============================================================
	// 静态文件服务（前端页面）
	// ============================================================
	// 将 ./web 目录下的文件作为静态资源提供
	// 访问 http://localhost:8080/ 即打开前端页面
	fs := http.FileServer(http.Dir("./web"))
	r.Handle("/*", fs)

	return r
}
