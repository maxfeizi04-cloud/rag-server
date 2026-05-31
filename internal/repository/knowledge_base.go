package repository

import (
	"context"
	"fmt"
	"rag-server/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// KnowledgeBaseRepo 知识库数据访问层
// 所有 SQL 操作集中在这个结构体中，不暴露 SQL 细节给上层
type KnowledgeBaseRepo struct {
	pool *pgxpool.Pool // 数据库连接池
}

func NewKnowledgeBaseRepo(pool *pgxpool.Pool) *KnowledgeBaseRepo {
	return &KnowledgeBaseRepo{pool: pool}
}

// Create 创建知识库
// 使用 RETURNING 子句一次完成 INSERT + 读取，避免额外查询
func (r *KnowledgeBaseRepo) Create(ctx context.Context, name, description string) (*model.KnowledgeBase, error) {
	var kb model.KnowledgeBase
	err := r.pool.QueryRow(ctx,
		`INSERT INTO knowledge_bases (name, description) VALUES ($1, $2)
		 RETURNING id, name, description, created_at, updated_at`,
		name, description,
	).Scan(&kb.ID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("创建知识库失败: %w", err)
	}
	return &kb, nil
}

// GetByID 根据 ID 查询知识库
// 返回 nil, nil 表示记录不存在（区别于数据库错误）
func (r *KnowledgeBaseRepo) GetByID(ctx context.Context, id string) (*model.KnowledgeBase, error) {
	var kb model.KnowledgeBase
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description, created_at, updated_at FROM knowledge_bases WHERE id = $1`,
		id,
	).Scan(&kb.ID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt)
	if err != nil {
		// pgx.ErrNoRows 表示查询成功但没找到记录，不是错误，返回 nil
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询知识库失败: %w", err)
	}
	return &kb, nil
}

// List 分页查询知识库列表
// 先 COUNT 总数，再查当前页数据
// 按创建时间倒序排列（最新的在前）
func (r *KnowledgeBaseRepo) List(ctx context.Context, page, pageSize int) ([]model.KnowledgeBase, int64, error) {
	// 第一步：查询总记录数
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM knowledge_bases`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("统计知识库数量失败: %w", err)
	}

	// 第二步：查询当前页数据
	offset := (page - 1) * pageSize
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, description, created_at, updated_at
		 FROM knowledge_bases ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		pageSize, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("查询知识库列表失败: %w", err)
	}
	defer rows.Close()

	var kbs []model.KnowledgeBase
	for rows.Next() {
		var kb model.KnowledgeBase
		if err := rows.Scan(&kb.ID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("扫描知识库行数据失败: %w", err)
		}
		kbs = append(kbs, kb)
	}
	// 确保返回空数组而非 null（方便前端处理）
	if kbs == nil {
		kbs = []model.KnowledgeBase{}
	}
	return kbs, total, nil
}

// Update 更新知识库名称和描述
// 自动更新 updated_at 时间戳
func (r *KnowledgeBaseRepo) Update(ctx context.Context, id, name, description string) (*model.KnowledgeBase, error) {
	var kb model.KnowledgeBase
	err := r.pool.QueryRow(ctx,
		`UPDATE knowledge_bases SET name = $1, description = $2, updated_at = now()
		 WHERE id = $3
		 RETURNING id, name, description, created_at, updated_at`,
		name, description, id,
	).Scan(&kb.ID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // 要更新的记录不存在
		}
		return nil, fmt.Errorf("更新知识库失败: %w", err)
	}
	return &kb, nil
}

// Delete 删除知识库
// ON DELETE CASCADE 确保级联删除所有关联的文档、切片、对话、消息
func (r *KnowledgeBaseRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM knowledge_bases WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("删除知识库失败: %w", err)
	}
	return nil
}
