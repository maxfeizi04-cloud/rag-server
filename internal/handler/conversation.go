package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"rag-server/internal/model"
	"rag-server/internal/service"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// ConversationHandler 对话和消息 HTTP 处理器
type ConversationHandler struct {
	svc *service.ConversationService
}

func NewConversationHandler(svc *service.ConversationService) *ConversationHandler {
	return &ConversationHandler{svc: svc}
}

// Create 创建新对话
// POST /api/v1/conversations
// Body: {"kb_id": "xxx"}
func (h *ConversationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, 42000, "请求参数格式错误")
		return
	}

	conv, err := h.svc.Create(r.Context(), req.KBID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, 42001, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, model.Response{
		Code:    0,
		Message: "ok",
		Data:    conv,
	})
}

// List 获取对话列表
// GET /api/v1/conversations?kb_id=xxx&page=1&page_size=20
func (h *ConversationHandler) List(w http.ResponseWriter, r *http.Request) {
	kbID := r.URL.Query().Get("kb_id")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	convs, total, err := h.svc.List(r.Context(), kbID, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, 42002, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.Response{
		Code:    0,
		Message: "ok",
		Data: model.PagedList{
			List:     convs,
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		},
	})
}

// GetMessages 获取消息历史
// GET /api/v1/conversations/{id}/messages?page=1&page_size=50
func (h *ConversationHandler) GetMessages(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	msgs, total, err := h.svc.GetMessages(r.Context(), id, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, 42003, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.Response{
		Code:    0,
		Message: "ok",
		Data: model.PagedList{
			List:     msgs,
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		},
	})
}

// Chat SSE 流式问答接口（核心端点）
// POST /api/v1/conversations/{id}/chat
// Body: {"question": "入职流程需要哪些材料？"}
//
// SSE（Server-Sent Events）协议要点：
//  1. Content-Type 必须是 text/event-stream
//  2. 每条消息格式: "event: <事件名>\ndata: <JSON数据>\n\n"
//  3. 每次写完必须调用 Flusher.Flush() 推送数据到客户端
//  4. 连接保持打开直到回答完成或出错
func (h *ConversationHandler) Chat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// 解析请求体
	var req model.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, 42000, "请求参数格式错误")
		return
	}

	if req.Question == "" {
		writeError(w, http.StatusBadRequest, 42004, "问题不能为空")
		return
	}

	// === 设置 SSE 必需的 HTTP 头 ===
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache") // 禁止缓存
	w.Header().Set("Connection", "keep-alive")  // 保持连接
	w.Header().Set("X-Accel-Buffering", "no")   // 禁用 Nginx 缓冲（否则 sse 会被缓冲成一大块）

	// Flusher 是 SSE 的生命线 —— 没有它，数据只会在缓冲区堆积，浏览器永远收不到
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, 50000, "当前服务器不支持 SSE 流式响应")
		return
	}

	// 调用 Service 层执行问答，通过回调函数发送 SSE 事件
	_, _, err := h.svc.Ask(r.Context(), id, req.Question,
		// 回调 1：发送检索到的参考资料
		func(sources []model.SSEChunkSource) error {
			event := model.SSESourcesEvent{Chunks: sources}
			data, _ := json.Marshal(event)
			// SSE 格式: "event: <事件名>\ndata: <JSON>\n\n"
			fmt.Fprintf(w, "event: sources\ndata: %s\n\n", data)
			flusher.Flush() // 立即推送给浏览器
			return nil
		},
		// 回调 2：发送 LLM 生成的增量文本
		func(delta string) error {
			event := model.SSEDeltaEvent{Content: delta}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: delta\ndata: %s\n\n", data)
			flusher.Flush()
			return nil
		},
	)

	// 如果出错，发送 error 事件
	if err != nil {
		errData, _ := json.Marshal(map[string]interface{}{
			"code":    42005,
			"message": err.Error(),
		})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", errData)
		flusher.Flush()
		return
	}

	// 完成信号
	fmt.Fprintf(w, "event: done\ndata: {}\n\n")
	flusher.Flush()
}
