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

// OllamaEmbedding Ollama 本地 Embedding 实现
type OllamaEmbedding struct {
	model   string
	baseURL string
	dim     int
	client  *http.Client
}

// OllamaConfig Ollama 配置
type OllamaConfig struct {
	Model   string // 默认 nomic-embed-text
	BaseURL string // 默认 http://localhost:11434
	Dim     int    // 向量维度（不同模型不同）
}

// NewOllamaEmbedding 创建 Ollama Embedding 实例
func NewOllamaEmbedding(cfg OllamaConfig) *OllamaEmbedding {
	if cfg.Model == "" {
		cfg.Model = "nomic-embed-text"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434"
	}
	if cfg.Dim <= 0 {
		// 常见模型默认维度
		switch cfg.Model {
		case "nomic-embed-text":
			cfg.Dim = 768
		case "mxbai-embed-large":
			cfg.Dim = 1024
		case "all-minilm":
			cfg.Dim = 384
		default:
			cfg.Dim = 768
		}
	}

	return &OllamaEmbedding{
		model:   cfg.Model,
		baseURL: cfg.BaseURL,
		dim:     cfg.Dim,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (e *OllamaEmbedding) Name() string {
	return fmt.Sprintf("ollama/%s", e.model)
}

func (e *OllamaEmbedding) Dimension() int {
	return e.dim
}

// ollamaEmbedRequest Ollama embed API 请求体
type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// ollamaEmbedResponse Ollama embed API 响应体
type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Error      string      `json:"error,omitempty"`
}

func (e *OllamaEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := ollamaEmbedRequest{
		Model: e.model,
		Input: text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("%s/api/embed", e.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Ollama API 失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API 错误 (状态码 %d): %s", resp.StatusCode, string(respBody))
	}

	var result ollamaEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("Ollama API 返回错误: %s", result.Error)
	}

	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("Ollama API 返回空 embeddings")
	}

	return result.Embeddings[0], nil
}

// EmbedTyped Ollama 没有 task type 概念，退化为 EmbedBatch
func (e *OllamaEmbedding) EmbedTyped(ctx context.Context, texts []string, _ TaskType) ([][]float32, error) {
	return e.EmbedBatch(ctx, texts)
}

func (e *OllamaEmbedding) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// Ollama 的 embed API 不原生支持批量，逐个调用
	results := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vec, err := e.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("批量 embedding 失败: %w", err)
		}
		results = append(results, vec)
	}
	return results, nil
}
