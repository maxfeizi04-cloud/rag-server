package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"rag-server/internal/config"
	"rag-server/internal/handler"
	"rag-server/internal/infrastructure/embedding"
	"rag-server/internal/infrastructure/llm"
	"rag-server/internal/infrastructure/parser"
	"rag-server/internal/repository"
	"rag-server/internal/service"
	"syscall"
	"time"
)

func main() {
	// ============================================================
	// 1. 初始化日志
	// ============================================================
	// 使用 slog 结构化日志，JSON 格式输出到 stdout
	// 生产环境中日志会被 Docker / systemd 收集
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// ============================================================
	// 2. 加载配置
	// ============================================================
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml" // 默认在当前目录查找
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("加载配置失败", "error", err)
		os.Exit(1)
	}

	// ============================================================
	// 3. 检查必要的环境变量
	// ============================================================
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		logger.Error("环境变量 DEEPSEEK_API_KEY 未设置，无法启动")
		os.Exit(1)
	}

	// ============================================================
	// 4. 初始化数据库连接池
	// ============================================================
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := repository.NewPool(ctx, cfg.Database)
	if err != nil {
		logger.Error("数据库连接失败", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("数据库连接成功")

	// ============================================================
	// 5. 初始化基础设施组件
	// ============================================================
	llmClient := llm.NewClient(cfg.DeepSeek, apiKey)        // DeepSeek Chat 客户端
	embedder := embedding.NewEmbedder(cfg.DeepSeek, apiKey) // Embedding 客户端

	// 注册文档解析器（PDF 和 DOCX 需要额外加载）
	parser.Register("pdf", parser.NewPDFParser())
	parser.Register("docx", &parser.DOCXParser{})

	// ============================================================
	// 6. 初始化 Repository 层
	// ============================================================
	kbRepo := repository.NewKnowledgeBaseRepo(pool)
	docRepo := repository.NewDocumentRepo(pool)
	chunkRepo := repository.NewChunkRepo(pool)
	convRepo := repository.NewConversationRepo(pool)

	// ============================================================
	// 7. 初始化 Service 层（依赖注入）
	// ============================================================
	// Pipeline Service 被 DocumentService 和 ConversationService 共用
	pipelineSvc := service.NewPipelineService(embedder, chunkRepo, cfg.RAG)
	kbSvc := service.NewKnowledgeBaseService(kbRepo)
	docSvc := service.NewDocumentService(docRepo, chunkRepo, pipelineSvc, embedder, cfg.RAG, "./data/uploads")
	convSvc := service.NewConversationService(convRepo, pipelineSvc, llmClient, cfg.RAG)

	// ============================================================
	// 8. 初始化 Handler 层
	// ============================================================
	kbHandler := handler.NewKnowledgeBaseHandler(kbSvc)
	docHandler := handler.NewDocumentHandler(docSvc)
	convHandler := handler.NewConversationHandler(convSvc)
	healthHandler := handler.NewHealthHandler(pool)

	// ============================================================
	// 9. 创建路由器
	// ============================================================
	router := handler.NewRouter(kbHandler, docHandler, convHandler, healthHandler, logger)

	// ============================================================
	// 10. 配置 HTTP 服务器
	// ============================================================
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,  // 请求读取超时（含请求体）
		WriteTimeout: 120 * time.Second, // 响应写入超时（SSE 长连接需要较长超时）
		IdleTimeout:  60 * time.Second,  // Keep-Alive 空闲超时
	}

	// ============================================================
	// 11. 优雅关停
	// ============================================================
	// 在独立 Goroutine 中监听系统信号
	// 收到 SIGINT (Ctrl+C) 或 SIGTERM (Docker stop) 时触发关停
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("收到关停信号，正在优雅关闭...", "signal", sig.String())

		// 给现有请求 15 秒时间完成
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("服务器关闭出错", "error", err)
		}
	}()

	// ============================================================
	// 12. 启动服务器
	// ============================================================
	logger.Info("服务器启动", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		// ErrServerClosed 是正常关停触发的错误，不是真正的异常
		logger.Error("服务器异常退出", "error", err)
		os.Exit(1)
	}
	logger.Info("服务器已停止")
}
