package repository

import (
	"context"
	"fmt"
	"rag-server/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool 创建 PostgreSQL 连接池
// pgxpool 是协程安全的连接池，在应用启动时创建一次，整个生命周期复用
func NewPool(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	// 从 DSN 创建连接池配置
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("解析数据库连接串失败: %w", err)
	}

	// 设置最大连接数
	// 20 对中等规模应用足够，可根据实际负载调整
	poolCfg.MaxConns = 20

	// 创建连接池
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("创建数据库连接池失败: %w", err)
	}

	// 启动时 Ping 一下数据库，确保网络和认证都没问题
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	return pool, nil
}
