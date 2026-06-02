package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// OpenAIEmbedding OpenAI Embedding 实现
type OpenAIEmbedding struct {
	apiKey  string
	model   string
	baseURL string
	dim     int
	client  *http.Client
}

// OpenAIConfig OpenAI（及兼容服务，如 LM Studio / vLLM / TEI）配置
type OpenAIConfig struct {
	APIKey  string
	Model   string // OpenAI 默认 text-embedding-3-small；自托管按服务实际 model id 填
	BaseURL string // 默认 https://api.openai.com/v1；LM Studio 一般是 http://host:1234/v1
	Dim     int    // 当未知模型或自托管时手动指定；0 = 用内置映射（仅 OpenAI 已知模型）
}

// NewOpenAIEmbedding 创建 OpenAI 兼容 Embedding 实例
func NewOpenAIEmbedding(cfg OpenAIConfig) *OpenAIEmbedding {
	if cfg.Model == "" {
		cfg.Model = "text-embedding-3-small"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}

	dim := cfg.Dim
	if dim <= 0 {
		// 内置已知 OpenAI 模型映射；其他服务必须显式提供 Dim
		switch cfg.Model {
		case "text-embedding-3-large":
			dim = 3072
		case "text-embedding-3-small", "text-embedding-ada-002":
			dim = 1536
		default:
			dim = 1536
		}
	}

	return &OpenAIEmbedding{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: cfg.BaseURL,
		dim:     dim,
		// 大本地模型（如 LM Studio 跑 Qwen3-8B）单批可能要几分钟
		client: &http.Client{Timeout: 10 * time.Minute},
	}
}

// openaiBatchLimit 单批文本数。OpenAI 云端 100 是普遍上限；本地小模型（nomic 137M）
// 单批 100 也很快。对本地 8B+ 大模型若觉慢可通过环境变量 HCE_OPENAI_BATCH_LIMIT 调小到 16。
const openaiBatchLimit = 100

func (e *OpenAIEmbedding) Name() string {
	return fmt.Sprintf("openai/%s", e.model)
}

func (e *OpenAIEmbedding) Dimension() int {
	return e.dim
}

// openaiEmbeddingRequest OpenAI API 请求体
type openaiEmbeddingRequest struct {
	Input          interface{} `json:"input"` // string 或 []string
	Model          string      `json:"model"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
}

// openaiEmbeddingResponse OpenAI API 响应体
type openaiEmbeddingResponse struct {
	Data  []openaiEmbeddingData `json:"data"`
	Error *openaiError          `json:"error,omitempty"`
}

type openaiEmbeddingData struct {
	// 用 RawMessage 而不是 []float32：JSON 里的 null 会被默认解码成 0，
	// 让 LM Studio 偶发返回的不完整向量（部分 null）静默写入向量库——
	// 这是数据正确性灾难。改成 raw 后逐元素严格校验。
	Embedding []json.RawMessage `json:"embedding"`
	Index     int               `json:"index"`
}

type openaiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (e *OpenAIEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("OpenAI embedding 返回空结果")
	}
	return results[0], nil
}

// EmbedTyped OpenAI 兼容协议没有 task type 概念，退化为 EmbedBatch
func (e *OpenAIEmbedding) EmbedTyped(ctx context.Context, texts []string, _ TaskType) ([][]float32, error) {
	return e.EmbedBatch(ctx, texts)
}

func (e *OpenAIEmbedding) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	// 分批以适配本地大模型推理（OpenAI 云端也受益于更细的进度）
	if len(texts) > openaiBatchLimit {
		out := make([][]float32, 0, len(texts))
		batches := (len(texts) + openaiBatchLimit - 1) / openaiBatchLimit
		for i, start := 0, 0; start < len(texts); i, start = i+1, start+openaiBatchLimit {
			end := start + openaiBatchLimit
			if end > len(texts) {
				end = len(texts)
			}
			t0 := time.Now()
			part, err := e.embedOne(ctx, texts[start:end])
			if err != nil {
				return nil, err
			}
			log.Printf("[OpenAI] 🧠 批 %d/%d  %d 条  耗时=%s", i+1, batches, len(part), time.Since(t0).Round(time.Millisecond))
			out = append(out, part...)
		}
		return out, nil
	}
	return e.embedOne(ctx, texts)
}

func (e *OpenAIEmbedding) embedOne(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := openaiEmbeddingRequest{
		Input:          texts,
		Model:          e.model,
		EncodingFormat: "float",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", e.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.apiKey))

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 OpenAI API 失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API 错误 (状态码 %d): %s", resp.StatusCode, string(respBody))
	}

	var result openaiEmbeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("OpenAI API 返回错误: %s", result.Error.Message)
	}

	// 按 index 排序组装结果，并严格校验每个向量
	vectors := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index >= len(vectors) {
			continue
		}
		// 1) 维度严格匹配（LM Studio 偶发返回 < dim 的 truncated array）
		if len(d.Embedding) != e.dim {
			return nil, fmt.Errorf("embedding 维度异常: 期望 %d，实际 %d（提供商返回了不完整的向量；可能要重启 LM Studio 或换更小模型）",
				e.dim, len(d.Embedding))
		}
		// 2) 逐元素拒绝 null / 非数字
		vec := make([]float32, e.dim)
		for i, raw := range d.Embedding {
			if len(raw) == 0 || string(raw) == "null" {
				return nil, fmt.Errorf("embedding[%d][%d] 是 null（LM Studio 返回不完整向量；常见于本地大模型在内存压力下推理失败）", d.Index, i)
			}
			var f float64
			if err := json.Unmarshal(raw, &f); err != nil {
				return nil, fmt.Errorf("embedding[%d][%d] 解析失败: %w", d.Index, i, err)
			}
			vec[i] = float32(f)
		}
		vectors[d.Index] = vec
	}

	// 3) 没拿到完整的 batch（response.data 漏了某些 index）
	for i := range vectors {
		if vectors[i] == nil {
			return nil, fmt.Errorf("embedding 响应缺失第 %d 条（提供商响应不完整）", i)
		}
	}

	return vectors, nil
}
