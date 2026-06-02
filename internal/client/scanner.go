package client

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultExtensions 默认会被索引的源码扩展名
var DefaultExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true,
	".java": true, ".c": true, ".h": true, ".cpp": true, ".hpp": true, ".cc": true,
	".cs": true, ".rs": true, ".rb": true, ".php": true, ".swift": true, ".kt": true,
	".scala": true, ".lua": true, ".sh": true, ".bash": true, ".md": true,
}

// MaxFileSize 单文件大小上限（>1MB 跳过）
const MaxFileSize = 1024 * 1024

// defaultIgnorePatterns 默认忽略规则；与 .gitignore 合并使用
func defaultIgnorePatterns() []string {
	return []string{
		"node_modules", "vendor", "dist", "build", "out", "target", "coverage",
		".git", ".svn", ".hg", ".idea", ".vscode",
		"__pycache__", ".pytest_cache", ".nyc_output",
		"logs", "tmp", "temp", ".cache",
		".hce",
		"*.min.js", "*.min.css", "*.map", "*.bundle.js",
		"*.lock", "go.sum", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
	}
}

// ScanOptions 扫描参数
type ScanOptions struct {
	Extensions map[string]bool // nil 表示用默认
	MaxSize    int64           // 0 表示用默认
}

// FileEntry 扫描得到的候选文件
type FileEntry struct {
	AbsPath      string
	RelativePath string
	Size         int64
	ModTime      time.Time
}

// Scan 遍历 root，返回符合条件的文件（统一用 forward-slash 相对路径，跨平台一致）
func Scan(root string, opts ScanOptions) ([]FileEntry, error) {
	exts := opts.Extensions
	if exts == nil {
		exts = DefaultExtensions
	}
	maxSize := opts.MaxSize
	if maxSize <= 0 {
		maxSize = MaxFileSize
	}

	patterns := append([]string(nil), defaultIgnorePatterns()...)
	patterns = append(patterns, loadGitignore(root)...)

	var out []FileEntry
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		// 统一为 forward-slash，传到服务端 / 写入 index.json 时跨平台稳定
		relSlash := filepath.ToSlash(rel)

		if info.IsDir() {
			if rel == "." {
				return nil
			}
			if shouldIgnoreDir(patterns, relSlash) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldIgnoreFile(patterns, relSlash) {
			return nil
		}
		if !exts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		if info.Size() > maxSize {
			return nil
		}
		out = append(out, FileEntry{
			AbsPath:      path,
			RelativePath: relSlash,
			Size:         info.Size(),
			ModTime:      info.ModTime(),
		})
		return nil
	})
	return out, err
}

func loadGitignore(root string) []string {
	f, err := os.Open(filepath.Join(root, ".gitignore"))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func shouldIgnoreDir(patterns []string, relPathSlash string) bool {
	dirName := pathBase(relPathSlash)
	if strings.HasPrefix(dirName, ".") && dirName != "." {
		return true
	}
	for _, raw := range patterns {
		p := strings.TrimSuffix(strings.TrimSuffix(raw, "/"), "/**")
		if p == dirName {
			return true
		}
		if strings.HasPrefix(relPathSlash, p+"/") || relPathSlash == p {
			return true
		}
		if matched, _ := filepath.Match(p, dirName); matched {
			return true
		}
	}
	return false
}

func shouldIgnoreFile(patterns []string, relPathSlash string) bool {
	name := pathBase(relPathSlash)
	for _, p := range patterns {
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
		if matched, _ := filepath.Match(p, relPathSlash); matched {
			return true
		}
	}
	return false
}

func pathBase(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return p
	}
	return p[i+1:]
}
