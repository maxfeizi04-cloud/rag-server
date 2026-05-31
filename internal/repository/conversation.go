package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"rag-server/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ConversationRepo struct {
	pool *pgxpool.Pool
}

func NewConversationRepo(pool *pgxpool.Pool) *ConversationRepo {
	return &ConversationRepo{pool: pool}
}

// Create 创建新对话
func (r *ConversationRepo) Create(ctx context.Context, kbID, title string) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.pool.QueryRow(ctx,
		`INSERT INTO conversations (kb_id, title) VALUES ($1, $2)
		 RETURNING id, kb_id, title, created_at, updated_at`,
		kbID, title,
	).Scan(&conv.ID, &conv.KBID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("创建对话失败: %w", err)
	}
	return &conv, nil
}

// GetByID 查询对话
func (r *ConversationRepo) GetByID(ctx context.Context, id string) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.pool.QueryRow(ctx,
		`SELECT id, kb_id, title, created_at, updated_at FROM conversations WHERE id = $1`, id,
	).Scan(&conv.ID, &conv.KBID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询对话失败: %w", err)
	}
	return &conv, nil
}

// List 查询知识库下的对话列表（按最后更新时间倒序）
func (r *ConversationRepo) List(ctx context.Context, kbID string, page, pageSize int) ([]model.Conversation, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM conversations WHERE kb_id = $1`, kbID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("统计对话数量失败: %w", err)
	}

	offset := (page - 1) * pageSize
	rows, err := r.pool.Query(ctx,
		`SELECT id, kb_id, title, created_at, updated_at
		 FROM conversations WHERE kb_id = $1 ORDER BY updated_at DESC LIMIT $2 OFFSET $3`,
		kbID, pageSize, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("查询对话列表失败: %w", err)
	}
	defer rows.Close()

	var convs []model.Conversation
	for rows.Next() {
		var c model.Conversation
		if err := rows.Scan(&c.ID, &c.KBID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("扫描对话数据失败: %w", err)
		}
		convs = append(convs, c)
	}
	if convs == nil {
		convs = []model.Conversation{}
	}
	return convs, total, nil
}

// UpdateTitle 更新对话标题（首次问答后自动设置）
func (r *ConversationRepo) UpdateTitle(ctx context.Context, id, title string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE conversations SET title = $1, updated_at = now() WHERE id = $2`,
		title, id,
	)
	return err
}

// ============================================================
// 消息相关操作
// ============================================================

// AddMessage 添加一条消息（用户提问或助手回答）
// sources: JSON 格式的引用来源，可为 nil（用户消息通常无来源）
func (r *ConversationRepo) AddMessage(ctx context.Context, conversationID, role, content string, sources json.RawMessage) (*model.Message, error) {
	var msg model.Message
	err := r.pool.QueryRow(ctx,
		`INSERT INTO messages (conversation_id, role, content, sources)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, conversation_id, role, content, sources, created_at`,
		conversationID, role, content, sources,
	).Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.Sources, &msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("添加消息失败: %w", err)
	}
	return &msg, nil
}

// GetMessages 获取对话的消息历史（最近的在前）
func (r *ConversationRepo) GetMessages(ctx context.Context, conversationID string, limit int) ([]model.Message, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, conversation_id, role, content, sources, created_at
		 FROM messages WHERE conversation_id = $1
		 ORDER BY created_at DESC LIMIT $2`,
		conversationID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("查询消息历史失败: %w", err)
	}
	defer rows.Close()

	var msgs []model.Message
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Sources, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描消息数据失败: %w", err)
		}
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []model.Message{}
	}
	return msgs, nil
}

// GetRecentMessages 获取最近 N 轮对话历史
// rounds: 保留的对话轮数（每轮 = user + assistant 两条消息）
func (r *ConversationRepo) GetRecentMessages(ctx context.Context, conversationID string, rounds int) ([]model.Message, error) {
	limit := rounds * 2 // 每轮两条消息
	return r.GetMessages(ctx, conversationID, limit)
}
