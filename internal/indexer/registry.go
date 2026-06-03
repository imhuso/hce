package indexer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// codebaseMeta 单个 collection 的服务端附加信息。
type codebaseMeta struct {
	CodebaseID string           `json:"codebase_id"`
	Languages  map[string]int64 `json:"languages,omitempty"` // 语言 → chunk 数
}

// codebaseRegistry 持久化 collection → 元信息（可读 id、语言分布）的映射。
//
// collection 名是 codebase_id 的单向哈希（见 CollectionName），服务端原本无从得知
// 原始 id。但每次 search / upsert 请求都带着 codebase_id，于是我们在那时机会性地把
// 映射记下来——既无需重建已有索引（下次 CLI sync / search 时自动补全），又能让
// /indexes 暴露可读 id 与语言构成，前端据此实现"点击代码库直接搜索" + 语言分析。
type codebaseRegistry struct {
	mu   sync.RWMutex
	path string
	m    map[string]*codebaseMeta // collection -> meta
}

func newCodebaseRegistry(path string) *codebaseRegistry {
	r := &codebaseRegistry{path: path, m: make(map[string]*codebaseMeta)}
	r.load()
	return r
}

func (r *codebaseRegistry) load() {
	if r.path == "" {
		return
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		return
	}
	// 新格式：collection -> {codebase_id, languages}
	var m map[string]*codebaseMeta
	if json.Unmarshal(data, &m) == nil && m != nil {
		r.m = m
		return
	}
	// 兼容旧格式：collection -> codebase_id（字符串）
	var old map[string]string
	if json.Unmarshal(data, &old) == nil {
		for k, v := range old {
			r.m[k] = &codebaseMeta{CodebaseID: v}
		}
	}
}

// save 原子写：先写临时文件再 rename，避免并发读到半截 JSON。
func (r *codebaseRegistry) save() {
	if r.path == "" {
		return
	}
	r.mu.RLock()
	data, err := json.MarshalIndent(r.m, "", "  ")
	r.mu.RUnlock()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, r.path)
}

func (r *codebaseRegistry) entry(collection string) *codebaseMeta {
	e := r.m[collection]
	if e == nil {
		e = &codebaseMeta{}
		r.m[collection] = e
	}
	return e
}

// record 记下可读 id；已是相同值则跳过写盘。
func (r *codebaseRegistry) record(collection, codebaseID string) {
	if collection == "" || codebaseID == "" {
		return
	}
	r.mu.Lock()
	e := r.entry(collection)
	if e.CodebaseID == codebaseID {
		r.mu.Unlock()
		return
	}
	e.CodebaseID = codebaseID
	r.mu.Unlock()
	r.save()
}

// recordLanguages 记下语言分布（来自一次全量扫描）。
func (r *codebaseRegistry) recordLanguages(collection string, langs map[string]int64) {
	if collection == "" || len(langs) == 0 {
		return
	}
	r.mu.Lock()
	r.entry(collection).Languages = langs
	r.mu.Unlock()
	r.save()
}

func (r *codebaseRegistry) lookup(collection string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e := r.m[collection]; e != nil {
		return e.CodebaseID
	}
	return ""
}

func (r *codebaseRegistry) lookupLanguages(collection string) map[string]int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e := r.m[collection]
	if e == nil || len(e.Languages) == 0 {
		return nil
	}
	out := make(map[string]int64, len(e.Languages))
	for k, v := range e.Languages {
		out[k] = v
	}
	return out
}

func (r *codebaseRegistry) remove(collection string) {
	r.mu.Lock()
	_, ok := r.m[collection]
	delete(r.m, collection)
	r.mu.Unlock()
	if ok {
		r.save()
	}
}
