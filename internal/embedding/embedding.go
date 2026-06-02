package embedding

import "context"

// TaskType 标识 embedding 调用的语义场景，让支持非对称编码的供应商
// （如 Gemini 的 RETRIEVAL_DOCUMENT/CODE_RETRIEVAL_QUERY、VoyageAI 的 input_type）
// 能用更优的向量空间。
type TaskType int

const (
	TaskUnspecified  TaskType = iota
	TaskDocument              // 索引代码片段时使用
	TaskCodeQuery             // 用户用自然语言查代码时使用
)

// Embedding 向量化接口，抽象不同 Embedding 供应商
type Embedding interface {
	// Embed 把单段查询文本转成向量（语义视为 query）
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch 批量向量化（语义视为 document，索引时用）
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// EmbedTyped 显式指定 task type 的批量调用。supplier 不支持 task type
	// 时退化为普通 EmbedBatch；支持的供应商（Gemini/Voyage）会按 task 优化。
	EmbedTyped(ctx context.Context, texts []string, task TaskType) ([][]float32, error)

	Dimension() int
	Name() string
}
