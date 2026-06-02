// Package keyword 是 hce 的轻量字面/关键词倒排索引。
//
// 用途：补足 dense vector 的字面命中盲区——例如代码 chunk 里只是顺带含
// `@RepeatSubmit` 注解，但 chunk 主语义被淹没，纯向量召不回。
// 倒排索引按 token → chunk_id 维护内存映射，search 时与 vector 结果合并。
//
// 索引数据**不持久化**：服务端启动后第一次 search 该 codebase 时按需 lazy 重建
// （从 Milvus 拉所有 chunks）。后续 IndexFiles 写入时同步增量更新。
package keyword

import (
	"regexp"
	"strings"
	"sync"
)

// Index 单个 codebase 的内存倒排
type Index struct {
	mu sync.RWMutex
	// token (lower) → chunk_id → token 在该 chunk 中出现次数（TF）
	tokens map[string]map[string]int
	// chunk_id → 所有 token（删除时反查）
	byChunk map[string][]string
}

// NewIndex 创建空索引
func NewIndex() *Index {
	return &Index{
		tokens:  make(map[string]map[string]int),
		byChunk: make(map[string][]string),
	}
}

// Add 索引一个 chunk
func (i *Index) Add(chunkID, content string) {
	toks := tokenize(content) // map[token]freq
	i.mu.Lock()
	defer i.mu.Unlock()
	i.removeLocked(chunkID)
	tokSlice := make([]string, 0, len(toks))
	for t, freq := range toks {
		m, ok := i.tokens[t]
		if !ok {
			m = make(map[string]int)
			i.tokens[t] = m
		}
		m[chunkID] = freq
		tokSlice = append(tokSlice, t)
	}
	i.byChunk[chunkID] = tokSlice
}

// Remove 删除一个 chunk
func (i *Index) Remove(chunkID string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.removeLocked(chunkID)
}

func (i *Index) removeLocked(chunkID string) {
	toks, ok := i.byChunk[chunkID]
	if !ok {
		return
	}
	for _, t := range toks {
		if m, ok := i.tokens[t]; ok {
			delete(m, chunkID)
			if len(m) == 0 {
				delete(i.tokens, t)
			}
		}
	}
	delete(i.byChunk, chunkID)
}

// Lookup 用查询字符串找命中 chunk_id 及 TF 加权得分（mini-BM25）。
// 返回值为 chunk_id → 累计 TF（token 在 chunk 出现次数加总）；越大越相关。
// 例如 chunk 里 @RepeatSubmit 出现 4 次，query 是 "@RepeatSubmit"（拆 3 个子 token），
// 得分 = 4 * 3 = 12，远高于单次出现的 3。
func (i *Index) Lookup(query string) map[string]int {
	qToks := tokenize(query)
	if len(qToks) == 0 {
		return nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	scores := make(map[string]int)
	for qt := range qToks {
		if m, ok := i.tokens[qt]; ok {
			for cid, tf := range m {
				scores[cid] += tf
			}
		}
	}
	return scores
}

// Size 返回索引中 chunk 数量（诊断用）
func (i *Index) Size() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.byChunk)
}

// ============ tokenizer ============

// 提取规则：identifier（英文字母/数字/下划线），保留 @ 前缀（如 @RepeatSubmit）
// 中文不分词（让 dense vector 处理）；纯数字跳过；过短跳过
var tokenRE = regexp.MustCompile(`@?[A-Za-z][A-Za-z0-9_]{2,}`)

// stopWords 常见英文 / 编程关键词，避免它们成为 keyword 信号
var stopWords = map[string]bool{
	// 通用
	"the": true, "and": true, "for": true, "with": true, "from": true,
	"this": true, "that": true, "have": true, "has": true, "are": true,
	"was": true, "were": true, "but": true, "not": true, "you": true,
	"can": true, "will": true, "would": true, "could": true, "should": true,
	// 编程关键字
	"public": true, "private": true, "protected": true, "static": true,
	"final": true, "void": true, "return": true, "true": true, "false": true,
	"null": true, "import": true, "package": true, "class": true,
	"interface": true, "extends": true, "implements": true, "func": true,
	"const": true, "var": true, "let": true, "function": true,
	"if": true, "else": true, "for_": true, "while": true, "switch": true,
	"case": true, "default": true, "break": true, "continue": true,
	"new": true, "throw": true, "throws": true, "try": true, "catch": true,
	"finally": true, "string": true, "int": true, "boolean": true,
	"long": true, "short": true, "double": true, "float": true,
	"map": true, "list": true, "set": true, "array": true,
	// 常见高频函数 / 字段名（不带 @ 的；带 @ 的注解保留）
	"get": true, "set_": true, "put": true, "is_": true,
}

// tokenize 提取 token：identifier、注解。同时把 camelCase 拆成子词，并统计每个 token 的频率
// 例如 createOrder → createorder + create + order；@RepeatSubmit → @repeatsubmit + repeat + submit
func tokenize(s string) map[string]int {
	out := make(map[string]int)
	for _, m := range tokenRE.FindAllString(s, -1) {
		raw := strings.ToLower(m)
		if !addToken(out, raw) {
			continue
		}
		body := strings.TrimPrefix(raw, "@")
		for _, sub := range splitCamel(m) {
			if sub == "" {
				continue
			}
			lower := strings.ToLower(sub)
			if lower != body && lower != raw {
				addToken(out, lower)
			}
		}
	}
	return out
}

func addToken(out map[string]int, t string) bool {
	if len(t) < 3 {
		return false
	}
	if stopWords[strings.TrimPrefix(t, "@")] {
		return false
	}
	out[t]++
	return true
}

// splitCamel 把 camelCase / PascalCase 拆开。RepeatSubmit → ["Repeat", "Submit"]
func splitCamel(s string) []string {
	s = strings.TrimPrefix(s, "@")
	var parts []string
	start := 0
	for i := 1; i < len(s); i++ {
		if isUpper(s[i]) && (!isUpper(s[i-1]) || (i+1 < len(s) && !isUpper(s[i+1]))) {
			parts = append(parts, s[start:i])
			start = i
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
