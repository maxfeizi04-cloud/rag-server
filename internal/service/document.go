package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"rag-server/internal/config"
	"rag-server/internal/infrastructure/embedding"
	"rag-server/internal/infrastructure/parser"
	"rag-server/internal/model"
	"rag-server/internal/repository"
	"time"

	"github.com/google/uuid"
)

// DocumentService 文档管理服务
// 负责文档上传、异步处理和状态管理
type DocumentService struct {
	docRepo   *repository.DocumentRepo
	chunkRepo *repository.ChunkRepo
	pipeline  *PipelineService    // 切片 + 检索管道
	embedder  *embedding.Embedder // Embedding 客户端（处理时逐条生成向量）
	cfg       config.RAGConfig    // RAG 配置参数
	uploadDir string              // 上传文件存储目录
}

func NewDocumentService(
	docRepo *repository.DocumentRepo,
	chunkRepo *repository.ChunkRepo,
	pipeline *PipelineService,
	embedder *embedding.Embedder,
	cfg config.RAGConfig,
	uploadDir string,
) *DocumentService {
	return &DocumentService{
		docRepo:   docRepo,
		chunkRepo: chunkRepo,
		pipeline:  pipeline,
		embedder:  embedder,
		cfg:       cfg,
		uploadDir: uploadDir,
	}
}

// Upload 上传文档并启动异步处理
//
// 流程：
//  1. 保存文件到磁盘（UUID 重命名，避免冲突）
//  2. 在数据库创建文档记录（status=pending）
//  3. 启动后台 Goroutine 异步处理
//  4. 立即返回文档信息给前端
//
// 前端拿到 doc.id 后，可以轮询 GET /api/v1/documents/{id} 查看处理进度
func (s *DocumentService) Upload(ctx context.Context, kbID string, filename string, reader io.Reader, fileSize int64) (*model.Document, error) {
	// 提取文件扩展名并确定类型
	ext := filepath.Ext(filename)           // 如 ".pdf"
	fileType := stringsTrimPrefix(ext, ".") // 去掉点号 → "pdf"

	// 确保上传目录存在（递归创建、所有者读写执行）
	if err := os.MkdirAll(s.uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("创建上传目录失败: %w", err)
	}

	// 用 UUID 重命名文件存储，避免：
	//   1. 文件名冲突（两个用户上传同名文件）
	//   2. 路径穿越攻击（恶意文件名如 ../../etc/passwd）
	savedName := uuid.New().String() + ext
	savedPath := filepath.Join(s.uploadDir, savedName)

	// 写入文件
	f, err := os.Create(savedPath)
	if err != nil {
		return nil, fmt.Errorf("创建文件失败: %w", err)
	}
	if _, err := io.Copy(f, reader); err != nil {
		f.Close()
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}
	f.Close()

	// 在数据库创建文档记录
	doc, err := s.docRepo.Create(ctx, kbID, filename, fileType, fileSize)
	if err != nil {
		os.Remove(savedPath) // 数据库创建失败，清理磁盘文件
		return nil, err
	}

	// 启动后台 Goroutine 异步处理（不阻塞上传响应）
	// 使用 context.Background() 而非请求 ctx，因为 HTTP 请求返回后 ctx 会被取消
	go s.processDocument(doc.ID, doc.KBID, fileType, savedPath)

	return doc, nil
}

// processDocument 后台异步处理文档的完整流程
//
// 状态流转：
//
//	pending → parsing → chunking → embedding → completed
//	任何步骤失败 → failed（记录 error_message）
//
// 超时保护：整个处理流程最多 10 分钟
func (s *DocumentService) processDocument(docID, kbID, fileType, filePath string) {
	// 使用独立的 context，设置 10 分钟超时
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	logger := slog.With("doc_id", docID)
	logger.Info("开始处理文档")

	// === 第 1 步：解析文档 ===
	s.docRepo.UpdateStatus(ctx, docID, model.DocStatusParsing, "")
	text, err := parser.Parse(fileType, filePath)
	if err != nil {
		s.docRepo.UpdateStatus(ctx, docID, model.DocStatusFailed, err.Error())
		logger.Error("文档解析失败", "error", err)
		return
	}
	if text == "" {
		s.docRepo.UpdateStatus(ctx, docID, model.DocStatusFailed, "文档内容为空，无法提取文本")
		return
	}

	// === 第 2 步：文本切片 ===
	s.docRepo.UpdateStatus(ctx, docID, model.DocStatusChunking, "")
	chunkTexts := s.pipeline.SplitText(text)
	if len(chunkTexts) == 0 {
		s.docRepo.UpdateStatus(ctx, docID, model.DocStatusFailed, "文档切分后无有效内容")
		return
	}

	// === 第 3 步：生成 Embedding 向量 ===
	s.docRepo.UpdateStatus(ctx, docID, model.DocStatusEmbedding, "")
	chunks := make([]model.Chunk, len(chunkTexts))
	embeddings := make([][]float64, len(chunkTexts))

	for i, ct := range chunkTexts {
		// 调 Embedding API 将文本转为向量
		emb, err := s.embedder.Embed(ctx, ct)
		if err != nil {
			s.docRepo.UpdateStatus(ctx, docID, model.DocStatusFailed, fmt.Sprintf("生成向量失败: %v", err))
			logger.Error("Embedding 失败", "error", err, "chunk_index", i)
			return
		}
		// 构建切片记录
		chunks[i] = model.Chunk{
			DocID:      docID,
			KBID:       kbID,
			ChunkIndex: i,
			Content:    ct,
			TokenCount: EstimateTokens(ct),
		}
		embeddings[i] = emb
	}

	// === 第 4 步：批量存入数据库 ===
	if err := s.chunkRepo.BatchInsert(ctx, chunks, embeddings); err != nil {
		s.docRepo.UpdateStatus(ctx, docID, model.DocStatusFailed, fmt.Sprintf("存储切片失败: %v", err))
		logger.Error("存储切片失败", "error", err)
		return
	}

	// === 第 5 步：标记完成 ===
	s.docRepo.UpdateStatus(ctx, docID, model.DocStatusCompleted, "")
	logger.Info("文档处理完成", "chunk_count", len(chunks))
}

// Get 查询文档详情
func (s *DocumentService) Get(ctx context.Context, id string) (*model.Document, error) {
	doc, err := s.docRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("文档不存在")
	}
	return doc, nil
}

// ListByKB 查询知识库下的文档列表
func (s *DocumentService) ListByKB(ctx context.Context, kbID string, page, pageSize int) ([]model.Document, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.docRepo.ListByKB(ctx, kbID, page, pageSize)
}

// Delete 删除文档
func (s *DocumentService) Delete(ctx context.Context, id string) error {
	doc, err := s.docRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if doc == nil {
		return fmt.Errorf("文档不存在")
	}
	return s.docRepo.Delete(ctx, id)
}

// stringsTrimPrefix 去掉字符串的前缀
// 用于将 ".pdf" 转为 "pdf"
func stringsTrimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}
