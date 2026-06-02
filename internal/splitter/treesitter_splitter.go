package splitter

import (
	gocontext "context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/lua"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/scala"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/imhuso/hce/pkg/model"
)

// TreeSitterSplitter 基于 tree-sitter AST 的多语言代码分块器
type TreeSitterSplitter struct {
	maxChunkSize int // 单个 chunk 最大字符数
	overlap      int // chunk 之间的重叠字符数
}

// NewTreeSitterSplitter 创建 tree-sitter 分块器
func NewTreeSitterSplitter(maxChunkSize, overlap int) *TreeSitterSplitter {
	if maxChunkSize <= 0 {
		maxChunkSize = 2500
	}
	if overlap < 0 {
		overlap = 300
	}
	return &TreeSitterSplitter{
		maxChunkSize: maxChunkSize,
		overlap:      overlap,
	}
}

// SupportedLanguages 返回支持的语言列表
func (s *TreeSitterSplitter) SupportedLanguages() []string {
	return []string{
		"go", "python", "javascript", "typescript", "java",
		"c", "cpp", "csharp", "rust", "ruby", "php",
		"swift", "kotlin", "scala", "lua", "bash",
	}
}

// wholeFileThreshold 文件 < 此字节数时，整体作为单 chunk（不切分）。
// 对中型项目能砍掉 60-80% 的 chunk 数（多数源文件都偏小），
// 大幅减少 EMB 调用，且对语义检索精度几乎无影响（小文件本来语义紧凑）。
const wholeFileThreshold = 4096

// Split 将文件内容按 AST 语义边界分块
func (s *TreeSitterSplitter) Split(content string, filePath string) ([]model.CodeChunk, error) {
	lang := DetectLanguage(filePath)

	// 小文件：整体作为一个 chunk，跳过 AST 切分
	if len(content) > 0 && len(content) <= wholeFileThreshold && strings.TrimSpace(content) != "" {
		lineCount := strings.Count(content, "\n") + 1
		return []model.CodeChunk{{
			Content:      content,
			RelativePath: filePath,
			StartLine:    1,
			EndLine:      lineCount,
			Language:     lang,
		}}, nil
	}

	tsLang := s.getLanguage(lang)
	// 如果无法获取 tree-sitter 语言定义，回退到简单分块
	if tsLang == nil {
		return s.fallbackSplit(content, filePath, lang)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(tsLang)

	ctx := gocontext.Background()
	tree, err := parser.ParseCtx(ctx, nil, []byte(content))
	if err != nil {
		// AST 解析失败，回退
		return s.fallbackSplit(content, filePath, lang)
	}
	defer tree.Close()

	rootNode := tree.RootNode()
	chunks := s.extractChunks(rootNode, content, filePath, lang)

	// 如果 AST 未提取到任何有意义的块，回退
	if len(chunks) == 0 {
		return s.fallbackSplit(content, filePath, lang)
	}

	return chunks, nil
}

// getLanguage 返回对应语言的 tree-sitter Language 对象
func (s *TreeSitterSplitter) getLanguage(lang string) *sitter.Language {
	switch lang {
	case "go":
		return golang.GetLanguage()
	case "python":
		return python.GetLanguage()
	case "javascript":
		return javascript.GetLanguage()
	case "typescript":
		return typescript.GetLanguage()
	case "java":
		return java.GetLanguage()
	case "c":
		return c.GetLanguage()
	case "cpp":
		return cpp.GetLanguage()
	case "csharp":
		return csharp.GetLanguage()
	case "rust":
		return rust.GetLanguage()
	case "ruby":
		return ruby.GetLanguage()
	case "php":
		return php.GetLanguage()
	case "swift":
		return swift.GetLanguage()
	case "kotlin":
		return kotlin.GetLanguage()
	case "scala":
		return scala.GetLanguage()
	case "lua":
		return lua.GetLanguage()
	case "bash":
		return bash.GetLanguage()
	default:
		return nil
	}
}

// topLevelNodeTypes 返回不同语言中应作为独立 chunk 的顶级节点类型
func (s *TreeSitterSplitter) topLevelNodeTypes(lang string) map[string]bool {
	// 通用的函数/类/方法定义节点类型
	common := map[string]bool{
		"function_definition":   true,
		"function_declaration":  true,
		"method_definition":     true,
		"method_declaration":    true,
		"class_definition":      true,
		"class_declaration":     true,
		"interface_declaration": true,
		"struct_definition":     true,
		"enum_definition":       true,
		"enum_declaration":      true,
		"trait_definition":      true,
		"impl_item":             true,
		"module_definition":     true,
	}

	// 各语言特有的节点类型
	switch lang {
	case "go":
		common["function_declaration"] = true
		common["method_declaration"] = true
		common["type_declaration"] = true
		common["type_spec"] = true
	case "python":
		common["decorated_definition"] = true
		common["async_function_definition"] = true
	case "javascript", "typescript":
		common["export_statement"] = true
		common["lexical_declaration"] = true
		common["arrow_function"] = true
		common["generator_function_declaration"] = true
	case "java":
		common["constructor_declaration"] = true
		common["annotation_type_declaration"] = true
	case "rust":
		common["function_item"] = true
		common["struct_item"] = true
		common["enum_item"] = true
		common["impl_item"] = true
		common["trait_item"] = true
		common["mod_item"] = true
	case "cpp", "c":
		common["preproc_function_def"] = true
		common["template_declaration"] = true
		common["namespace_definition"] = true
	}

	return common
}

// extractChunks 从 AST 提取代码块
func (s *TreeSitterSplitter) extractChunks(root *sitter.Node, content string, filePath string, lang string) []model.CodeChunk {
	var chunks []model.CodeChunk
	topTypes := s.topLevelNodeTypes(lang)
	relPath := filePath

	// 遍历顶级子节点
	for i := range int(root.ChildCount()) {
		child := root.Child(i)
		nodeType := child.Type()

		if topTypes[nodeType] {
			chunkContent := content[child.StartByte():child.EndByte()]
			startLine := int(child.StartPoint().Row) + 1
			endLine := int(child.EndPoint().Row) + 1

			// 如果该节点太大，递归处理其子节点
			if len(chunkContent) > s.maxChunkSize {
				subChunks := s.splitLargeNode(child, content, relPath, lang)
				chunks = append(chunks, subChunks...)
			} else {
				chunks = append(chunks, model.CodeChunk{
					Content:      chunkContent,
					RelativePath: relPath,
					StartLine:    startLine,
					EndLine:      endLine,
					Language:     lang,
				})
			}
		} else if nodeType == "comment" || nodeType == "line_comment" || nodeType == "block_comment" {
			// 跳过独立注释（它们会被关联到下一个代码块）
			continue
		} else {
			// 其他顶级节点（import 声明、变量声明、minified 单行表达式等）
			chunkContent := content[child.StartByte():child.EndByte()]
			if strings.TrimSpace(chunkContent) == "" {
				continue
			}
			startLine := int(child.StartPoint().Row) + 1
			endLine := int(child.EndPoint().Row) + 1
			// 关键保护：超大 chunk（如 minified JS 一行就 400KB）必须硬切，
			// 否则会撑爆向量库 content 字段（Milvus 65535 字节上限）。
			if len(chunkContent) > s.maxChunkSize {
				chunks = append(chunks, s.splitByLines(chunkContent, relPath, lang, startLine)...)
			} else {
				chunks = append(chunks, model.CodeChunk{
					Content:      chunkContent,
					RelativePath: relPath,
					StartLine:    startLine,
					EndLine:      endLine,
					Language:     lang,
				})
			}
		}
	}

	// 合并过小的相邻 chunk
	return s.mergeSmallChunks(chunks)
}

// splitLargeNode 拆分过大的 AST 节点
func (s *TreeSitterSplitter) splitLargeNode(node *sitter.Node, content string, filePath string, lang string) []model.CodeChunk {
	var chunks []model.CodeChunk

	childCount := int(node.ChildCount())
	if childCount == 0 {
		// 叶子节点但太大，按行分割
		return s.splitByLines(content[node.StartByte():node.EndByte()], filePath, lang, int(node.StartPoint().Row)+1)
	}

	var currentContent strings.Builder
	currentStartLine := int(node.StartPoint().Row) + 1
	flush := func(endLine int) {
		if currentContent.Len() == 0 {
			return
		}
		chunks = append(chunks, model.CodeChunk{
			Content:      currentContent.String(),
			RelativePath: filePath,
			StartLine:    currentStartLine,
			EndLine:      endLine,
			Language:     lang,
		})
		currentContent.Reset()
	}

	for i := range childCount {
		child := node.Child(i)
		childContent := content[child.StartByte():child.EndByte()]
		childStartLine := int(child.StartPoint().Row) + 1

		// 单个子节点本身超限：先 flush 当前 buffer，再按字节切分该节点
		if len(childContent) > s.maxChunkSize {
			flush(childStartLine - 1)
			chunks = append(chunks, s.splitByLines(childContent, filePath, lang, childStartLine)...)
			currentStartLine = int(child.EndPoint().Row) + 2
			continue
		}

		if currentContent.Len()+len(childContent)+1 > s.maxChunkSize && currentContent.Len() > 0 {
			flush(childStartLine - 1)
			currentStartLine = childStartLine
		}

		currentContent.WriteString(childContent)
		currentContent.WriteString("\n")
	}

	// 最后剩余的内容
	if currentContent.Len() > 0 {
		chunks = append(chunks, model.CodeChunk{
			Content:      currentContent.String(),
			RelativePath: filePath,
			StartLine:    currentStartLine,
			EndLine:      int(node.EndPoint().Row) + 1,
			Language:     lang,
		})
	}

	return chunks
}

// mergeSmallChunks 合并过小的相邻 chunk
func (s *TreeSitterSplitter) mergeSmallChunks(chunks []model.CodeChunk) []model.CodeChunk {
	if len(chunks) <= 1 {
		return chunks
	}

	minChunkSize := s.maxChunkSize / 4 // 小于 1/4 最大尺寸的认为太小
	var merged []model.CodeChunk
	var current *model.CodeChunk

	for i := range chunks {
		if current == nil {
			c := chunks[i]
			current = &c
			continue
		}

		// 如果当前 chunk 太小且合并后不超过上限，则合并
		if len(current.Content) < minChunkSize &&
			len(current.Content)+len(chunks[i].Content) <= s.maxChunkSize {
			current.Content += "\n" + chunks[i].Content
			current.EndLine = chunks[i].EndLine
		} else {
			merged = append(merged, *current)
			c := chunks[i]
			current = &c
		}
	}

	if current != nil {
		merged = append(merged, *current)
	}

	return merged
}

// splitByLines 按字节上限分块（最终回退策略），同时遵守 overlap
// 既保证每个 chunk 不超过 maxChunkSize 字节（避免触发 Milvus VARCHAR 上限），
// 又保留行号映射；连续 chunk 之间按 overlap 字节回退起点以保留上下文。
func (s *TreeSitterSplitter) splitByLines(content string, filePath string, lang string, baseLineNum int) []model.CodeChunk {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	lines := strings.Split(content, "\n")
	var chunks []model.CodeChunk

	i := 0
	for i < len(lines) {
		var buf strings.Builder
		start := i
		for i < len(lines) {
			line := lines[i]
			// 单行就超过上限：硬切（极少见，但 minified 文件会触发）
			if buf.Len() == 0 && len(line) > s.maxChunkSize {
				for off := 0; off < len(line); off += s.maxChunkSize {
					end := min(off+s.maxChunkSize, len(line))
					chunks = append(chunks, model.CodeChunk{
						Content:      line[off:end],
						RelativePath: filePath,
						StartLine:    baseLineNum + i,
						EndLine:      baseLineNum + i,
						Language:     lang,
					})
				}
				i++
				start = i
				continue
			}
			// 加入这一行会超过上限，停止累积
			extra := len(line)
			if buf.Len() > 0 {
				extra++ // \n
			}
			if buf.Len()+extra > s.maxChunkSize {
				break
			}
			if buf.Len() > 0 {
				buf.WriteByte('\n')
			}
			buf.WriteString(line)
			i++
		}

		if strings.TrimSpace(buf.String()) != "" {
			chunks = append(chunks, model.CodeChunk{
				Content:      buf.String(),
				RelativePath: filePath,
				StartLine:    baseLineNum + start,
				EndLine:      baseLineNum + i - 1,
				Language:     lang,
			})
		}

		// overlap：下一段起点回退若干字节对应的行数
		if s.overlap > 0 && i < len(lines) && i > start {
			back := 0
			j := i - 1
			for j > start && back < s.overlap {
				back += len(lines[j]) + 1
				j--
			}
			i = j + 1
		}
	}

	return chunks
}

// fallbackSplit 当 tree-sitter 不支持或解析失败时的回退分块策略
func (s *TreeSitterSplitter) fallbackSplit(content string, filePath string, lang string) ([]model.CodeChunk, error) {
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}
	return s.splitByLines(content, filePath, lang, 1), nil
}
