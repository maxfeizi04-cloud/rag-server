package model

import "encoding/json"

// Conversation 对话数据模型
// 对应数据库 conversations 表
type Conversation struct {
	ID        string `json:"id"`
	KBID      string `json:"kb_id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Message 详细数据模型
// 对应数据库 messages 表
type Message struct {
	ID             string          `json:"id"`
	ConversationID string          `json:"conversation_id"`
	Role           string          `json:"role"` // user / assistant
	Content        string          `json:"content"`
	Sources        json.RawMessage `json:"sources,omitempty"`
	CreatedAt      string          `json:"created_at"`
}
