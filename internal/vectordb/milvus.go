package vectordb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/imhuso/hce/pkg/model"
)

// MilvusDB Milvus 向量数据库实现
type MilvusDB struct {
	client  client.Client
	address string
	token   string
}

// MilvusConfig Milvus 配置
type MilvusConfig struct {
	Address string
	Token   string
}

// NewMilvusDB 创建 Milvus 实例
func NewMilvusDB(ctx context.Context, cfg MilvusConfig) (*MilvusDB, error) {
	if cfg.Address == "" {
		cfg.Address = "localhost:19530"
	}
	conf := client.Config{Address: cfg.Address}
	if cfg.Token != "" {
		conf.APIKey = cfg.Token
	}
	conf.DialOptions = []grpc.DialOption{
		// grpc keepalive：每 10s ping，5s 内无响应判死。解决长任务后半开连接导致 hang。
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
		// 调大单消息上限到 256MB：keyword 索引懒加载会一次性拉所有 chunks 的 content，
		// 默认 4MB 远不够（lookah 4500 chunks 含 content 就 ~18MB）。
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(256*1024*1024),
			grpc.MaxCallSendMsgSize(256*1024*1024),
		),
	}
	c, err := client.NewClient(ctx, conf)
	if err != nil {
		return nil, fmt.Errorf("连接 Milvus 失败: %w", err)
	}
	return &MilvusDB{client: c, address: cfg.Address, token: cfg.Token}, nil
}

func (m *MilvusDB) Close() error {
	return m.client.Close()
}

// CreateCollection 创建集合（含 chunk_hash 字段与索引）
func (m *MilvusDB) CreateCollection(ctx context.Context, name string, dim int) error {
	exists, err := m.HasCollection(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	schema := &entity.Schema{
		CollectionName: name,
		Description:    "HCE 代码语义索引",
		Fields: []*entity.Field{
			{
				Name: "id", DataType: entity.FieldTypeVarChar,
				PrimaryKey: true, AutoID: false,
				TypeParams: map[string]string{"max_length": "128"},
			},
			{
				Name: "chunk_hash", DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{"max_length": "64"},
			},
			{
				Name: "content", DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{"max_length": "65535"},
			},
			{
				Name: "relative_path", DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{"max_length": "1024"},
			},
			{Name: "start_line", DataType: entity.FieldTypeInt32},
			{Name: "end_line", DataType: entity.FieldTypeInt32},
			{
				Name: "language", DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{"max_length": "32"},
			},
			{
				Name: "vector", DataType: entity.FieldTypeFloatVector,
				TypeParams: map[string]string{"dim": fmt.Sprintf("%d", dim)},
			},
		},
	}

	if err := m.client.CreateCollection(ctx, schema, entity.DefaultShardNumber); err != nil {
		return fmt.Errorf("创建集合 %s 失败: %w", name, err)
	}
	idx, err := entity.NewIndexIvfFlat(entity.COSINE, 128)
	if err != nil {
		return fmt.Errorf("创建索引参数失败: %w", err)
	}
	if err := m.client.CreateIndex(ctx, name, "vector", idx, false); err != nil {
		return fmt.Errorf("创建向量索引失败: %w", err)
	}
	if err := m.client.LoadCollection(ctx, name, false); err != nil {
		return fmt.Errorf("加载集合 %s 失败: %w", name, err)
	}
	return nil
}

func (m *MilvusDB) HasCollection(ctx context.Context, name string) (bool, error) {
	has, err := m.client.HasCollection(ctx, name)
	if err != nil {
		return false, fmt.Errorf("检查集合 %s 失败: %w", name, err)
	}
	return has, nil
}

func (m *MilvusDB) DropCollection(ctx context.Context, name string) error {
	if err := m.client.DropCollection(ctx, name); err != nil {
		return fmt.Errorf("删除集合 %s 失败: %w", name, err)
	}
	return nil
}

// ListCollections 列出所有以 prefix 开头的集合（带 chunk 数量）
func (m *MilvusDB) ListCollections(ctx context.Context, prefix string) ([]CollectionInfo, error) {
	cols, err := m.client.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("列出集合失败: %w", err)
	}
	var result []CollectionInfo
	for _, c := range cols {
		if prefix != "" && !strings.HasPrefix(c.Name, prefix) {
			continue
		}
		stats, err := m.client.GetCollectionStatistics(ctx, c.Name)
		var n int64
		if err == nil {
			if v, ok := stats["row_count"]; ok {
				fmt.Sscanf(v, "%d", &n)
			}
		}
		result = append(result, CollectionInfo{Name: c.Name, NumChunks: n})
	}
	return result, nil
}

// Insert 批量插入文档
func (m *MilvusDB) Insert(ctx context.Context, collection string, docs []model.VectorDocument) error {
	if len(docs) == 0 {
		return nil
	}
	ids := make([]string, len(docs))
	hashes := make([]string, len(docs))
	contents := make([]string, len(docs))
	paths := make([]string, len(docs))
	startLines := make([]int32, len(docs))
	endLines := make([]int32, len(docs))
	languages := make([]string, len(docs))
	vectors := make([][]float32, len(docs))

	for i, d := range docs {
		ids[i] = d.ID
		hashes[i] = d.ChunkHash
		contents[i] = d.Content
		paths[i] = d.RelativePath
		startLines[i] = int32(d.StartLine)
		endLines[i] = int32(d.EndLine)
		languages[i] = d.Language
		vectors[i] = d.Vector
	}

	cols := []entity.Column{
		entity.NewColumnVarChar("id", ids),
		entity.NewColumnVarChar("chunk_hash", hashes),
		entity.NewColumnVarChar("content", contents),
		entity.NewColumnVarChar("relative_path", paths),
		entity.NewColumnInt32("start_line", startLines),
		entity.NewColumnInt32("end_line", endLines),
		entity.NewColumnVarChar("language", languages),
		entity.NewColumnFloatVector("vector", len(docs[0].Vector), vectors),
	}
	if _, err := m.client.Insert(ctx, collection, "", cols...); err != nil {
		return fmt.Errorf("插入失败: %w", err)
	}
	// 不再每批 Flush！频繁 Flush 会创建大量 small sealed segments，触发 Milvus
	// compaction storm，最终阻塞 Insert RPC（曾让大库索引整个卡死）。
	// Milvus 会按 growing segment 容量（默认 512MB）自动 seal，更稳。
	return nil
}

// Search 向量相似性搜索
func (m *MilvusDB) Search(ctx context.Context, collection string, vector []float32, topK int) ([]model.VectorSearchResult, error) {
	vectors := []entity.Vector{entity.FloatVector(vector)}
	sp, err := entity.NewIndexIvfFlatSearchParam(16)
	if err != nil {
		return nil, fmt.Errorf("创建搜索参数失败: %w", err)
	}
	results, err := m.client.Search(
		ctx, collection, nil, "",
		[]string{"id", "chunk_hash", "content", "relative_path", "start_line", "end_line", "language"},
		vectors, "vector", entity.COSINE, topK, sp,
	)
	if err != nil {
		return nil, fmt.Errorf("搜索失败: %w", err)
	}
	var out []model.VectorSearchResult
	for _, r := range results {
		for i := 0; i < r.ResultCount; i++ {
			d := model.VectorDocument{}
			if c := getColumn(r.Fields, "id"); c != nil {
				if v, err := c.GetAsString(i); err == nil {
					d.ID = v
				}
			}
			if c := getColumn(r.Fields, "chunk_hash"); c != nil {
				if v, err := c.GetAsString(i); err == nil {
					d.ChunkHash = v
				}
			}
			if c := getColumn(r.Fields, "content"); c != nil {
				if v, err := c.GetAsString(i); err == nil {
					d.Content = v
				}
			}
			if c := getColumn(r.Fields, "relative_path"); c != nil {
				if v, err := c.GetAsString(i); err == nil {
					d.RelativePath = v
				}
			}
			if c := getColumn(r.Fields, "start_line"); c != nil {
				if v, err := c.GetAsInt64(i); err == nil {
					d.StartLine = int(v)
				}
			}
			if c := getColumn(r.Fields, "end_line"); c != nil {
				if v, err := c.GetAsInt64(i); err == nil {
					d.EndLine = int(v)
				}
			}
			if c := getColumn(r.Fields, "language"); c != nil {
				if v, err := c.GetAsString(i); err == nil {
					d.Language = v
				}
			}
			out = append(out, model.VectorSearchResult{Document: d, Score: float64(r.Scores[i])})
		}
	}
	return out, nil
}

// Flush 强制 seal growing segment，让新写入立即可见。同步等待完成。
func (m *MilvusDB) Flush(ctx context.Context, collection string) error {
	if err := m.client.Flush(ctx, collection, false); err != nil {
		return fmt.Errorf("flush 失败: %w", err)
	}
	return nil
}

func (m *MilvusDB) Delete(ctx context.Context, collection string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	idCol := entity.NewColumnVarChar("id", ids)
	if err := m.client.DeleteByPks(ctx, collection, "", idCol); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	return nil
}

// Query 按过滤表达式查询，把列还原成 VectorDocument。outputFields 决定取哪些列；
// 取 vector 列时需把 "vector" 显式写入。
func (m *MilvusDB) Query(ctx context.Context, collection, filter string, outputFields []string, limit int) ([]model.VectorDocument, error) {
	opts := []client.SearchQueryOptionFunc{}
	if limit > 0 {
		opts = append(opts, client.WithLimit(int64(limit)))
	}
	rs, err := m.client.Query(ctx, collection, nil, filter, outputFields, opts...)
	if err != nil {
		return nil, fmt.Errorf("查询失败: %w", err)
	}
	if len(rs) == 0 {
		return nil, nil
	}
	rowCount := rs[0].Len()
	out := make([]model.VectorDocument, rowCount)
	for _, col := range rs {
		name := col.Name()
		for i := 0; i < rowCount; i++ {
			switch name {
			case "id":
				if v, err := col.GetAsString(i); err == nil {
					out[i].ID = v
				}
			case "chunk_hash":
				if v, err := col.GetAsString(i); err == nil {
					out[i].ChunkHash = v
				}
			case "content":
				if v, err := col.GetAsString(i); err == nil {
					out[i].Content = v
				}
			case "relative_path":
				if v, err := col.GetAsString(i); err == nil {
					out[i].RelativePath = v
				}
			case "start_line":
				if v, err := col.GetAsInt64(i); err == nil {
					out[i].StartLine = int(v)
				}
			case "end_line":
				if v, err := col.GetAsInt64(i); err == nil {
					out[i].EndLine = int(v)
				}
			case "language":
				if v, err := col.GetAsString(i); err == nil {
					out[i].Language = v
				}
			case "vector":
				// vector 字段需要类型断言
				if vc, ok := col.(*entity.ColumnFloatVector); ok {
					data := vc.Data()
					if i < len(data) {
						out[i].Vector = data[i]
					}
				}
			}
		}
	}
	return out, nil
}

func getColumn(fields client.ResultSet, name string) entity.Column {
	for _, c := range fields {
		if c.Name() == name {
			return c
		}
	}
	return nil
}
