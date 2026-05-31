package service

import (
	"context"
	"encoding/json"
	"fmt"
	"rag-server/internal/config"
	"rag-server/internal/infrastructure/llm"
	"rag-server/internal/model"
	"rag-server/internal/repository"
	"strings"
)

// ConversationService 对话服务
// 编排 RAG 问答的完整流程
type ConversationService struct {
	convRepo *repository.ConversationRepo // 对话/消息数据访问
	pipeline *PipelineService             // 切片 + 检索管道
	llm      *llm.Client                  // LLM 客户端
	cfg      config.RAGConfig             // RAG 配置
}

func NewConversationService(
	convRepo *repository.ConversationRepo,
	pipeline *PipelineService,
	llmClient *llm.Client,
	cfg config.RAGConfig,
) *ConversationService {
	return &ConversationService{
		convRepo: convRepo,
		pipeline: pipeline,
		llm:      llmClient,
		cfg:      cfg,
	}
}

func (s *ConversationService) Create(ctx context.Context, kbID string) (*model.Conversation, error) {
	return s.convRepo.Create(ctx, kbID, "新对话")
}

func (s *ConversationService) Get(ctx context.Context, id string) (*model.Conversation, error) {
	conv, err := s.convRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, fmt.Errorf("对话不存在")
	}
	return conv, nil
}

func (s *ConversationService) List(ctx context.Context, kbID string, page, pageSize int) ([]model.Conversation, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.convRepo.List(ctx, kbID, page, pageSize)
}

// GetMessages 获取对话历史（分页）
func (s *ConversationService) GetMessages(ctx context.Context, conversationID string, page, pageSize int) ([]model.Message, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	msgs, err := s.convRepo.GetMessages(ctx, conversationID, pageSize*page)
	if err != nil {
		return nil, 0, err
	}
	// 简单分页：获取最近 N 条再按页切分
	total := int64(len(msgs))
	start := (page - 1) * pageSize
	if start >= len(msgs) {
		return []model.Message{}, total, nil
	}
	end := start + pageSize
	if end > len(msgs) {
		end = len(msgs)
	}
	return msgs[start:end], total, nil
}

// Ask 执行 RAG 问答（核心方法）
//
// 参数：
//
//	conversationID: 对话 ID
//	question: 用户的问题文本
//	onSources: 检索到参考资料后的回调（用于 SSE sources 事件）
//	onDelta: LLM 每生成一个 token 时的回调（用于 SSE delta 事件）
//
// 返回：
//
//	*model.Message: 保存的助手消息
//	int: LLM 消耗的总 token 数
//	error: 任何步骤的错误
func (s *ConversationService) Ask(
	ctx context.Context,
	conversationID string,
	question string,
	onSources func([]model.SSEChunkSource) error, // 检索结果回调
	onDelta func(string) error, // 流式文本回调
) (*model.Message, int, error) {
	// 第 1 步：查询对话信息（获取 kb_id）
	conv, err := s.Get(ctx, conversationID)
	if err != nil {
		return nil, 0, err
	}

	// 第 2 步：保存用户问题到数据库
	userMsg, err := s.convRepo.AddMessage(ctx, conversationID, "user", question, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("保存用户消息失败: %w", err)
	}

	// 第 3 步：向量检索相关文档片段
	chunks, err := s.pipeline.SearchRelevant(ctx, question, conv.KBID)
	if err != nil {
		return nil, 0, fmt.Errorf("知识库检索失败: %w", err)
	}

	// 第 4 步：构建 sources 事件数据并发送
	var sources []model.SSEChunkSource
	for _, ch := range chunks {
		sources = append(sources, model.SSEChunkSource{
			Content: ch.Content,
			Doc:     ch.Filename,
			Score:   ch.Score,
		})
	}
	if onSources != nil {
		if err := onSources(sources); err != nil {
			return nil, 0, err
		}
	}

	// 第 5 步：拼接 Prompt
	// 5a. 构建参考资料上下文
	contextText := buildContext(chunks)

	// 5b. 获取对话历史（最近 N 轮）
	historyMsgs, _ := s.convRepo.GetRecentMessages(ctx, conversationID, s.cfg.MaxHistoryRounds)
	historyText := buildHistory(historyMsgs)

	// 5c. 用实际文本替换 Prompt 模板中的占位符
	systemPrompt := strings.Replace(s.cfg.SystemPrompt, "{context}", contextText, 1)
	systemPrompt = strings.Replace(systemPrompt, "{history}", historyText, 1)
	systemPrompt = strings.Replace(systemPrompt, "{question}", question, 1)

	messages := []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: question},
	}

	// 第 6 步：调用 LLM（流式）
	var fullAnswer strings.Builder // 累积完整回答文本
	totalTokens := 0

	if onDelta != nil {
		// 流式模式：每生成一个 token 就回调
		totalTokens, err = s.llm.ChatStream(ctx, messages, func(delta string) error {
			fullAnswer.WriteString(delta)
			return onDelta(delta) // 将增量文本传给 SSE handler
		})
	} else {
		// 非流式模式（备用）：等待完整回答
		result, chatErr := s.llm.Chat(ctx, messages)
		if chatErr != nil {
			return nil, 0, fmt.Errorf("LLM 调用失败: %w", chatErr)
		}
		fullAnswer.WriteString(result.Content)
		totalTokens = result.TotalTokens
	}

	if err != nil {
		return nil, 0, fmt.Errorf("LLM 流式生成失败: %w", err)
	}

	// 第 7 步：保存助手回答到数据库
	sourcesJSON, _ := json.Marshal(sources)
	assistantMsg, err := s.convRepo.AddMessage(ctx, conversationID, "assistant", fullAnswer.String(), sourcesJSON)
	if err != nil {
		return nil, 0, fmt.Errorf("保存助手回答失败: %w", err)
	}

	// 第 8 步：如果是第一条有效消息，自动设置对话标题
	if conv.Title == "新对话" {
		title := question
		runes := []rune(title)
		if len(runes) > 30 { // 标题最长 30 个字符
			title = string(runes[:30]) + "..."
		}
		s.convRepo.UpdateTitle(ctx, conversationID, title)
	}

	_ = userMsg // 避免未使用变量的编译警告
	return assistantMsg, totalTokens, nil
}

// buildContext 将检索到的切片拼接成参考资料文本
// 格式：【来源1：文档名】\n内容\n\n【来源2：文档名】\n内容
func buildContext(chunks []model.ChunkWithScore) string {
	if len(chunks) == 0 {
		return "（知识库中暂无相关参考资料）"
	}
	var parts []string
	for i, ch := range chunks {
		parts = append(parts, fmt.Sprintf("【来源%d：%s】\n%s", i+1, ch.Filename, ch.Content))
	}
	return strings.Join(parts, "\n\n")
}

// buildHistory 将消息历史拼接为对话文本
// messages 是倒序的（最新的在前），需要反转后再拼接
func buildHistory(messages []model.Message) string {
	if len(messages) == 0 {
		return "（无历史对话）"
	}
	// 反转消息顺序（数据库中是倒序，需要变回正序）
	var parts []string
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role == "user" {
			parts = append(parts, "用户："+m.Content)
		} else {
			parts = append(parts, "助手："+m.Content)
		}
	}
	return strings.Join(parts, "\n")
}
