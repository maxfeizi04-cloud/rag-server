package handler

import (
	"net/http"
	"rag-server/internal/model"
	"rag-server/internal/service"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// DocumentHandler 文档 HTTP 处理器
type DocumentHandler struct {
	svc *service.DocumentService
}

func NewDocumentHandler(svc *service.DocumentService) *DocumentHandler {
	return &DocumentHandler{svc: svc}
}

// Upload 上传文档
// POST /api/v1/knowledge-bases/{id}/documents
// Content-Type: multipart/form-data
// 字段名: file
func (h *DocumentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	kbID := chi.URLParam(r, "id")

	// 解析 multipart 表单，限制最大 32MB
	// 32 << 20 = 33554432 字节 ≈ 32MB
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, 41000, "文件上传解析失败，文件可能过大（最大 32MB）")
		return
	}

	// 获取上传的文件
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, 41001, "未找到上传文件，请确保表单字段名为 'file'")
		return
	}
	defer file.Close()

	// 调用 Service 层处理上传
	doc, err := h.svc.Upload(r.Context(), kbID, header.Filename, file, header.Size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, 41002, err.Error())
		return
	}

	// 201 Created：资源创建成功
	// 注意：此时文档状态为 pending，后台正在异步处理
	writeJSON(w, http.StatusCreated, model.Response{
		Code:    0,
		Message: "ok",
		Data:    doc,
	})
}

// Get 获取文档详情（含处理状态）
// GET /api/v1/documents/{id}
func (h *DocumentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	doc, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, 41003, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.Response{
		Code:    0,
		Message: "ok",
		Data:    doc,
	})
}

// List 获取知识库下的文档列表
// GET /api/v1/knowledge-bases/{id}/documents?page=1&page_size=20
func (h *DocumentHandler) List(w http.ResponseWriter, r *http.Request) {
	kbID := chi.URLParam(r, "id")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	docs, total, err := h.svc.ListByKB(r.Context(), kbID, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, 41004, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.Response{
		Code:    0,
		Message: "ok",
		Data: model.PagedList{
			List:     docs,
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		},
	})
}

// Delete 删除文档
// DELETE /api/v1/documents/{id}
func (h *DocumentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, 41003, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.Response{
		Code:    0,
		Message: "ok",
		Data:    nil,
	})
}
