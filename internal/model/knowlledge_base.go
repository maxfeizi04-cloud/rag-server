package model

// knowledgeBase 知识库数据模型
// 对应数据库 knowledge_base 表
type KnowledgeBase struct {
	ID          string `json:"id"`          //	UUID 主键
	Name        string `json:"name"`        // 知识库名称
	Description string `json:"description"` // 描述
	CreatedAt   string `json:"created_at"`  //	创建时间
	UpdatedAt   string `json:"updated_at"`  // 最后更新时间
}
