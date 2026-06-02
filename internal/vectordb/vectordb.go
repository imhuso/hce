package vectordb

import (
	"context"

	"github.com/imhuso/hce/pkg/model"
)

// VectorDB 向量数据库接口
type VectorDB interface {
	CreateCollection(ctx context.Context, name string, dim int) error
	HasCollection(ctx context.Context, name string) (bool, error)
	DropCollection(ctx context.Context, name string) error
	ListCollections(ctx context.Context, prefix string) ([]CollectionInfo, error)

	Insert(ctx context.Context, collection string, docs []model.VectorDocument) error
	// Flush 强制把当前 growing segment 落盘 sealed，让数据立即可被搜索。
	// 应该在 sync 完成时显式调用一次，而不是每批 Insert 都调用。
	Flush(ctx context.Context, collection string) error
	Search(ctx context.Context, collection string, vector []float32, topK int) ([]model.VectorSearchResult, error)
	Delete(ctx context.Context, collection string, ids []string) error

	// Query 按过滤表达式查询；caller 通过 outputFields 指定需要的列；vector 列也可取。
	Query(ctx context.Context, collection, filter string, outputFields []string, limit int) ([]model.VectorDocument, error)

	Close() error
}

// CollectionInfo 集合信息
type CollectionInfo struct {
	Name      string
	NumChunks int64
}
