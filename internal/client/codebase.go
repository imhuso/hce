package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config 项目级配置（落到 .hce/config.json）
type Config struct {
	CodebaseID string `json:"codebase_id"`
	BaseURL    string `json:"base_url,omitempty"`
}

// HCEDir 项目内 hce 工作目录
const HCEDir = ".hce"

// ConfigFile 项目配置文件名
const ConfigFile = "config.json"

// IndexFile 项目索引文件名
const IndexFile = "index.json"

// LoadOrInit 读取 .hce/config.json；不存在则在 root 下创建并返回。codebase_id 默认派生自 root 绝对路径。
func LoadOrInit(root string) (*Config, error) {
	dir := filepath.Join(root, HCEDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建 %s 失败: %w", dir, err)
	}
	cfgPath := filepath.Join(dir, ConfigFile)

	data, err := os.ReadFile(cfgPath)
	if err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", cfgPath, err)
		}
		if cfg.CodebaseID == "" {
			cfg.CodebaseID = deriveCodebaseID(root)
		}
		return &cfg, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	// 首次：基于绝对路径派生稳定 ID + 项目目录名做提示
	cfg := &Config{CodebaseID: deriveCodebaseID(root)}
	if err := SaveConfig(root, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveConfig 保存 .hce/config.json
func SaveConfig(root string, cfg *Config) error {
	dir := filepath.Join(root, HCEDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ConfigFile), data, 0o644)
}

// deriveCodebaseID 用 "<dirName>-<hash8>" 形式，可读且全局唯一
func deriveCodebaseID(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	h := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%s-%s", filepath.Base(abs), hex.EncodeToString(h[:])[:8])
}

// FindProjectRoot 从 start 向上找到含 .hce/ 或 .git/ 的目录；都没有就返回 start。
func FindProjectRoot(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return start
	}
	cur := abs
	for {
		if isDir(filepath.Join(cur, HCEDir)) || isDir(filepath.Join(cur, ".git")) {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		cur = parent
	}
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
