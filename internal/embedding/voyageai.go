package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// VoyageAIEmbedding VoyageAI Embedding 实现
type VoyageAIEmbedding struct {
	apiKey  string
	model   string
	baseURL string
	dim     int
	client  *http.Client
}

// VoyageAIConfig VoyageAI 配置
type VoyageAIConfig struct {
	APIKey string
	Model  string // 默认 voyage-code-3
}

// NewVoyageAIEmbedding 创建 VoyageAI Embedding 实例
func NewVoyageAIEmbedding(cfg VoyageAIConfig) *VoyageAIEmbedding {
	if cfg.Model == "" {
		cfg.Model = "voyage-code-3"
	}

	dim := 1024 // voyage-code-3 默认维度
	switch cfg.Model {
	case "voyage-code-3":
		dim = 1024
	case "voyage-3":
		dim = 1024
	case "voyage-3-lite":
		dim = 512
	}

	return &VoyageAIEmbedding{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: "https://api.voyageai.com/v1",
		dim:     dim,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (e *VoyageAIEmbedding) Name() string {
	return fmt.Sprintf("voyageai/%s", e.model)
}

func (e *VoyageAIEmbedding) Dimension() int {
	return e.dim
}

// voyageEmbeddingRequest VoyageAI API 请求体
type voyageEmbeddingRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type,omitempty"` // "document" 或 "query"
}

// voyageEmbeddingResponse VoyageAI API 响应体
type voyageEmbeddingResponse struct {
	Data  []voyageEmbeddingData `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type voyageEmbeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

func (e *VoyageAIEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	// 单条 Embed 视为查询场景
	results, err := e.embedWithType(ctx, []string{text}, "query")
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("VoyageAI embedding 返回空结果")
	}
	return results[0], nil
}

func (e *VoyageAIEmbedding) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// 批量 EmbedBatch 默认视为索引文档场景
	return e.embedWithType(ctx, texts, "document")
}

// EmbedTyped 把 hce TaskType 映射到 VoyageAI 的 input_type
func (e *VoyageAIEmbedding) EmbedTyped(ctx context.Context, texts []string, task TaskType) ([][]float32, error) {
	inputType := "document"
	if task == TaskCodeQuery {
		inputType = "query"
	}
	return e.embedWithType(ctx, texts, inputType)
}

// Voyage 单请求上限：120K tokens / 1000 inputs。留安全余量，按 token 估算分批。
const (
	voyageMaxBatchTokens = 100000
	voyageMaxBatchInputs = 128
)

// voyageEstimateTokens 粗略估算 token（代码约 3 字符/token，偏保守以防超限）
func voyageEstimateTokens(s string) int {
	return len(s)/3 + 1
}

// embedWithType 按 token 上限把 texts 切成多个子批逐个请求，避免大批次超过
// Voyage 单请求 120K token 限制（大文件批次曾整批 400 失败、文件漏索引）。
func (e *VoyageAIEmbedding) embedWithType(ctx context.Context, texts []string, inputType string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); {
		j, tok := i, 0
		for j < len(texts) {
			t := voyageEstimateTokens(texts[j])
			// 单条就超阈值也至少放一条（Voyage 会自行 truncate 到模型上限），避免死循环
			if j > i && (tok+t > voyageMaxBatchTokens || j-i >= voyageMaxBatchInputs) {
				break
			}
			tok += t
			j++
		}
		vecs, err := e.embedOnce(ctx, texts[i:j], inputType)
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
		i = j
	}
	return out, nil
}

func (e *VoyageAIEmbedding) embedOnce(ctx context.Context, texts []string, inputType string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := voyageEmbeddingRequest{
		Input:     texts,
		Model:     e.model,
		InputType: inputType,
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
		return nil, fmt.Errorf("请求 VoyageAI API 失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("VoyageAI API 错误 (状态码 %d): %s", resp.StatusCode, string(respBody))
	}

	var result voyageEmbeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("VoyageAI API 返回错误: %s", result.Error.Message)
	}

	vectors := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < len(vectors) {
			vectors[d.Index] = d.Embedding
		}
	}

	return vectors, nil
}
