package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/imhuso/hce/internal/indexer"
	"github.com/imhuso/hce/pkg/model"
)

// Handler API 处理器
type Handler struct {
	indexer *indexer.Indexer
}

func NewHandler(idx *indexer.Indexer) *Handler {
	return &Handler{indexer: idx}
}

type apiResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type upsertRequest struct {
	CodebaseID string            `json:"codebase_id"`
	Files      []model.PushFile  `json:"files"`
}

type deleteFilesRequest struct {
	CodebaseID    string   `json:"codebase_id"`
	RelativePaths []string `json:"relative_paths"`
}

type searchRequest struct {
	CodebaseID string `json:"codebase_id"`
	Query      string `json:"query"`
	TopK       int    `json:"top_k"`
	Format     string `json:"format"` // json | text
}

// HandleUpsert 接收客户端推送的文件内容并做 chunk 级增量
// POST /api/v1/index/upsert
func (h *Handler) HandleUpsert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	var req upsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	if req.CodebaseID == "" {
		respondError(w, http.StatusBadRequest, "codebase_id 不能为空")
		return
	}
	if len(req.Files) == 0 {
		respondError(w, http.StatusBadRequest, "files 不能为空")
		return
	}
	// 用独立 ctx 避免长任务被客户端断开取消
	stats, err := h.indexer.IndexFiles(context.Background(), req.CodebaseID, req.Files)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, apiResponse{Code: 0, Message: "ok", Data: stats})
}

// HandleDeleteFiles 删除指定文件的索引
// POST /api/v1/index/delete
func (h *Handler) HandleDeleteFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	var req deleteFilesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	if req.CodebaseID == "" {
		respondError(w, http.StatusBadRequest, "codebase_id 不能为空")
		return
	}
	deleted, err := h.indexer.DeleteFiles(context.Background(), req.CodebaseID, req.RelativePaths)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, apiResponse{
		Code: 0, Message: "ok",
		Data: map[string]int{"chunks_deleted": deleted},
	})
}

// HandleSearch 语义搜索
// POST /api/v1/search
func (h *Handler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	if req.CodebaseID == "" || req.Query == "" {
		respondError(w, http.StatusBadRequest, "codebase_id 和 query 不能为空")
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if req.TopK > 100 {
		req.TopK = 100
	}
	results, err := h.indexer.Search(r.Context(), req.CodebaseID, req.Query, req.TopK)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if req.Format == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(formatResultsAsText(results)))
		return
	}
	respondJSON(w, http.StatusOK, apiResponse{Code: 0, Message: "ok", Data: results})
}

// HandleFlush 触发 codebase 的 Milvus flush（让最近写入的 chunk 可见）
// POST /api/v1/index/flush?codebase_id=...
func (h *Handler) HandleFlush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	id := r.URL.Query().Get("codebase_id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "缺少 codebase_id 参数")
		return
	}
	if err := h.indexer.Flush(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, apiResponse{Code: 0, Message: "ok"})
}

// HandleClear 清除整个 codebase 的索引
// DELETE /api/v1/index?codebase_id=...
func (h *Handler) HandleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respondError(w, http.StatusMethodNotAllowed, "仅支持 DELETE")
		return
	}
	id := r.URL.Query().Get("codebase_id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "缺少 codebase_id 参数")
		return
	}
	if err := h.indexer.Clear(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, apiResponse{Code: 0, Message: "已清除"})
}

// HandleListIndexes 列出所有已索引的 codebase（仅暴露 collection 名 + chunk 数）
// GET /api/v1/indexes
func (h *Handler) HandleListIndexes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "仅支持 GET")
		return
	}
	indexes, err := h.indexer.ListIndexes(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, apiResponse{Code: 0, Message: "ok", Data: indexes})
}

// HandleHealth 健康检查
// GET /api/v1/health
func (h *Handler) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, apiResponse{Code: 0, Message: "healthy"})
}

func respondJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, statusCode int, message string) {
	respondJSON(w, statusCode, apiResponse{Code: -1, Message: message})
}

func formatResultsAsText(results []model.SearchResult) string {
	if len(results) == 0 {
		return "No results found.\n"
	}
	var sb strings.Builder
	sb.WriteString("The following code sections were retrieved:\n")
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "Path: %s\n", r.RelativePath)
		lines := strings.Split(r.Content, "\n")
		for j, line := range lines {
			fmt.Fprintf(&sb, "%6d  %s\n", r.StartLine+j, line)
		}
		if i < len(results)-1 {
			sb.WriteString("...\n")
		}
	}
	return sb.String()
}
