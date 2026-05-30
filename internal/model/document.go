package model

import "time"

// DocumentStatus 文档处理状态枚举类型
// 定义文档从上传到可检索的完整生命周期
type DocumentStatus string

const (
	DocStatusPending   DocumentStatus = "pending"   // 刚上传,等待处理
	DocStatusParsing   DocumentStatus = "parsing"   // 正在解析文档内容
	DocStatusChunking  DocumentStatus = "chunking"  // 正在将文本切成小块
	DocStatusEmbedding DocumentStatus = "embedding" // 正在生成向量
	DocStatusCompleted DocumentStatus = "completed" // 处理完成,可以被检索
	DocStatusFailed    DocumentStatus = "failed"    // 处理失败,查看 error_message 了解原因
)

// Document 文档数据模型
type Document struct {
	ID           string         `json:"id"`                      // UUID 主键
	KBID         string         `json:"kb_id"`                   // 所属知识库 ID
	Filename     string         `json:"filename"`                // 原始文件名
	FileType     string         `json:"file_type"`               // 文件类型
	FileSize     int64          `json:"file_size"`               // 文件大小
	Status       DocumentStatus `json:"status"`                  // 当前处理状态
	ErrorMessage string         `json:"error_message,omitempty"` // 处理失败的原因
	CreatedAt    time.Time      `json:"created_at"`              // 创建时间
}
