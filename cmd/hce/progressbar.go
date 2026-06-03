package main

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

// isStderrTTY 判断 stderr 是否绑定到一个交互终端（决定是否启用动画进度条）
func isStderrTTY() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// progressBar 简单 ANSI 进度条（仅在 TTY 启用）。
// 输出形如：
//   [██████████░░░░░░░░░░] 50% 5/10 批 ETA 1m20s • 批 #5/10 发送中 (50 文件 / 1.2 MiB)
type progressBar struct {
	total     int
	startedAt time.Time
	width     int
}

func newProgressBar(total int) *progressBar {
	if total <= 0 {
		total = 1
	}
	return &progressBar{total: total, startedAt: time.Now(), width: 24}
}

func (p *progressBar) update(done int, hint string) {
	if done < 0 {
		done = 0
	}
	if done > p.total {
		done = p.total
	}
	pct := float64(done) / float64(p.total)
	filled := int(pct * float64(p.width))
	bar := make([]byte, 0, p.width*3)
	for range filled {
		bar = append(bar, "█"...)
	}
	for range p.width - filled {
		bar = append(bar, "░"...)
	}

	eta := ""
	if done > 0 && done < p.total {
		elapsed := time.Since(p.startedAt)
		per := elapsed / time.Duration(done)
		remaining := per * time.Duration(p.total-done)
		eta = " ETA " + humanDuration(remaining)
	}

	// \r 回到行首；\033[K 清除到行尾
	fmt.Fprintf(os.Stderr, "\r\033[K[%s] %3d%% %d/%d 批%s • %s",
		string(bar), int(pct*100), done, p.total, eta, hint)
}

func (p *progressBar) finish() {
	fmt.Fprintln(os.Stderr)
}

func humanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d / time.Minute)
	s := int((d % time.Minute) / time.Second)
	if m < 60 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	h := m / 60
	m = m % 60
	return fmt.Sprintf("%dh%02dm", h, m)
}
