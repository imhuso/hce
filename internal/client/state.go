package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// IndexState 客户端索引状态：path → 文件 hash
type IndexState struct {
	Version  int                  `json:"version"`
	Files    map[string]FileState `json:"files"`
	LastSync time.Time            `json:"last_sync,omitempty"`
}

// FileState 单文件状态
type FileState struct {
	SHA256  string    `json:"sha256"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mtime,omitempty"` // 用于秒级跳过 sha256：mtime+size 一致就认为内容未变
}

// LoadState 读取 .hce/index.json；不存在返回空 state（不报错）
func LoadState(root string) (*IndexState, error) {
	p := filepath.Join(root, HCEDir, IndexFile)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &IndexState{Version: 1, Files: make(map[string]FileState)}, nil
		}
		return nil, err
	}
	var st IndexState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	if st.Files == nil {
		st.Files = make(map[string]FileState)
	}
	return &st, nil
}

// SaveState 写回 .hce/index.json
func SaveState(root string, st *IndexState) error {
	p := filepath.Join(root, HCEDir, IndexFile)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
