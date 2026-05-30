package model

import "time"

// Chunk 文本切片数据模型
// 对应数据库 chunks,每个 chunk 是文档的一部分
type Chunk struct {
	ID         string    `json:"id"`
	DocID      string    `json:"doc_id"`      // 所属文档 ID
	KBID       string    `json:"kb_id"`       // 所属知识库 ID
	ChunkIndex int       `json:"chunk_index"` // 在原文档中的序号
	Content    string    `json:"content"`     // 切片文本内容
	TokenCount int       `json:"token_count"` // 估算 token 数
	Embedding  []float32 `json:"embedding,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ChunkWithScore 带相似度分数的切片,用于检索结果返回
// 相比 Chunk 多了文档名和相似度分数
type ChunkWithScore struct {
	Chunk            // 嵌入 Chunk 结构体，包含所有基础字段
	Score    float64 `json:"score"` // 余弦相似度分数(0~1,越高越相关)
	Filename string  `json:"filename"` // 来源文档名
}
