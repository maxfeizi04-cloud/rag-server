package repository

import (
	"context"
	"fmt"
	"rag-server/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DocumentRepo struct {
	pool *pgxpool.Pool
}

func NewDocumentRepo(pool *pgxpool.Pool) *DocumentRepo {
	return &DocumentRepo{pool: pool}
}

// Create 创建文档记录，初始状态为 pending
func (r *DocumentRepo) Create(ctx context.Context, kbID, filename, fileType string, fileSize int64) (*model.Document, error) {
	var doc model.Document
	err := r.pool.QueryRow(ctx,
		`INSERT INTO documents (kb_id, filename, file_type, file_size, status)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, kb_id, filename, file_type, file_size, status, created_at`,
		kbID, filename, fileType, fileSize, model.DocStatusPending,
	).Scan(&doc.ID, &doc.KBID, &doc.Filename, &doc.FileType, &doc.FileSize, &doc.Status, &doc.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("创建文档记录失败: %w", err)
	}
	return &doc, nil
}

// GetByID 查询单个文档
// error_message 字段可能为 NULL，用指针处理
func (r *DocumentRepo) GetByID(ctx context.Context, id string) (*model.Document, error) {
	var doc model.Document
	var errMsg *string // 用指针接收可为 NULL 的字段
	err := r.pool.QueryRow(ctx,
		`SELECT id, kb_id, filename, file_type, file_size, status, error_message, created_at
		 FROM documents WHERE id = $1`, id,
	).Scan(&doc.ID, &doc.KBID, &doc.Filename, &doc.FileType, &doc.FileSize, &doc.Status, &errMsg, &doc.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询文档失败: %w", err)
	}
	if errMsg != nil {
		doc.ErrorMessage = *errMsg
	}
	return &doc, nil
}

// ListByKB 查询某个知识库下的所有文档（分页）
func (r *DocumentRepo) ListByKB(ctx context.Context, kbID string, page, pageSize int) ([]model.Document, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM documents WHERE kb_id = $1`, kbID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("统计文档数量失败: %w", err)
	}

	offset := (page - 1) * pageSize
	rows, err := r.pool.Query(ctx,
		`SELECT id, kb_id, filename, file_type, file_size, status, COALESCE(error_message, ''), created_at
		 FROM documents WHERE kb_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		kbID, pageSize, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("查询文档列表失败: %w", err)
	}
	defer rows.Close()

	var docs []model.Document
	for rows.Next() {
		var doc model.Document
		if err := rows.Scan(&doc.ID, &doc.KBID, &doc.Filename, &doc.FileType, &doc.FileSize, &doc.Status, &doc.ErrorMessage, &doc.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("扫描文档行数据失败: %w", err)
		}
		docs = append(docs, doc)
	}
	if docs == nil {
		docs = []model.Document{}
	}
	return docs, total, nil
}

// UpdateStatus 更新文档处理状态
// 这是异步处理管道中的关键方法，Goroutine 通过它向前端报告进度
func (r *DocumentRepo) UpdateStatus(ctx context.Context, id string, status model.DocumentStatus, errMsg string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE documents SET status = $1, error_message = $2 WHERE id = $3`,
		status, errMsg, id,
	)
	if err != nil {
		return fmt.Errorf("更新文档状态失败: %w", err)
	}
	return nil
}

// Delete 删除文档（级联删切片）
func (r *DocumentRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM documents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("删除文档失败: %w", err)
	}
	return nil
}
