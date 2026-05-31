package service

import (
	"context"
	"fmt"
	"rag-server/internal/config"
	"rag-server/internal/infrastructure/embedding"
	"rag-server/internal/model"
	"rag-server/internal/repository"
	"strings"
	"unicode/utf8"
)

// PipelineService RAG 管道服务
// 负责文本切片、Token 估算、向量检索
type PipelineService struct {
	embedder  *embedding.Embedder   // Embedding 客户端
	chunkRepo *repository.ChunkRepo // 切片数据访问
	cfg       config.RAGConfig      // RAG 参数配置
}

func NewPipelineService(embedder *embedding.Embedder, chunkRepo *repository.ChunkRepo, cfg config.RAGConfig) *PipelineService {
	return &PipelineService{embedder: embedder, chunkRepo: chunkRepo, cfg: cfg}
}

// SplitText 将长文本按策略切分成多个片段
//
// 切分策略（按优先级依次尝试）：
//  1. 段落边界（\n\n）— 最自然的分割点
//  2. 换行符（\n）— 次级分割点
//  3. 句号/问号/感叹号 — 句子级分割
//  4. 字符数硬截断 — 兜底方案
//
// 重叠机制：
//
//	每个新切片会包含前一片段末尾的 overlap 个字符
//	这确保了如果一个答案的关键信息恰好落在边界上，至少有一个切片包含完整上下文
func (s *PipelineService) SplitText(text string) []string {
	chunkSize := s.cfg.ChunkSize  // 每个切片的目标大小（tokens）
	overlap := s.cfg.ChunkOverlap // 重叠大小（tokens）

	// 第一步：按段落分割
	paragraphs := strings.Split(text, "\n\n")

	var chunks []string
	var current strings.Builder // 当前正在构建的切片

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// 如果加入当前段落后超过 chunk_size，先保存当前切片
		if utf8.RuneCountInString(current.String())+utf8.RuneCountInString(para) > chunkSize && current.Len() > 0 {
			chunks = append(chunks, strings.TrimSpace(current.String()))
			current.Reset()
			// 添加重叠：保留前一个切片末尾的部分字符
			if overlap > 0 && len(chunks) > 0 {
				prev := chunks[len(chunks)-1]
				runes := []rune(prev)
				if len(runes) > overlap {
					current.WriteString(string(runes[len(runes)-overlap:]))
				} else {
					current.WriteString(prev)
				}
			}
		}

		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(para)

		// 如果累积内容远超过 chunk_size（一个段落本身就很大），按句子再细分
		if utf8.RuneCountInString(current.String()) > chunkSize*2 {
			sentences := splitBySentence(current.String())
			current.Reset()
			for _, sent := range sentences {
				if utf8.RuneCountInString(current.String())+utf8.RuneCountInString(sent) > chunkSize && current.Len() > 0 {
					chunks = append(chunks, strings.TrimSpace(current.String()))
					current.Reset()
				}
				if current.Len() > 0 {
					current.WriteString(" ")
				}
				current.WriteString(sent)
			}
		}
	}

	// 最后一段
	if current.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	}

	return chunks
}

// splitBySentence 按中文标点分割句子
// 识别：。？！. ? !
func splitBySentence(text string) []string {
	var result []string
	start := 0
	runes := []rune(text)
	for i, r := range runes {
		if r == '。' || r == '？' || r == '！' || r == '.' || r == '?' || r == '!' {
			sent := strings.TrimSpace(string(runes[start : i+1]))
			if sent != "" {
				result = append(result, sent)
			}
			start = i + 1 // 下一句从标点符号之后开始
		}
	}
	// 最后一句（没有标点结尾的部分）
	if start < len(runes) {
		sent := strings.TrimSpace(string(runes[start:]))
		if sent != "" {
			result = append(result, sent)
		}
	}
	return result
}

// EstimateTokens 粗略估算文本的 Token 数量
//
// 估算规则（经验值，非精确计算）：
//   - 中文字符：1 字 ≈ 1 token
//   - 英文单词：4 字符 ≈ 1 token
//   - 空格和换行不计入
//
// 为什么不用 tiktoken 精确计算？
//
//	这里的 token 估算仅用于判断切片是否超长、是否继续追加内容
//	精确计算需要调 tiktoken 库（CGO/额外依赖），V1 不值得
func EstimateTokens(text string) int {
	chineseCount := 0
	otherCount := 0
	for _, r := range text {
		if r >= 0x4e00 && r <= 0x9fff {
			// CJK 统一汉字区间（U+4E00 ~ U+9FFF）
			chineseCount++
		} else if r != ' ' && r != '\n' && r != '\t' {
			otherCount++
		}
	}
	return chineseCount + otherCount/4
}

// SearchRelevant 检索与问题最相关的文档片段
//
// 流程：
//  1. 调 Embedding API 将问题转为向量
//  2. 用 pgvector 的余弦相似度在知识库中检索 Top K 个最相似的切片
//  3. 返回带相似度分数和来源文档名的结果列表
func (s *PipelineService) SearchRelevant(ctx context.Context, question, kbID string) ([]model.ChunkWithScore, error) {
	// 第一步：问题向量化
	vec, err := s.embedder.Embed(ctx, question)
	if err != nil {
		return nil, fmt.Errorf("问题向量化失败: %w", err)
	}

	// 第二步：向量检索
	return s.chunkRepo.Search(ctx, kbID, vec, s.cfg.TopK, s.cfg.SimilarityThreshold)
}
