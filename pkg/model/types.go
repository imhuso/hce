package model

// CodeChunk 代码分块
type CodeChunk struct {
	Content      string // 代码内容
	RelativePath string // 相对文件路径
	StartLine    int    // 起始行号（从 1 开始）
	EndLine      int    // 结束行号
	Language     string // 编程语言标识
}

// SearchResult 语义搜索结果
type SearchResult struct {
	Content      string  `json:"content"`
	RelativePath string  `json:"relative_path"`
	StartLine    int     `json:"start_line"`
	EndLine      int     `json:"end_line"`
	Language     string  `json:"language"`
	Score        float64 `json:"score"`
}

// VectorDocument 向量数据库文档
type VectorDocument struct {
	ID           string    `json:"id"`
	ChunkHash    string    `json:"chunk_hash"`
	Content      string    `json:"content"`
	RelativePath string    `json:"relative_path"`
	StartLine    int       `json:"start_line"`
	EndLine      int       `json:"end_line"`
	Language     string    `json:"language"`
	Vector       []float32 `json:"vector"`
}

// VectorSearchResult 向量搜索结果
type VectorSearchResult struct {
	Document VectorDocument `json:"document"`
	Score    float64        `json:"score"`
}

// PushFile 客户端推送的单个文件
type PushFile struct {
	RelativePath string `json:"relative_path"`
	Content      string `json:"content"`
}

// UpsertResult upsert 操作的统计结果
type UpsertResult struct {
	FilesProcessed    int `json:"files_processed"`
	ChunksProcessed   int `json:"chunks_processed"`     // 这次推送切出的 chunk 总数
	ChunksNewEmbedded int `json:"chunks_new_embedded"`  // 触发 EMB 的新 chunk 数
	ChunksReused      int `json:"chunks_reused"`        // 命中既有 hash 跳过 EMB 的 chunk 数
	ChunksDeleted     int `json:"chunks_deleted"`       // 文件级清理 / 替换时删除的旧 chunk 数
}

// IndexInfo 已索引项目摘要（list 用）
type IndexInfo struct {
	CodebaseID string           `json:"codebase_id"`
	Collection string           `json:"collection"`
	NumChunks  int64            `json:"num_chunks"`
	Languages  map[string]int64 `json:"languages,omitempty"` // 语言 → chunk 数（搜过一次后才有）
}
