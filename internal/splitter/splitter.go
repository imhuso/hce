package splitter

import "github.com/imhuso/hce/pkg/model"

// Splitter 代码分块器接口
type Splitter interface {
	// Split 将文件内容按语义边界分块
	Split(content string, filePath string) ([]model.CodeChunk, error)

	// SupportedLanguages 返回该分块器支持的语言列表
	SupportedLanguages() []string
}

// 语言与文件扩展名的映射
var ExtToLanguage = map[string]string{
	".go":    "go",
	".py":    "python",
	".js":    "javascript",
	".jsx":   "javascript",
	".ts":    "typescript",
	".tsx":   "typescript",
	".java":  "java",
	".c":     "c",
	".h":     "c",
	".cpp":   "cpp",
	".hpp":   "cpp",
	".cc":    "cpp",
	".cs":    "csharp",
	".rs":    "rust",
	".rb":    "ruby",
	".php":   "php",
	".swift": "swift",
	".kt":    "kotlin",
	".scala": "scala",
	".m":     "objc",
	".mm":    "objc",
	".lua":   "lua",
	".sh":    "bash",
	".bash":  "bash",
	".md":    "markdown",
}

// DetectLanguage 根据文件扩展名检测编程语言
func DetectLanguage(filePath string) string {
	for ext, lang := range ExtToLanguage {
		if len(filePath) > len(ext) && filePath[len(filePath)-len(ext):] == ext {
			return lang
		}
	}
	return "unknown"
}
