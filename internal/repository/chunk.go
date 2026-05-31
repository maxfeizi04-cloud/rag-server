package repository

import (
	"context"
	"fmt"
	"rag-server/internal/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ChunkRepo struct {
	pool *pgxpool.Pool
}

func NewChunkRepo(pool *pgxpool.Pool) *ChunkRepo {
	return &ChunkRepo{pool: pool}
}

// BatchInsert 批量插入切片和向量
// 整个操作在一个事务中完成，保证数据一致性
// chunks: 切片文本列表
// embeddings: 对应的向量列表，顺序和 chunks 一致，每个向量是 []float64
func (r *ChunkRepo) BatchInsert(ctx context.Context, chunks []model.Chunk, embeddings [][]float64) error {
	// 开启事务
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback(ctx) // 如果 Commit 没执行，自动回滚

	for i, ch := range chunks {
		emb := embeddings[i]
		// pgvector 的 vector 类型可以直接接收 Go 的 []float64 切片
		_, err := tx.Exec(ctx,
			`INSERT INTO chunks (doc_id, kb_id, chunk_index, content, token_count, embedding)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			ch.DocID, ch.KBID, ch.ChunkIndex, ch.Content, ch.TokenCount, emb,
		)
		if err != nil {
			return fmt.Errorf("插入第 %d 个切片失败: %w", i, err)
		}
	}

	return tx.Commit(ctx)
}

// Search 向量相似度检索
// queryEmbedding: 用户问题向量化后的结果
// kbID: 限定在哪个知识库内检索
// topK: 返回最相似的前 K 个片段
// threshold: 最低相似度阈值，低于此值的结果不返回
//
// pgvector 的核心查询：
//
//	embedding <=> $1  → 余弦距离（越小越相似）
//	1 - (embedding <=> $1)  → 余弦相似度（0~1，越大越相似）
func (r *ChunkRepo) Search(ctx context.Context, kbID string, queryEmbedding []float64, topK int, threshold float64) ([]model.ChunkWithScore, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.id, c.doc_id, c.kb_id, c.chunk_index, c.content, c.token_count,
		        c.created_at, d.filename,
		        1 - (c.embedding <=> $1) AS similarity
		 FROM chunks c
		 JOIN documents d ON c.doc_id = d.id    -- JOIN 获取文档名，用于展示引用来源
		 WHERE c.kb_id = $2                       -- 限定知识库范围
		   AND 1 - (c.embedding <=> $1) >= $3    -- 过滤低于阈值的碎片
		 ORDER BY c.embedding <=> $1              -- 按距离升序（越相似越靠前）
		 LIMIT $4`,
		queryEmbedding, kbID, threshold, topK,
	)
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}
	defer rows.Close()

	var results []model.ChunkWithScore
	for rows.Next() {
		var cs model.ChunkWithScore
		if err := rows.Scan(
			&cs.ID, &cs.DocID, &cs.KBID, &cs.ChunkIndex, &cs.Content, &cs.TokenCount,
			&cs.CreatedAt, &cs.Filename, &cs.Score,
		); err != nil {
			return nil, fmt.Errorf("扫描检索结果失败: %w", err)
		}
		results = append(results, cs)
	}
	if results == nil {
		results = []model.ChunkWithScore{}
	}
	return results, nil
}

// ListByDoc 获取某个文档的所有切片（按序号排序）
// 用于前端预览文档被切成了哪些片段
func (r *ChunkRepo) ListByDoc(ctx context.Context, docID string) ([]model.Chunk, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, doc_id, kb_id, chunk_index, content, token_count, created_at
		 FROM chunks WHERE doc_id = $1 ORDER BY chunk_index`, docID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询切片列表失败: %w", err)
	}
	defer rows.Close()

	var chunks []model.Chunk
	for rows.Next() {
		var ch model.Chunk
		if err := rows.Scan(&ch.ID, &ch.DocID, &ch.KBID, &ch.ChunkIndex, &ch.Content, &ch.TokenCount, &ch.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描切片数据失败: %w", err)
		}
		chunks = append(chunks, ch)
	}
	if chunks == nil {
		chunks = []model.Chunk{}
	}
	return chunks, nil
}
