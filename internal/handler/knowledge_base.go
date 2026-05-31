package handler

import (
	"encoding/json"
	"net/http"
	"rag-server/internal/model"
	"rag-server/internal/service"
	"strconv"
)

package handler

import (
"encoding/json"
"net/http"
"rag-server/internal/model"
"rag-server/internal/service"
"strconv"

"github.com/go-chi/chi/v5"
)

// KnowledgeBaseHandler 知识库 HTTP 处理器
// 每个方法对应一个 API 端点
type KnowledgeBaseHandler struct {
	svc *service.KnowledgeBaseService
}

func NewKnowledgeBaseHandler(svc *service.KnowledgeBaseService) *KnowledgeBaseHandler {
	return &KnowledgeBaseHandler{svc: svc}
}

// Create 创建知识库
// POST /api/v1/knowledge-bases
func (h *KnowledgeBaseHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateKBRequest
	// 解析 JSON 请求体
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, 40000, "请求参数格式错误，请检查 JSON 结构")
		return
	}

	kb, err := h.svc.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, 40001, err.Error())
		return
	}

	// 201 Created：资源创建成功的标准响应码
	writeJSON(w, http.StatusCreated, model.Response{
		Code:    0,
		Message: "ok",
		Data:    kb,
	})
}

// Get 获取知识库详情
// GET /api/v1/knowledge-bases/{id}
func (h *KnowledgeBaseHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id") // 从 URL 路径中提取 {id} 参数

	kb, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, 40002, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.Response{
		Code:    0,
		Message: "ok",
		Data:    kb,
	})
}

// List 获取知识库列表
// GET /api/v1/knowledge-bases?page=1&page_size=20
func (h *KnowledgeBaseHandler) List(w http.ResponseWriter, r *http.Request) {
	// 从查询参数获取分页信息，解析失败时用默认值
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	kbs, total, err := h.svc.List(r.Context(), page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, 40003, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.Response{
		Code:    0,
		Message: "ok",
		Data: model.PagedList{
			List:     kbs,
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		},
	})
}

// Update 更新知识库
// PUT /api/v1/knowledge-bases/{id}
func (h *KnowledgeBaseHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req model.UpdateKBRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, 40000, "请求参数格式错误")
		return
	}

	kb, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusNotFound, 40002, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.Response{
		Code:    0,
		Message: "ok",
		Data:    kb,
	})
}

// Delete 删除知识库
// DELETE /api/v1/knowledge-bases/{id}
// 级联删除：所有关联的文档、切片、对话、消息都会被删除
func (h *KnowledgeBaseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, 40002, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.Response{
		Code:    0,
		Message: "ok",
		Data:    nil,
	})
}