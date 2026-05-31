package llm

import (
	"context"
	"fmt"
	"io"
	"rag-server/internal/config"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// Client DeepSeek 大模型客户端
// 封装 OpenAI 兼容的 Chat Completions API
//
//	支持流式(ChatStream)和非流式(Chat)两种调用方式
type Client struct {
	client *openai.Client        // OpenAI SDK 客户端实例
	cfg    config.DeepSeekConfig // 模型配置(模型名,温度等)
}

// NewClient 创建 DeepSeek 客户端
// cfg: 从 config.yaml 加载配置模型
// apikey: DeepSeek API 密钥,格式 sk-xxx
func NewClient(cfg config.DeepSeekConfig, apikey string) *Client {
	// 配置 OpenAI SDK 的连接参数
	opts := []option.RequestOption{
		option.WithAPIKey(apikey),       // API 密钥(必须)
		option.WithBaseURL(cfg.BaseURL), //	自定义 BaseURL，指向 DeepSeek 的地址
	}
	return &Client{
		client: openai.NewClient(opts...),
		cfg:    cfg,
	}
}

// ChatMessage 对话消息,简化版的 OpenAI message 结构
// 不直接暴露 OpenAI SDK 的类型,方便以后切换 LMM 厂商
type ChatMessage struct {
	Role    string // system / user / assistant
	Content string // 消息文本
}

// StreamCallback 流式回调函数类型
// LLM 每生成一个 token 片段就调用一次这个回调
// 回调返回 error 时停止流式读取（例如前端断开连接）
type StreamCallback func(delta string) error

// ChatResult 非流式对话的返回结果
type ChatResult struct {
	Content     string // 完整回答文本
	TotalTokens int    // 本次请求消耗的总 token 数
}

// Chat 非流式对话
// 发送 messages 给 LLM，等待完整回答后返回
// 用于不需要流式输出的场景（如生成对话标题）
func (c *Client) Chat(ctx context.Context, messages []ChatMessage) (*ChatResult, error) {
	// 将内部 ChatMessage 转换为 OpenAI SDK 的消息格式
	// openai-go 使用强类型 Union 类型，不同 role 用不同构造函数
	openaiMsgs := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, m := range messages {
		switch m.Role {
		case "system":
			// SystemMessage：设置 AI 的行为和规则
			openaiMsgs[i] = openai.SystemMessage(m.Content)
		case "assistant":
			// AssistantMessage：AI 之前的回答
			openaiMsgs[i] = openai.AssistantMessage(m.Content)
		default:
			// UserMessage: 用户的提问
			openaiMsgs[i] = openai.UserMessage(m.Content)
		}

	}

	// 构建 API 请求参数
	params := openai.ChatCompletionNewParams{
		Model:       openai.F(c.cfg.ChatModel),        // 模型名称
		Messages:    openai.F(openaiMsgs),             // 消息列表
		MaxTokens:   openai.F(int64(c.cfg.MaxTokens)), // 最大生成 token 数
		Temperature: openai.F(c.cfg.Temperature),      // 生成温度
	}

	// 调用 API
	resp, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("DeepSeek 对话请求失败: %w", err)
	}

	// 检查是否有有效返回
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("DeepSeek 对话: 未返回任何回答")
	}

	return &ChatResult{
		Content:     resp.Choices[0].Message.Content,
		TotalTokens: int(resp.Usage.TotalTokens),
	}, nil
}

// ChatStream 流式对话
// 发送 messages 给 LLM，每收到一个 token 就调用 callback
// callback 返回 error 时立即中止（用于处理客户端断开连接等场景）
// 返回消耗的总 token 数
func (c *Client) ChatStream(ctx context.Context, messages []ChatMessage, callback StreamCallback) (int, error) {
	// 转换消息格式（同 Chat 方法）
	openaiMsgs := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, m := range messages {
		switch m.Role {
		case "system":
			openaiMsgs[i] = openai.SystemMessage(m.Content)
		case "assistant":
			openaiMsgs[i] = openai.AssistantMessage(m.Content)
		default:
			openaiMsgs[i] = openai.UserMessage(m.Content)
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:       openai.F(c.cfg.ChatModel),
		Messages:    openai.F(openaiMsgs),
		MaxTokens:   openai.F(int64(c.cfg.MaxTokens)),
		Temperature: openai.F(c.cfg.Temperature),
	}

	// 启动流式请求
	// NewStreaming 返回一个迭代器，通过 Next() 逐个获取 chunk
	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close() // 确保连接关闭

	var fullContent strings.Builder // 累积完整回答（用于保存到数据库）
	totalTokens := 0

	// 循环获取每个 token 片段
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			if delta != "" {
				fullContent.WriteString(delta)
				// 通过回调将增量文本传给调用方（通常是 SSE handler）
				if err := callback(delta); err != nil {
					return 0, fmt.Errorf("流式回调失败: %w", err)
				}
			}
		}
		// DeepSeek 在最后一个 chunk 返回 usage 信息
		if chunk.Usage.TotalTokens > 0 {
			totalTokens = int(chunk.Usage.TotalTokens)
		}
	}

	// 检查流是否正常结束
	if err := stream.Err(); err != nil {
		if err == io.EOF {
			return totalTokens, nil // EOF 是正常的流结束信号
		}
		return 0, fmt.Errorf("DeepSeek 流式请求失败: %w", err)
	}

	return totalTokens, nil
}
