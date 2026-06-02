package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 应用配置
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	VectorDB  VectorDBConfig  `yaml:"vectordb"`
	Splitter  SplitterConfig  `yaml:"splitter"`
}

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// EmbeddingConfig Embedding 配置
type EmbeddingConfig struct {
	Provider string `yaml:"provider"` // openai | ollama | voyageai | gemini
	Model    string `yaml:"model"`
	APIKey   string `yaml:"api_key"`
	BaseURL  string `yaml:"base_url"`
	Dim      int    `yaml:"dim"`           // 仅 Ollama / 自托管 OpenAI 兼容服务需要手动指定
	RPM      int    `yaml:"rpm,omitempty"` // 限速：每分钟请求数（0=用 provider 默认）
	TPM      int    `yaml:"tpm,omitempty"` // 限速：每分钟 tokens（0=用 provider 默认；free tier 应调小）
}

// VectorDBConfig 向量数据库配置
type VectorDBConfig struct {
	Provider string `yaml:"provider"` // milvus
	Address  string `yaml:"address"`
	Token    string `yaml:"token"`
}

// SplitterConfig 切分器配置（push 模式下，文件扫描在客户端做，这里只关心切分参数）
type SplitterConfig struct {
	MaxChunkSize  int `yaml:"max_chunk_size"`
	ChunkOverlap  int `yaml:"chunk_overlap"`
	MinChunkBytes int `yaml:"min_chunk_bytes"` // 小于该字节数的 chunk 在服务端被过滤；0 = 不过滤
}

// Load 从 YAML 文件加载配置，环境变量可覆盖
func Load(path string) (*Config, error) {
	cfg := defaultConfig()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("读取配置文件失败: %w", err)
			}
		} else if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件失败: %w", err)
		}
	}
	cfg.overrideFromEnv()
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{Port: 9527, Host: "0.0.0.0"},
		Embedding: EmbeddingConfig{
			Provider: "gemini",
			Model:    "gemini-embedding-001",
		},
		VectorDB: VectorDBConfig{Provider: "milvus", Address: "localhost:19530"},
		Splitter: SplitterConfig{MaxChunkSize: 5000, ChunkOverlap: 300, MinChunkBytes: 100},
	}
}

func (c *Config) overrideFromEnv() {
	if v := os.Getenv("HCE_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &c.Server.Port)
	}
	if v := os.Getenv("HCE_HOST"); v != "" {
		c.Server.Host = v
	}

	if v := os.Getenv("HCE_EMBEDDING_PROVIDER"); v != "" {
		c.Embedding.Provider = v
	}
	if v := os.Getenv("HCE_EMBEDDING_MODEL"); v != "" {
		c.Embedding.Model = v
	}
	if v := os.Getenv("HCE_EMBEDDING_API_KEY"); v != "" {
		c.Embedding.APIKey = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" && c.Embedding.APIKey == "" {
		c.Embedding.APIKey = v
	}
	if v := os.Getenv("GEMINI_API_KEY"); v != "" && c.Embedding.Provider == "gemini" {
		c.Embedding.APIKey = v
	}
	if v := os.Getenv("VOYAGEAI_API_KEY"); v != "" && c.Embedding.Provider == "voyageai" {
		c.Embedding.APIKey = v
	}
	if v := os.Getenv("HCE_EMBEDDING_BASE_URL"); v != "" {
		c.Embedding.BaseURL = v
	}
	if v := os.Getenv("HCE_EMBEDDING_DIM"); v != "" {
		fmt.Sscanf(v, "%d", &c.Embedding.Dim)
	}
	if v := os.Getenv("HCE_EMBEDDING_RPM"); v != "" {
		fmt.Sscanf(v, "%d", &c.Embedding.RPM)
	}
	if v := os.Getenv("HCE_EMBEDDING_TPM"); v != "" {
		fmt.Sscanf(v, "%d", &c.Embedding.TPM)
	}
	if v := os.Getenv("OLLAMA_BASE_URL"); v != "" && c.Embedding.Provider == "ollama" {
		c.Embedding.BaseURL = v
	}

	if v := os.Getenv("MILVUS_ADDRESS"); v != "" {
		c.VectorDB.Address = v
	}
	if v := os.Getenv("HCE_VECTORDB_ADDRESS"); v != "" {
		c.VectorDB.Address = v
	}
	if v := os.Getenv("MILVUS_TOKEN"); v != "" {
		c.VectorDB.Token = v
	}
	if v := os.Getenv("HCE_VECTORDB_TOKEN"); v != "" {
		c.VectorDB.Token = v
	}
}
