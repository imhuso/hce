package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/imhuso/hce/internal/api"
	"github.com/imhuso/hce/internal/config"
	"github.com/imhuso/hce/internal/embedding"
	"github.com/imhuso/hce/internal/indexer"
	"github.com/imhuso/hce/internal/splitter"
	"github.com/imhuso/hce/internal/vectordb"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	log.Printf("[Main] 📝 配置加载完成: embedding=%s/%s, vectordb=%s@%s",
		cfg.Embedding.Provider, cfg.Embedding.Model, cfg.VectorDB.Provider, cfg.VectorDB.Address)

	ctx := context.Background()

	emb, err := initEmbedding(cfg)
	if err != nil {
		log.Fatalf("初始化 Embedding 失败: %v", err)
	}
	log.Printf("[Main] 🧠 Embedding: %s (维度: %d)", emb.Name(), emb.Dimension())

	vdb, err := initVectorDB(ctx, cfg)
	if err != nil {
		log.Fatalf("初始化 VectorDB 失败: %v", err)
	}
	defer vdb.Close()
	log.Printf("[Main] 🗄️ VectorDB: %s@%s", cfg.VectorDB.Provider, cfg.VectorDB.Address)

	sp := splitter.NewTreeSitterSplitter(cfg.Splitter.MaxChunkSize, cfg.Splitter.ChunkOverlap)
	log.Printf("[Main] ✂️ Splitter: tree-sitter (支持 %d 种语言)", len(sp.SupportedLanguages()))

	dataDir := os.Getenv("HCE_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	idx := indexer.NewIndexer(indexer.IndexerConfig{
		Splitter:      sp,
		Embedding:     emb,
		VectorDB:      vdb,
		MinChunkBytes: cfg.Splitter.MinChunkBytes,
		RegistryPath:  filepath.Join(dataDir, "codebases.json"),
	})

	router := api.NewRouter(idx)
	serverErr := make(chan error, 1)
	go func() {
		if err := router.Start(cfg.Server.Host, cfg.Server.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case <-stop:
		log.Printf("[Main] 🛑 收到关闭信号，优雅关闭中...")
	case err := <-serverErr:
		log.Printf("[Main] ❌ HTTP 服务异常: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := router.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Main] ⚠️ HTTP 关闭异常: %v", err)
	}
	log.Printf("[Main] ✅ 已退出")
}

func initEmbedding(cfg *config.Config) (embedding.Embedding, error) {
	switch cfg.Embedding.Provider {
	case "openai":
		return embedding.NewOpenAIEmbedding(embedding.OpenAIConfig{
			APIKey: cfg.Embedding.APIKey, Model: cfg.Embedding.Model,
			BaseURL: cfg.Embedding.BaseURL, Dim: cfg.Embedding.Dim,
		}), nil
	case "ollama":
		return embedding.NewOllamaEmbedding(embedding.OllamaConfig{
			Model: cfg.Embedding.Model, BaseURL: cfg.Embedding.BaseURL, Dim: cfg.Embedding.Dim,
		}), nil
	case "voyageai":
		return embedding.NewVoyageAIEmbedding(embedding.VoyageAIConfig{
			APIKey: cfg.Embedding.APIKey, Model: cfg.Embedding.Model,
		}), nil
	case "gemini":
		return embedding.NewGeminiEmbedding(embedding.GeminiConfig{
			APIKey: cfg.Embedding.APIKey, Model: cfg.Embedding.Model,
			RPM: cfg.Embedding.RPM, TPM: cfg.Embedding.TPM,
		}), nil
	default:
		log.Printf("⚠️ 未知 Embedding 供应商 %q，使用 OpenAI", cfg.Embedding.Provider)
		return embedding.NewOpenAIEmbedding(embedding.OpenAIConfig{
			APIKey: cfg.Embedding.APIKey, Model: cfg.Embedding.Model, BaseURL: cfg.Embedding.BaseURL,
		}), nil
	}
}

func initVectorDB(ctx context.Context, cfg *config.Config) (vectordb.VectorDB, error) {
	switch cfg.VectorDB.Provider {
	case "milvus":
		return vectordb.NewMilvusDB(ctx, vectordb.MilvusConfig{
			Address: cfg.VectorDB.Address, Token: cfg.VectorDB.Token,
		})
	default:
		log.Printf("⚠️ 未知 VectorDB 供应商 %q，使用 Milvus", cfg.VectorDB.Provider)
		return vectordb.NewMilvusDB(ctx, vectordb.MilvusConfig{
			Address: cfg.VectorDB.Address, Token: cfg.VectorDB.Token,
		})
	}
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.SetOutput(os.Stdout)
}
