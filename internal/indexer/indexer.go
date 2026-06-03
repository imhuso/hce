package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/imhuso/hce/internal/embedding"
	"github.com/imhuso/hce/internal/keyword"
	"github.com/imhuso/hce/internal/splitter"
	"github.com/imhuso/hce/internal/vectordb"
	"github.com/imhuso/hce/pkg/model"
)

// Indexer push-mode 索引编排器：服务端只接收文件内容，做切分/去重/向量化/检索。
type Indexer struct {
	splitter  splitter.Splitter
	embedding embedding.Embedding
	vectorDB  vectordb.VectorDB

	minChunkBytes int

	// 同 collection 上的写操作串行化（防并发触发的重复 Insert）
	mu       sync.Mutex
	colLocks map[string]*sync.Mutex

	// keyword 倒排索引：补足 dense vector 在长 chunk 里淹没字面信号的盲区。
	// per-collection，lazy 构建（首次该 collection 的 search 时从 Milvus 重建）。
	kwMu      sync.Mutex
	kwIndexes map[string]*keyword.Index
	kwLoaded  map[string]bool

	// collection → codebase_id 映射，让 /indexes 能暴露可读 id。
	registry *codebaseRegistry
}

// IndexerConfig 索引器配置
type IndexerConfig struct {
	Splitter      splitter.Splitter
	Embedding     embedding.Embedding
	VectorDB      vectordb.VectorDB
	MinChunkBytes int    // 过滤掉小于此长度的 chunk（0 = 不过滤）
	RegistryPath  string // collection→codebase_id 映射的持久化路径（空 = 仅内存）
}

const collectionPrefix = "hce_"

// NewIndexer 创建索引编排器
func NewIndexer(cfg IndexerConfig) *Indexer {
	return &Indexer{
		splitter:      cfg.Splitter,
		embedding:     cfg.Embedding,
		vectorDB:      cfg.VectorDB,
		minChunkBytes: cfg.MinChunkBytes,
		colLocks:      make(map[string]*sync.Mutex),
		kwIndexes:     make(map[string]*keyword.Index),
		kwLoaded:      make(map[string]bool),
		registry:      newCodebaseRegistry(cfg.RegistryPath),
	}
}

// kwIndexFor 返回 collection 对应的 keyword 索引；不存在则创建空的
func (idx *Indexer) kwIndexFor(collection string) *keyword.Index {
	idx.kwMu.Lock()
	defer idx.kwMu.Unlock()
	ki, ok := idx.kwIndexes[collection]
	if !ok {
		ki = keyword.NewIndex()
		idx.kwIndexes[collection] = ki
	}
	return ki
}

// ensureKwLoaded 首次某 collection 的 search 时，从 Milvus 拉所有现存 chunk 把
// keyword 倒排重建出来。后续写入由 IndexFiles 增量维护。
func (idx *Indexer) ensureKwLoaded(ctx context.Context, collection string) error {
	idx.kwMu.Lock()
	if idx.kwLoaded[collection] {
		idx.kwMu.Unlock()
		return nil
	}
	idx.kwMu.Unlock()

	ki := idx.kwIndexFor(collection)
	t0 := time.Now()

	// 单次拉取（Milvus 单次 query limit 16384，对中型项目够用）。
	// TODO: 超 16K chunks 的项目要做 PK 分页迭代
	const pageSize = 16000
	rows, err := idx.vectorDB.Query(ctx, collection, "id != \"\"",
		[]string{"id", "content", "language"}, pageSize)
	if err != nil {
		return fmt.Errorf("加载 keyword 索引失败: %w", err)
	}
	// 顺便统计语言分布（这是唯一的全量扫描点，零额外开销），供 /indexes 展示。
	langCount := make(map[string]int64)
	for _, r := range rows {
		ki.Add(r.ID, r.Content)
		lang := r.Language
		if lang == "" {
			lang = "other"
		}
		langCount[lang]++
	}
	idx.registry.recordLanguages(collection, langCount)
	loaded := len(rows)

	idx.kwMu.Lock()
	idx.kwLoaded[collection] = true
	idx.kwMu.Unlock()
	log.Printf("[Indexer] 🔑 keyword 索引重建完成  collection=%s  chunks=%d  耗时=%s",
		collection, loaded, time.Since(t0).Round(time.Millisecond))
	return nil
}

// maxChunkContentBytes Milvus VARCHAR 字段上限。任何超长 chunk 在入库前必须截断，
// 否则会触发 "exceeds max length" 整批失败。
const maxChunkContentBytes = 60000

// truncateOversized 防御性截断：splitter bug / 大 minified 文件等场景下，
// 单 chunk 仍可能超长，必须保证最终入库的每条 < Milvus 上限。
func truncateOversized(c *model.CodeChunk) {
	if len(c.Content) > maxChunkContentBytes {
		c.Content = c.Content[:maxChunkContentBytes]
	}
}

// sanitizeUTF8 把非 UTF-8 字节替换为 �。Milvus grpc marshal 对非 UTF-8
// 字符串会整批拒收，导致一颗坏数据毁掉整个 batch。这里强制保证合法。
func sanitizeUTF8(c *model.CodeChunk) {
	c.Content = strings.ToValidUTF8(c.Content, "�")
	c.RelativePath = strings.ToValidUTF8(c.RelativePath, "�")
	c.Language = strings.ToValidUTF8(c.Language, "")
}

// isLowValueChunk 判断 chunk 是否值得索引（过滤掉对检索几乎无价值的内容以省 EMB 钱）
func (idx *Indexer) isLowValueChunk(c *model.CodeChunk) (bool, string) {
	body := strings.TrimSpace(c.Content)
	if body == "" {
		return true, "空白"
	}
	if idx.minChunkBytes > 0 && len(body) < idx.minChunkBytes {
		return true, "过短"
	}
	if isAllImports(body, c.Language) {
		return true, "纯 import"
	}
	if isAllComments(body) {
		return true, "纯注释"
	}
	return false, ""
}

// isAllImports 粗判一个 chunk 是不是只含 import / using / require 等
func isAllImports(body, lang string) bool {
	lines := strings.Split(body, "\n")
	hasContent := false
	for _, raw := range lines {
		l := strings.TrimSpace(raw)
		if l == "" || strings.HasPrefix(l, "//") || strings.HasPrefix(l, "#") {
			continue
		}
		hasContent = true
		switch {
		case strings.HasPrefix(l, "import ") || strings.HasPrefix(l, "import("):
		case strings.HasPrefix(l, "from ") && strings.Contains(l, " import "):
		case strings.HasPrefix(l, "using "):
		case strings.HasPrefix(l, "require ") || strings.HasPrefix(l, "require("):
		case strings.HasPrefix(l, "use ") && strings.HasSuffix(l, ";"):
		case strings.HasPrefix(l, "package ") && strings.HasSuffix(l, ";"):
		case strings.HasPrefix(l, "#include"):
		case l == "(" || l == ")" || l == "{" || l == "}":
		default:
			return false
		}
	}
	_ = lang
	return hasContent
}

// isAllComments 粗判一个 chunk 是不是只含注释
func isAllComments(body string) bool {
	lines := strings.Split(body, "\n")
	hasContent := false
	for _, raw := range lines {
		l := strings.TrimSpace(raw)
		if l == "" {
			continue
		}
		hasContent = true
		if strings.HasPrefix(l, "//") || strings.HasPrefix(l, "#") ||
			strings.HasPrefix(l, "*") || strings.HasPrefix(l, "/*") ||
			strings.HasPrefix(l, "*/") || strings.HasPrefix(l, "<!--") {
			continue
		}
		return false
	}
	return hasContent
}

func (idx *Indexer) lockFor(name string) *sync.Mutex {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	m, ok := idx.colLocks[name]
	if !ok {
		m = &sync.Mutex{}
		idx.colLocks[name] = m
	}
	return m
}

// CollectionName 把 codebase_id 映射成 milvus collection 名（避免特殊字符）。
func CollectionName(codebaseID string) string {
	h := sha256.Sum256([]byte(codebaseID))
	return collectionPrefix + hex.EncodeToString(h[:])[:24]
}

// chunkID 文档主键：同 codebase 内 (relative_path + chunk_hash) 唯一。
func chunkID(relPath, chunkHash string) string {
	raw := relPath + ":" + chunkHash
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])[:32]
}

func chunkContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// ensureCollection 集合不存在就创建。
func (idx *Indexer) ensureCollection(ctx context.Context, name string) error {
	exists, err := idx.vectorDB.HasCollection(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return idx.vectorDB.CreateCollection(ctx, name, idx.embedding.Dimension())
}

// IndexFiles 处理客户端推送的一批文件：按文件做 chunk 级增量。
//
// 算法（每个文件独立处理）：
//  1. 切分得到 newChunks，算每个 chunk 的 content sha256 → newHashes
//  2. 查 Milvus 该文件已有的 chunks（id, chunk_hash）→ existing
//  3. existing ∩ new（按 hash 匹配）= 复用，跳过 EMB
//  4. existing - new = 已不存在的旧 chunk，需要 Delete
//  5. new - existing = 真正的新 chunk，批量 EMB → Insert
func (idx *Indexer) IndexFiles(ctx context.Context, codebaseID string, files []model.PushFile) (*model.UpsertResult, error) {
	if codebaseID == "" {
		return nil, fmt.Errorf("codebase_id 不能为空")
	}
	if len(files) == 0 {
		return &model.UpsertResult{}, nil
	}

	collection := CollectionName(codebaseID)
	idx.registry.record(collection, codebaseID)
	t0 := time.Now()
	log.Printf("[Indexer] 📥 收到 upsert  codebase=%s  文件=%d  集合=%s", codebaseID, len(files), collection)

	lock := idx.lockFor(collection)
	lock.Lock()
	defer lock.Unlock()

	if err := idx.ensureCollection(ctx, collection); err != nil {
		return nil, fmt.Errorf("准备集合失败: %w", err)
	}

	stats := &model.UpsertResult{FilesProcessed: len(files)}
	// 跨多文件累积"待 embed 的 chunks"，最后批量请求一次 EMB（省 RTT）
	var newDocs []model.VectorDocument
	var deleteIDs []string

	// 1) 切分所有文件 + 过滤低价值（纯 CPU、可顺序）
	tSplit := time.Now()
	type fileChunks struct {
		Path      string
		NewByHash map[string]*model.CodeChunk
	}
	allFiles := make([]fileChunks, 0, len(files))
	totalChunksSplit, totalChunksFiltered := 0, 0
	for _, f := range files {
		if f.RelativePath == "" {
			continue
		}
		rawChunks, err := idx.splitter.Split(f.Content, f.RelativePath)
		if err != nil {
			log.Printf("[Indexer] ⚠️  切分 %s 失败: %v", f.RelativePath, err)
			continue
		}
		newByHash := make(map[string]*model.CodeChunk)
		for i := range rawChunks {
			truncateOversized(&rawChunks[i]) // 防 splitter 漏网之鱼
			sanitizeUTF8(&rawChunks[i])      // 防非 UTF-8 字节让 Milvus 拒收整批
			if drop, _ := idx.isLowValueChunk(&rawChunks[i]); drop {
				totalChunksFiltered++
				continue
			}
			c := rawChunks[i]
			h := chunkContentHash(c.Content)
			newByHash[h] = &c
			totalChunksSplit++
		}
		allFiles = append(allFiles, fileChunks{Path: f.RelativePath, NewByHash: newByHash})
	}
	log.Printf("[Indexer] ✂️  切分完成  保留=%d  过滤=%d（低价值）  耗时=%s",
		totalChunksSplit, totalChunksFiltered, time.Since(tSplit).Round(time.Millisecond))

	// 2) 一次性批量查询所有文件的既有 chunks（按 path IN 分批，防 filter 超长）
	tQ := time.Now()
	existingByPath := make(map[string]map[string]string) // path -> hash -> id
	// 每批 300 paths，按平均 ~5 chunks/file 估算，单批结果远低于 Milvus 16384 上限
	const queryBatchPaths = 300
	for start := 0; start < len(allFiles); start += queryBatchPaths {
		end := start + queryBatchPaths
		if end > len(allFiles) {
			end = len(allFiles)
		}
		paths := make([]string, end-start)
		for i, fc := range allFiles[start:end] {
			paths[i] = fc.Path
		}
		filter := buildPathInFilter(paths)
		// 单次 query 限 30s 防 Milvus 偶发挂起拖死整个 sync
		qctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		rows, err := idx.vectorDB.Query(qctx, collection, filter,
			[]string{"id", "chunk_hash", "relative_path"}, 16000)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("批量查询既有 chunks 失败 (%d-%d): %w", start, end, err)
		}
		for _, r := range rows {
			m, ok := existingByPath[r.RelativePath]
			if !ok {
				m = make(map[string]string)
				existingByPath[r.RelativePath] = m
			}
			m[r.ChunkHash] = r.ID
		}
	}
	totalExisting := 0
	for _, m := range existingByPath {
		totalExisting += len(m)
	}
	log.Printf("[Indexer] 🔎 批量查询完成  既有=%d 个 chunk  覆盖文件=%d  耗时=%s",
		totalExisting, len(existingByPath), time.Since(tQ).Round(time.Millisecond))

	// 3) 对每个文件做 diff（纯内存）
	for _, fc := range allFiles {
		existingByHash := existingByPath[fc.Path] // 可能为 nil（首次出现）

		fileReused, fileNew, fileDelete := 0, 0, 0
		for h, id := range existingByHash {
			if _, ok := fc.NewByHash[h]; !ok {
				deleteIDs = append(deleteIDs, id)
				stats.ChunksDeleted++
				fileDelete++
			}
		}
		for h, c := range fc.NewByHash {
			stats.ChunksProcessed++
			if _, ok := existingByHash[h]; ok {
				stats.ChunksReused++
				fileReused++
				continue
			}
			newDocs = append(newDocs, model.VectorDocument{
				ID:           chunkID(c.RelativePath, h),
				ChunkHash:    h,
				Content:      c.Content,
				RelativePath: c.RelativePath,
				StartLine:    c.StartLine,
				EndLine:      c.EndLine,
				Language:     c.Language,
			})
			fileNew++
		}
		// 文件多时只在异常时打印；常规聚合到总览即可
		if fileNew > 0 || fileDelete > 0 {
			log.Printf("[Indexer] 📄 %s  切分=%d  既有=%d  新增=%d  复用=%d  删除=%d",
				fc.Path, len(fc.NewByHash), len(existingByHash), fileNew, fileReused, fileDelete)
		}
	}

	// 批量 EMB 新 chunks
	if len(newDocs) > 0 {
		log.Printf("[Indexer] 🧠 调用 Embedding API  待向量化 %d 条 (%s)", len(newDocs), idx.embedding.Name())
		tEmb := time.Now()
		texts := make([]string, len(newDocs))
		for i, d := range newDocs {
			texts[i] = d.Content
		}
		// 索引侧用 RETRIEVAL_DOCUMENT；Gemini/Voyage 利用此 task_type 进入"代码片段"
		// 向量空间，与查询侧的 CODE_RETRIEVAL_QUERY 配对，大幅提升检索精度
		vectors, err := idx.embedding.EmbedTyped(ctx, texts, embedding.TaskDocument)
		if err != nil {
			return nil, fmt.Errorf("批量 embedding 失败: %w", err)
		}
		if len(vectors) < len(newDocs) {
			return nil, fmt.Errorf("embedding 返回数量与请求不一致: got %d, want %d", len(vectors), len(newDocs))
		}
		for i := range newDocs {
			newDocs[i].Vector = vectors[i]
		}
		stats.ChunksNewEmbedded = len(newDocs)
		log.Printf("[Indexer] 🧠 Embedding 完成  %d 条  耗时=%s", len(vectors), time.Since(tEmb).Round(time.Millisecond))

		tIns := time.Now()
		ictx, icancel := context.WithTimeout(ctx, 60*time.Second)
		insertErr := idx.vectorDB.Insert(ictx, collection, newDocs)
		icancel()
		if insertErr != nil {
			return nil, fmt.Errorf("写入向量库失败: %w", insertErr)
		}
		log.Printf("[Indexer] 💾 向量库写入完成  %d 条  耗时=%s", len(newDocs), time.Since(tIns).Round(time.Millisecond))

		// 同步更新 keyword 倒排（仅在已 lazy-loaded 时增量维护，否则首次 search 会重建）
		idx.kwMu.Lock()
		alreadyLoaded := idx.kwLoaded[collection]
		idx.kwMu.Unlock()
		if alreadyLoaded {
			ki := idx.kwIndexFor(collection)
			for _, d := range newDocs {
				ki.Add(d.ID, d.Content)
			}
		}
	} else {
		log.Printf("[Indexer] 🧠 无新 chunk，跳过 Embedding")
	}

	// 删除已不存在的旧 chunks
	if len(deleteIDs) > 0 {
		tDel := time.Now()
		if err := idx.vectorDB.Delete(ctx, collection, deleteIDs); err != nil {
			log.Printf("[Indexer] ⚠️  删除旧 chunks 失败: %v", err)
		} else {
			log.Printf("[Indexer] 🗑️  删除旧 chunks 完成  %d 条  耗时=%s", len(deleteIDs), time.Since(tDel).Round(time.Millisecond))
			// keyword 索引同步移除（lazy 状态下也安全：未加载时是 no-op）
			ki := idx.kwIndexFor(collection)
			for _, id := range deleteIDs {
				ki.Remove(id)
			}
		}
	}

	log.Printf("[Indexer] ✅ upsert 完成  codebase=%s  文件=%d  新 EMB=%d  复用=%d  删除=%d  总耗时=%s",
		codebaseID, stats.FilesProcessed, stats.ChunksNewEmbedded, stats.ChunksReused, stats.ChunksDeleted, time.Since(t0).Round(time.Millisecond))

	return stats, nil
}

// DeleteFiles 删除指定文件的所有 chunks
func (idx *Indexer) DeleteFiles(ctx context.Context, codebaseID string, relativePaths []string) (int, error) {
	if codebaseID == "" {
		return 0, fmt.Errorf("codebase_id 不能为空")
	}
	if len(relativePaths) == 0 {
		return 0, nil
	}
	collection := CollectionName(codebaseID)
	exists, err := idx.vectorDB.HasCollection(ctx, collection)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}

	lock := idx.lockFor(collection)
	lock.Lock()
	defer lock.Unlock()

	deleted := 0
	for _, p := range relativePaths {
		filter := fmt.Sprintf("relative_path == \"%s\"", escapeMilvus(p))
		rows, err := idx.vectorDB.Query(ctx, collection, filter, []string{"id"}, 10000)
		if err != nil {
			log.Printf("[Indexer] ⚠️ 查 %s 失败: %v", p, err)
			continue
		}
		if len(rows) == 0 {
			continue
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}
		if err := idx.vectorDB.Delete(ctx, collection, ids); err != nil {
			log.Printf("[Indexer] ⚠️ 删 %s 失败: %v", p, err)
			continue
		}
		deleted += len(ids)
	}
	return deleted, nil
}

// Search 语义搜索（hybrid：dense vector + keyword 倒排）。
//
// 算法：
//  1. dense vector 拿 top_k * 3 个候选（多召回让 keyword 命中有空间挤进来）
//  2. keyword 倒排找 query 中关键 token 命中的所有 chunk_id
//  3. merge：
//     - 同时被 vector 和 keyword 命中的 chunk → score boost（每命中 1 个 token +0.05）
//     - 仅 keyword 命中的 chunk → 单独 query Milvus 拿元数据，base score = 0.5 + 命中数 × 0.03
//  4. 按 final score 排序，取 top_k
func (idx *Indexer) Search(ctx context.Context, codebaseID, query string, topK int) ([]model.SearchResult, error) {
	if codebaseID == "" {
		return nil, fmt.Errorf("codebase_id 不能为空")
	}
	collection := CollectionName(codebaseID)
	exists, err := idx.vectorDB.HasCollection(ctx, collection)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("codebase_id %q 尚未索引", codebaseID)
	}
	// 集合确实存在 → 记下可读 id（即使后续向量检索失败也已落库，便于前端命名/管理）。
	idx.registry.record(collection, codebaseID)
	if topK <= 0 {
		topK = 5
	}

	// 1. dense vector 多召回（top_k * 3）
	queryVec, err := idx.embedding.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("生成查询向量失败: %w", err)
	}
	wideK := topK * 3
	if wideK < 30 {
		wideK = 30
	}
	hits, err := idx.vectorDB.Search(ctx, collection, queryVec, wideK)
	if err != nil {
		return nil, fmt.Errorf("向量搜索失败: %w", err)
	}

	// 2. keyword 命中（lazy 重建索引）
	if err := idx.ensureKwLoaded(ctx, collection); err != nil {
		log.Printf("[Indexer] ⚠️ keyword 索引加载失败，降级为纯 vector 搜索: %v", err)
	}
	kwHits := idx.kwIndexFor(collection).Lookup(query)

	// 3. merge
	type entry struct {
		doc   model.VectorDocument
		score float64
	}
	pool := make(map[string]*entry, len(hits)+len(kwHits))
	for _, h := range hits {
		pool[h.Document.ID] = &entry{doc: h.Document, score: h.Score}
	}
	// 给 vector 结果加 keyword TF boost（cap 0.20，避免淹没 vector 信号）
	for id, tf := range kwHits {
		if e, ok := pool[id]; ok {
			boost := 0.02 * float64(tf)
			if boost > 0.20 {
				boost = 0.20
			}
			e.score += boost
		}
	}
	// 把仅 keyword 命中、不在 vector top 中的 chunk 拉回元数据
	var missingIDs []string
	for id := range kwHits {
		if _, ok := pool[id]; !ok {
			missingIDs = append(missingIDs, id)
		}
	}
	if len(missingIDs) > 0 {
		// 按命中 token 数降序，多命中的优先（更可能是用户真正想找的）
		sort.Slice(missingIDs, func(i, j int) bool {
			return kwHits[missingIDs[i]] > kwHits[missingIDs[j]]
		})
		// 防爆但放宽到 200——Java 大型项目 @RepeatSubmit 这种注解可能有几十甚至上百调用方
		const maxKwOnly = 200
		if len(missingIDs) > maxKwOnly {
			missingIDs = missingIDs[:maxKwOnly]
		}
		filter := buildIDInFilter(missingIDs)
		rows, err := idx.vectorDB.Query(ctx, collection, filter,
			[]string{"id", "chunk_hash", "content", "relative_path", "start_line", "end_line", "language"},
			len(missingIDs))
		if err != nil {
			log.Printf("[Indexer] ⚠️ 拉取 keyword 命中元数据失败: %v", err)
		} else {
			for _, r := range rows {
				// keyword 独占 chunk（不在向量 top 内）：基础分压低、cap 0.50，避免盖过向量语义结果，仅作补充召回
				score := 0.30 + 0.015*float64(kwHits[r.ID])
				if score > 0.50 {
					score = 0.50
				}
				pool[r.ID] = &entry{doc: r, score: score}
			}
		}
	}

	// 4. 排序 + 截取 top_k
	all := make([]*entry, 0, len(pool))
	for _, e := range pool {
		all = append(all, e)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })
	if len(all) > topK {
		all = all[:topK]
	}

	out := make([]model.SearchResult, 0, len(all))
	for _, e := range all {
		out = append(out, model.SearchResult{
			Content:      e.doc.Content,
			RelativePath: e.doc.RelativePath,
			StartLine:    e.doc.StartLine,
			EndLine:      e.doc.EndLine,
			Language:     e.doc.Language,
			Score:        e.score,
		})
	}
	return out, nil
}

// buildIDInFilter 构造 milvus IN 过滤表达式：id in ["a","b",...]
func buildIDInFilter(ids []string) string {
	var sb strings.Builder
	sb.WriteString(`id in [`)
	for i, id := range ids {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte('"')
		sb.WriteString(escapeMilvus(id))
		sb.WriteByte('"')
	}
	sb.WriteByte(']')
	return sb.String()
}

// Flush 强制把 codebase 的 growing segment 落盘，使新写入的 chunk 可被搜索。
// 应在 sync 全部完成时调用一次，不要每批都调。
func (idx *Indexer) Flush(ctx context.Context, codebaseID string) error {
	if codebaseID == "" {
		return fmt.Errorf("codebase_id 不能为空")
	}
	collection := CollectionName(codebaseID)
	exists, err := idx.vectorDB.HasCollection(ctx, collection)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	t0 := time.Now()
	if err := idx.vectorDB.Flush(ctx, collection); err != nil {
		return err
	}
	log.Printf("[Indexer] 🔁 flush 完成  codebase=%s  耗时=%s", codebaseID, time.Since(t0).Round(time.Millisecond))
	return nil
}

// Clear 清除整个 codebase 的索引
func (idx *Indexer) Clear(ctx context.Context, codebaseID string) error {
	if codebaseID == "" {
		return fmt.Errorf("codebase_id 不能为空")
	}
	collection := CollectionName(codebaseID)
	exists, err := idx.vectorDB.HasCollection(ctx, collection)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	lock := idx.lockFor(collection)
	lock.Lock()
	defer lock.Unlock()
	if err := idx.vectorDB.DropCollection(ctx, collection); err != nil {
		return err
	}
	idx.registry.remove(collection)
	return nil
}

// ListIndexes 列出所有 HCE 集合（按 collection 名前缀过滤）
func (idx *Indexer) ListIndexes(ctx context.Context) ([]model.IndexInfo, error) {
	cols, err := idx.vectorDB.ListCollections(ctx, collectionPrefix)
	if err != nil {
		return nil, err
	}
	out := make([]model.IndexInfo, 0, len(cols))
	for _, c := range cols {
		out = append(out, model.IndexInfo{
			CodebaseID: idx.registry.lookup(c.Name), // 搜过/同步过的集合有可读 id，否则为空（匿名）
			Collection: c.Name,
			NumChunks:  c.NumChunks,
			Languages:  idx.registry.lookupLanguages(c.Name), // 搜过一次后才有
		})
	}
	return out, nil
}

// escapeMilvus 转义 milvus filter 字符串中的双引号
func escapeMilvus(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\\' {
			out = append(out, '\\')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// buildPathInFilter 构造 milvus IN 过滤表达式：relative_path in ["a","b",...]
func buildPathInFilter(paths []string) string {
	var sb strings.Builder
	sb.WriteString(`relative_path in [`)
	for i, p := range paths {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte('"')
		sb.WriteString(escapeMilvus(p))
		sb.WriteByte('"')
	}
	sb.WriteByte(']')
	return sb.String()
}
