package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/imhuso/hce/internal/indexer"
)

// Router HTTP 路由器
type Router struct {
	handler *Handler
	mux     *http.ServeMux
	server  *http.Server
}

func NewRouter(idx *indexer.Indexer) *Router {
	handler := NewHandler(idx)
	mux := http.NewServeMux()
	r := &Router{handler: handler, mux: mux}
	r.registerRoutes()
	return r
}

func (r *Router) registerRoutes() {
	r.mux.HandleFunc("/api/v1/index/upsert", r.cors(r.handler.HandleUpsert))
	r.mux.HandleFunc("/api/v1/index/delete", r.cors(r.handler.HandleDeleteFiles))
	r.mux.HandleFunc("/api/v1/index/flush", r.cors(r.handler.HandleFlush))
	r.mux.HandleFunc("/api/v1/index", r.cors(r.handler.HandleClear))
	r.mux.HandleFunc("/api/v1/indexes", r.cors(r.handler.HandleListIndexes))
	r.mux.HandleFunc("/api/v1/search", r.cors(r.handler.HandleSearch))
	r.mux.HandleFunc("/api/v1/health", r.cors(r.handler.HandleHealth))
}

func (r *Router) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if req.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, req)
	}
}

// Start 启动 HTTP 服务（阻塞）
func (r *Router) Start(host string, port int) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("[Server] 🚀 HTTP 服务启动: http://%s", addr)
	log.Printf("[Server] 📡 API 端点（push 模式 / codebase_id）:")
	log.Printf("[Server]   POST   /api/v1/index/upsert  - 推送文件内容并增量索引")
	log.Printf("[Server]   POST   /api/v1/index/delete  - 删除指定文件的索引")
	log.Printf("[Server]   DELETE /api/v1/index         - 清除整个 codebase")
	log.Printf("[Server]   GET    /api/v1/indexes       - 列出所有已索引集合")
	log.Printf("[Server]   POST   /api/v1/search        - 语义搜索")
	log.Printf("[Server]   GET    /api/v1/health        - 健康检查")

	r.server = &http.Server{
		Addr:              addr,
		Handler:           r.mux,
		ReadHeaderTimeout: 10 * time.Second,
		// 不设 ReadTimeout/WriteTimeout：upsert 大批文件可能用时较长。
		MaxHeaderBytes: 1 << 20,
	}
	return r.server.ListenAndServe()
}

// Shutdown 优雅关闭
func (r *Router) Shutdown(ctx context.Context) error {
	if r.server == nil {
		return nil
	}
	return r.server.Shutdown(ctx)
}
