package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/imhuso/hce/pkg/model"
)

// HTTP HCE 服务端 HTTP 客户端
type HTTP struct {
	BaseURL string
	Client  *http.Client
}

// NewHTTP 用默认超时（5 分钟）创建客户端
func NewHTTP(baseURL string) *HTTP {
	return &HTTP{
		BaseURL: baseURL,
		// 单次 HTTP 请求 90 秒上限。upsert 每批 50 文件，server 端理论 < 30s 完成。
		// 超时后 doJSON 会触发指数退避重试。
		Client: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

func (h *HTTP) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	endpoint := h.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint, rdr)
	if err != nil {
		return err
	}
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// 简单重试：网络错误 / 5xx 重试 4 次。429 用更长退避（且尊重 Retry-After）
	const maxAttempts = 5
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		// 重新构造 body reader（重试时第二次 read 不到）
		if body != nil && i > 0 {
			buf, _ := json.Marshal(body)
			req, _ = http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(buf))
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := h.Client.Do(req)
		var backoff time.Duration
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			// 关键：HTTP client.Timeout（90s）触发的 timeout 错误也不重试——
			// 服务端可能仍在处理这个请求，重试会让它做第二遍（重复 EMB / 写入）。
			// 网络层快速失败（拒连、reset）才重试。
			var ue *url.Error
			if errors.As(err, &ue) && ue.Timeout() {
				return fmt.Errorf("请求超时，不重试以避免服务端重复处理: %w", err)
			}
			lastErr = err
			backoff = time.Duration(1<<i) * 500 * time.Millisecond
		} else {
			respBody, _ := io.ReadAll(resp.Body)
			retryAfter := resp.Header.Get("Retry-After")
			resp.Body.Close()
			if resp.StatusCode == 200 {
				if out == nil || len(respBody) == 0 {
					return nil
				}
				return json.Unmarshal(respBody, out)
			}
			if resp.StatusCode == 429 {
				lastErr = fmt.Errorf("HTTP 429（限流）: %s", string(respBody))
				backoff = parseRetryAfter(retryAfter)
				if backoff <= 0 {
					backoff = time.Duration(1<<i)*5*time.Second + 5*time.Second // 5s/10s/20s/40s/80s
				}
			} else if resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504 {
				// 仅基础设施级临时错误重试。500（应用错误，如坏数据）不重试，
				// 重试同样数据只会无限循环且浪费资源。
				lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
				backoff = time.Duration(1<<i) * 500 * time.Millisecond
			} else {
				return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
			}
		}
		if i < maxAttempts-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return lastErr
}

// parseRetryAfter 把 Retry-After header 解析为时长（仅支持秒数；HTTP-date 暂略）
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	return 0
}

type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// Health 检查服务可达
func (h *HTTP) Health(ctx context.Context) error {
	var r apiResponse
	if err := h.doJSON(ctx, http.MethodGet, "/health", nil, &r); err != nil {
		return err
	}
	if r.Code != 0 {
		return fmt.Errorf("server: %s", r.Message)
	}
	return nil
}

// Upsert 推送一批文件
func (h *HTTP) Upsert(ctx context.Context, codebaseID string, files []model.PushFile) (*model.UpsertResult, error) {
	var r apiResponse
	body := map[string]any{"codebase_id": codebaseID, "files": files}
	if err := h.doJSON(ctx, http.MethodPost, "/index/upsert", body, &r); err != nil {
		return nil, err
	}
	if r.Code != 0 {
		return nil, fmt.Errorf("upsert: %s", r.Message)
	}
	var stats model.UpsertResult
	if err := json.Unmarshal(r.Data, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// DeleteFiles 删除指定相对路径
func (h *HTTP) DeleteFiles(ctx context.Context, codebaseID string, paths []string) (int, error) {
	var r apiResponse
	body := map[string]any{"codebase_id": codebaseID, "relative_paths": paths}
	if err := h.doJSON(ctx, http.MethodPost, "/index/delete", body, &r); err != nil {
		return 0, err
	}
	if r.Code != 0 {
		return 0, fmt.Errorf("delete: %s", r.Message)
	}
	var d struct {
		ChunksDeleted int `json:"chunks_deleted"`
	}
	if err := json.Unmarshal(r.Data, &d); err != nil {
		return 0, err
	}
	return d.ChunksDeleted, nil
}

// Search 语义搜索（json 格式）
func (h *HTTP) Search(ctx context.Context, codebaseID, query string, topK int) ([]model.SearchResult, error) {
	var r apiResponse
	body := map[string]any{
		"codebase_id": codebaseID,
		"query":       query,
		"top_k":       topK,
	}
	if err := h.doJSON(ctx, http.MethodPost, "/search", body, &r); err != nil {
		return nil, err
	}
	if r.Code != 0 {
		return nil, fmt.Errorf("search: %s", r.Message)
	}
	var out []model.SearchResult
	if err := json.Unmarshal(r.Data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SearchText 直接拿到 text 格式的字符串（保留服务端的 path:line 排版）
func (h *HTTP) SearchText(ctx context.Context, codebaseID, query string, topK int) (string, error) {
	body := map[string]any{
		"codebase_id": codebaseID,
		"query":       query,
		"top_k":       topK,
		"format":      "text",
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.BaseURL+"/search", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return string(respBody), nil
}

// Flush 触发服务端落盘 growing segment（sync 完成时调一次）
func (h *HTTP) Flush(ctx context.Context, codebaseID string) error {
	var r apiResponse
	q := url.Values{}
	q.Set("codebase_id", codebaseID)
	if err := h.doJSON(ctx, http.MethodPost, "/index/flush?"+q.Encode(), nil, &r); err != nil {
		return err
	}
	if r.Code != 0 {
		return fmt.Errorf("flush: %s", r.Message)
	}
	return nil
}

// Clear 清除整个 codebase
func (h *HTTP) Clear(ctx context.Context, codebaseID string) error {
	var r apiResponse
	q := url.Values{}
	q.Set("codebase_id", codebaseID)
	if err := h.doJSON(ctx, http.MethodDelete, "/index?"+q.Encode(), nil, &r); err != nil {
		return err
	}
	if r.Code != 0 {
		return fmt.Errorf("clear: %s", r.Message)
	}
	return nil
}

// List 列出所有已索引的集合
func (h *HTTP) List(ctx context.Context) ([]model.IndexInfo, error) {
	var r apiResponse
	if err := h.doJSON(ctx, http.MethodGet, "/indexes", nil, &r); err != nil {
		return nil, err
	}
	if r.Code != 0 {
		return nil, fmt.Errorf("list: %s", r.Message)
	}
	var out []model.IndexInfo
	if err := json.Unmarshal(r.Data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
