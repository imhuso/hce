package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// GeminiEmbedding Google Gemini Embedding 实现
type GeminiEmbedding struct {
	apiKey string
	model  string
	dim    int
	client *http.Client

	// 内置限速：避免触发 Gemini paid tier 的 3K RPM / 1M TPM。
	// 留 20% buffer 后取 800K TPM、2400 RPM；本地代码索引几乎用不上 RPM 限速，
	// 真正瓶颈是 TPM。Free tier 用户应通过 config 调小 TPM。
	rpmLimiter *rate.Limiter // 请求数维度
	tpmLimiter *rate.Limiter // token 数维度
}

// GeminiConfig Gemini 配置
type GeminiConfig struct {
	APIKey string
	Model  string // 默认 text-embedding-004
	// 限额覆盖（0 = 用 paid tier 默认；用 free tier 时调小，如 RPM=100 TPM=30000）
	RPM int
	TPM int
}

// NewGeminiEmbedding 创建 Gemini Embedding 实例
func NewGeminiEmbedding(cfg GeminiConfig) *GeminiEmbedding {
	if cfg.Model == "" {
		cfg.Model = "gemini-embedding-001"
	}
	// paid tier 默认（3K RPM / 1M TPM 留 20% buffer）
	rpm := cfg.RPM
	if rpm <= 0 {
		rpm = 2400
	}
	tpm := cfg.TPM
	if tpm <= 0 {
		tpm = 800000
	}

	dim := 3072 // gemini-embedding-001 默认维度
	return &GeminiEmbedding{
		apiKey: cfg.APIKey,
		model:  cfg.Model,
		dim:    dim,
		client: &http.Client{Timeout: 60 * time.Second},
		// 限速器允许一整分钟配额作为初始 burst：冷启动可立即发送，之后按速率补充
		rpmLimiter: rate.NewLimiter(rate.Limit(float64(rpm)/60.0), rpm),
		tpmLimiter: rate.NewLimiter(rate.Limit(float64(tpm)/60.0), tpm),
	}
}

func (e *GeminiEmbedding) Name() string {
	return fmt.Sprintf("gemini/%s", e.model)
}

func (e *GeminiEmbedding) Dimension() int {
	return e.dim
}

// geminiEmbedRequest Gemini 嵌入请求
type geminiEmbedRequest struct {
	Requests []geminiEmbedContentRequest `json:"requests"`
}

type geminiEmbedContentRequest struct {
	Model    string        `json:"model"`
	Content  geminiContent `json:"content"`
	TaskType string        `json:"taskType,omitempty"` // RETRIEVAL_DOCUMENT / CODE_RETRIEVAL_QUERY 等
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

// geminiEmbedResponse Gemini 嵌入响应
type geminiEmbedResponse struct {
	Embeddings []geminiEmbeddingValue `json:"embeddings"`
	Error      *geminiError           `json:"error,omitempty"`
}

type geminiEmbeddingValue struct {
	Values []float32 `json:"values"`
}

type geminiError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// geminiSingleEmbedResponse 单条嵌入响应
type geminiSingleEmbedResponse struct {
	Embedding geminiEmbeddingValue `json:"embedding"`
	Error     *geminiError         `json:"error,omitempty"`
}

// doJSONWithRetry 带退避重试的 JSON POST：网络错误（EOF / reset）和 5xx 自动重试。
func (e *GeminiEmbedding) doJSONWithRetry(ctx context.Context, url string, body []byte) ([]byte, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("创建请求失败: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := e.client.Do(req)
		if err != nil {
			// EOF / connection reset 等瞬态错误重试；context 取消则立即退出
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			lastErr = fmt.Errorf("请求 Gemini API 失败: %w", err)
			log.Printf("[Gemini] ⚠️ 第 %d/%d 次失败: %v", attempt, maxAttempts, err)
		} else {
			respBody, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				lastErr = fmt.Errorf("读取响应失败: %w", readErr)
			} else if resp.StatusCode == http.StatusOK {
				return respBody, nil
			} else if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
				lastErr = fmt.Errorf("Gemini API 错误 (状态码 %d): %s", resp.StatusCode, string(respBody))
				log.Printf("[Gemini] ⚠️ 第 %d/%d 次 %d: %s", attempt, maxAttempts, resp.StatusCode, string(respBody))
			} else {
				// 4xx 等非瞬态错误不重试
				return nil, fmt.Errorf("Gemini API 错误 (状态码 %d): %s", resp.StatusCode, string(respBody))
			}
		}

		if attempt < maxAttempts {
			backoff := time.Duration(1<<(attempt-1)) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return nil, lastErr
}

// estimateTokens 把字节数估算为 Gemini token 数（英文/代码约 3.5 字节/token，
// 偏保守按 3 估算让 limiter 提前生效，避免触发 1M TPM 上限）。
func estimateTokens(byteLen int) int {
	if byteLen <= 0 {
		return 1
	}
	return byteLen / 3
}

// waitLimits 同时等待 RPM 和 TPM 配额。返回 ctx 取消时的错误。
func (e *GeminiEmbedding) waitLimits(ctx context.Context, tokens int) error {
	if e.rpmLimiter != nil {
		if err := e.rpmLimiter.Wait(ctx); err != nil {
			return err
		}
	}
	if e.tpmLimiter != nil && tokens > 0 {
		if err := e.tpmLimiter.WaitN(ctx, tokens); err != nil {
			return err
		}
	}
	return nil
}

// taskTypeStr 把 hce TaskType 映射到 Gemini API 字符串
func taskTypeStr(t TaskType) string {
	switch t {
	case TaskDocument:
		return "RETRIEVAL_DOCUMENT"
	case TaskCodeQuery:
		return "CODE_RETRIEVAL_QUERY"
	default:
		return "" // 不传，让 Gemini 用默认
	}
}

func (e *GeminiEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	// 单条 Embed 视为代码查询场景
	return e.embedSingle(ctx, text, TaskCodeQuery)
}

func (e *GeminiEmbedding) embedSingle(ctx context.Context, text string, task TaskType) ([]float32, error) {
	// 等待限速（query 调用一般 token 数小，但仍要计入 TPM 防全局打爆）
	if err := e.waitLimits(ctx, estimateTokens(len(text))); err != nil {
		return nil, err
	}
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:embedContent?key=%s",
		e.model, e.apiKey,
	)

	contentBlock := map[string]any{
		"parts": []map[string]string{{"text": text}},
	}
	reqBody := map[string]any{
		"model":   fmt.Sprintf("models/%s", e.model),
		"content": contentBlock,
	}
	if tt := taskTypeStr(task); tt != "" {
		reqBody["taskType"] = tt
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	respBody, err := e.doJSONWithRetry(ctx, url, body)
	if err != nil {
		return nil, err
	}

	var result geminiSingleEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("Gemini API 返回错误: %s", result.Error.Message)
	}
	return result.Embedding.Values, nil
}

// geminiBatchLimit Gemini batchEmbedContents 单批上限
const geminiBatchLimit = 100

func (e *GeminiEmbedding) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// 默认视为索引代码片段
	return e.EmbedTyped(ctx, texts, TaskDocument)
}

// EmbedTyped 带 task_type 的批量调用，Gemini 据此选择最合适的向量空间。
func (e *GeminiEmbedding) EmbedTyped(ctx context.Context, texts []string, task TaskType) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	// 超出 Gemini 单批上限就自动拆分递归
	if len(texts) > geminiBatchLimit {
		out := make([][]float32, 0, len(texts))
		for start := 0; start < len(texts); start += geminiBatchLimit {
			end := min(start+geminiBatchLimit, len(texts))
			part, err := e.EmbedTyped(ctx, texts[start:end], task)
			if err != nil {
				return nil, err
			}
			out = append(out, part...)
		}
		return out, nil
	}

	// 等待限速：1 次请求 + 该批的总 token 数
	totalBytes := 0
	for _, t := range texts {
		totalBytes += len(t)
	}
	tokens := estimateTokens(totalBytes)
	if err := e.waitLimits(ctx, tokens); err != nil {
		return nil, err
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:batchEmbedContents?key=%s",
		e.model, e.apiKey,
	)

	tt := taskTypeStr(task)
	requests := make([]geminiEmbedContentRequest, len(texts))
	for i, text := range texts {
		requests[i] = geminiEmbedContentRequest{
			Model: fmt.Sprintf("models/%s", e.model),
			Content: geminiContent{
				Parts: []geminiPart{{Text: text}},
			},
			TaskType: tt,
		}
	}

	reqBody := geminiEmbedRequest{Requests: requests}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	respBody, err := e.doJSONWithRetry(ctx, url, body)
	if err != nil {
		return nil, err
	}

	var result geminiEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("Gemini API 返回错误: %s", result.Error.Message)
	}

	vectors := make([][]float32, len(result.Embeddings))
	for i, emb := range result.Embeddings {
		vectors[i] = emb.Values
	}

	return vectors, nil
}
