package model

// ============================================================
// 统一 API 响应结构
// 所有 HTTP 接口都使用这个信封格式
// 前端通过 code 判断成功（0）或失败（非 0），message 用于展示错误信息
// ============================================================

// Response 统一 API 响应信封
// code=0 表示成功, 非 0 表示业务错误
// data 可以是如何类型(但个对象,列表,分页等结构)
type Response struct {
	Code    int         `json:"code"`    // 业务状态码,0=成功
	Message string      `json:"message"` // 提示信息
	Data    interface{} `json:"data"`    // 响应数据
}

// PagedList 分页列表统一结构
// 所有列表接口(知识库列表、文档列表、对话列表)统一使用这个结构
type PagedList struct {
	List     interface{} `json:"list"`      // 当前页数据
	Total    int64       `json:"total"`     // 总数据量
	Page     int         `json:"page"`      // 当前页码
	PageSize int         `json:"page_Size"` // 每页数量
}

// ============================================================
// 请求 DTO（Data Transfer Object）
// 每个 API 接口的请求体结构
// ============================================================

// CreateKBRequest 创建知识库请求体
// binding tag（如 required）是 gin 的用法，chi 不自动校验
// 这里仅用 json + tag + 手动校验
type CreateKBRequest struct {
	Name        string `json:"name"`        // 知识库名称(必填)
	Description string `json:"description"` // 描述(可选)
}

// UpdateKBRequest 更新知识库请求体
type UpdateKBRequest struct {
	Name        string `json:"name"`        // 新名字
	Description string `json:"description"` // 新描述
}

// CreateConversationRequest 创建对话请求体
type CreateConversationRequest struct {
	KBID string `json:"kb_id"` // 要在哪个知识库下创建对话
}

// ChatRequest 提问请求体
type ChatRequest struct {
	Question string `json:"question"` // 用户提问的内容
}

// ============================================================
// SSE 事件结构（Server-Sent Events）
// 流式问答时，服务端通过 SSE 向浏览器推送以下事件
// ============================================================

// SSEChunkSource 检索到的来源片段
type SSEChunkSource struct {
	Content string  `json:"content"` // 切片文本内容
	Doc     string  `json:"doc"`     // 来源文档名
	Score   float64 `json:"score"`   // 相似度分数
}

// SSESourcesEvent SSE sources 事件数据
// 在 LLM 生成答案前,先发送检索到的参考资料
type SSESourcesEvent struct {
	Chunks []SSEChunkSource `json:"chunks"` // 检索到的片段列表
}

// SSEDeltaEvent SSE delta 事件数据
// LLM 每生成一个 token,就发一个 delta 事件推给前端
type SSEDeltaEvent struct {
	Content string `json:"content"` // 增量文本（通常 1~3 个字符）
}

// SSEDoneEvent SSE done 事件
// 回答完成后发送,包含元信息
type SSEDoneEvent struct {
	MessageID   string `json:"message_id"`   // 保存到数据库的消息 ID
	TotalTokens int    `json:"total_tokens"` // LLM 返回的总 token 消耗
}
