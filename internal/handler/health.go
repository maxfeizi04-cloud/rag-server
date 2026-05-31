package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"rag-server/internal/model"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthHandler 健康检查处理器
// 提供 /api/v1/health 端点,供负载均衡器和监控系统使用
type HealthHandler struct {
	pool *pgxpool.Pool
}

func NewHealthHandler(pool *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{pool: pool}
}

// Check 健康检查
// 检查项：
//   - 服务进程存活（能响应 HTTP 请求本身说明存活）
//   - 数据库连通性（Ping 一下 PostgreSQL）
//
// 状态：
//   - "ok"      → 所有检查通过
//   - "degraded" → 数据库不可达但服务仍在运行
func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	// 给数据库 Ping 设置 2s 超时
	ctx, cancel := context.WithTimeout(r.Context(), time.Second*2)
	defer cancel()

	dbOK := true
	if err := h.pool.Ping(ctx); err != nil {
		dbOK = false
	}

	status := "ok"
	if !dbOK {
		status = "degraded" // 降级状态：服务还在但数据库挂了
	}

	data := map[string]interface{}{
		"status":   status,
		"database": dbOK,
	}

	writeJSON(w, http.StatusOK, model.Response{
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

// ============================================================
// 通用辅助函数（所有 handler 共用）
// ============================================================

// writeJSON 写入统一的 JSON 响应
// 设置 Content-Type 为 application/json，防止浏览器误解析
func writeJSON(w http.ResponseWriter, status int, resp model.Response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// writeError 写入统一的错误响应
// code: 业务错误码（如 40002 表示知识库不存在）
// message: 中文错误描述
func writeError(w http.ResponseWriter, status int, code int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(model.Response{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}
