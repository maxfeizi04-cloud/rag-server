package embedding

import (
	"context"
	"fmt"
	"rag-server/internal/config"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// Embedder 文本向量化客户端
// 将任意文本转换为固定维度的浮点数向量
// 向量之间的余弦距离代表语义相似度
type Embedder struct {
	client *openai.Client        // 复用 OpenAI SDK 客户端
	cfg    config.DeepSeekConfig // 模型配置
}

// NewEmbedder 创建 Embedding 客户端
// 和 LLM 客户端共享同一个 BaseURL 和 API Key
func NewEmbedder(cfg config.DeepSeekConfig, apiKey string) *Embedder {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(cfg.BaseURL),
	}
	return &Embedder{
		client: openai.NewClient(opts...),
		cfg:    cfg,
	}
}

// Embed 将单个文本转换为向量
// text: 任意文本（通常是一段切片内容或用户问题）
// 返回: 1536 维浮点数向量（维度取决于 Embedding 模型）
func (e *Embedder) Embed(ctx context.Context, text string) ([]float64, error) {
	return e.EmbedBatch(ctx, []string{text})
}

// EmbedBatch 批量文本向量化
// 每次调用实际上传多个文本到 API，但 V1 版本每次只取结果的第一个向量
// 后续可优化为真正的批量调用以节省 API 开销
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([]float64, error) {
	// 将 []string 转换为 OpenAI SDK 需要的输入格式
	inputs := make([]openai.EmbeddingNewParamsInputUnion, len(texts))
	for i, t := range texts {
		// F 函数创建泛型参数，InputArrayOfStrings 表示输入类型是字符串数组
		inputs[i] = openai.F[openai.EmbeddingNewParamsInputUnion](
			openai.EmbeddingNewParamsInputArrayOfStrings(t),
		)
	}

	params := openai.EmbeddingNewParams{
		Input: openai.F(inputs),
		Model: openai.F(e.cfg.EmbeddingModel), // 使用配置中指定的 Embedding 模型
	}

	resp, err := e.client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("Embedding 请求失败: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("Embedding: 未返回任何向量")
	}

	// resp.Data[0].Embedding 是一个 []float64，长度 = 模型维度（如 1536）
	return resp.Data[0].Embedding, nil
}
