package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	gosync "sync"
	"time"
	"unicode/utf8"

	"golang.org/x/time/rate"

	"github.com/imhuso/hce/pkg/model"
)

// SyncOptions sync 控制参数
type SyncOptions struct {
	BatchSize   int     // 单次 upsert 的最大文件数；默认 50
	BatchBytes  int64   // 单次 upsert 的最大累计字节；默认 5 MiB
	RPS         float64 // 每秒最多多少次 upsert 请求（避免 EMB 提供商 RPM 限流）；0 = 无限制
	TPM         int     // 每分钟最多多少 tokens（避免 EMB 提供商 TPM 限流）；0 = 无限制
	Concurrency int     // 并发上传的 worker 数；默认 4
	Progress    func(stage string, args ...any)
}

// 把 batch 字节数估算为 token 数：英文/代码约 3.5 字节/token。
// 偏保守（除以 3）让客户端比真实 token 更早进入限速，避免触发 TPM 上限。
func estimateTokens(batchBytes int64) int {
	if batchBytes <= 0 {
		return 1
	}
	return int(batchBytes / 3)
}

// SyncReport sync 一次的统计
type SyncReport struct {
	Scanned       int
	Added         int
	Modified      int
	Removed       int
	Unchanged     int
	UpsertResults []model.UpsertResult
	ChunksDeleted int
	FailedBatches []FailedBatch // 单批失败不中断整体；这里记录每个失败 batch 的诊断
	Duration      time.Duration
}

// FailedBatch 单个失败 batch 的诊断信息
type FailedBatch struct {
	Idx   int
	Files int
	Err   string
}

// Sync 比较本地与 .hce/index.json，把新增/修改的文件推上去，删除已删除的；最后落盘 index.json。
func Sync(ctx context.Context, root string, cfg *Config, opts SyncOptions) (*SyncReport, error) {
	if opts.BatchSize <= 0 {
		opts.BatchSize = 50
	}
	if opts.BatchBytes <= 0 {
		opts.BatchBytes = 5 * 1024 * 1024
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	progress := opts.Progress
	if progress == nil {
		progress = func(string, ...any) {}
	}

	t0 := time.Now()
	state, err := LoadState(root)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	progress("scan-start")
	files, err := Scan(root, ScanOptions{})
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	progress("scan-done", len(files))

	rep := &SyncReport{Scanned: len(files)}

	// 1. 算 hash 并 diff
	var toPush []pending
	currentSet := make(map[string]struct{}, len(files))

	progress("diff-start", len(files))
	hashedFiles := 0
	for i, f := range files {
		currentSet[f.RelativePath] = struct{}{}
		old, ok := state.Files[f.RelativePath]

		// 快速路径：state 里已有该文件，且 size + mtime 一致 → 直接判定未变，跳过 sha256。
		// 这是大库 sync 速度的关键：4000+ 文件秒过，只有真正变更的才算 hash。
		if ok && old.Size == f.Size && !old.ModTime.IsZero() && old.ModTime.Equal(f.ModTime) {
			rep.Unchanged++
			if len(files) > 200 && (i+1)%500 == 0 {
				progress("diff-progress", i+1, len(files))
			}
			continue
		}

		// 慢路径：算 sha256 验证内容
		hash, err := hashFile(f.AbsPath)
		if err != nil {
			progress("hash-skip", f.RelativePath, err)
			continue
		}
		hashedFiles++
		if !ok {
			rep.Added++
			toPush = append(toPush, pending{f, hash})
		} else if old.SHA256 != hash {
			rep.Modified++
			toPush = append(toPush, pending{f, hash})
		} else {
			// hash 相同但 mtime 变了——更新 state.mtime 避免下次再 hash
			old.ModTime = f.ModTime
			state.Files[f.RelativePath] = old
			rep.Unchanged++
		}
		if len(files) > 200 && (i+1)%500 == 0 {
			progress("diff-progress", i+1, len(files))
		}
	}
	progress("diff-done", rep.Added, rep.Modified, rep.Unchanged, hashedFiles)

	// 2. 计算需要删除的文件（旧 state 里有，新扫描没有）
	var toDelete []string
	for relPath := range state.Files {
		if _, ok := currentSet[relPath]; !ok {
			toDelete = append(toDelete, relPath)
			rep.Removed++
		}
	}

	http := NewHTTP(cfg.BaseURL)

	// 限速 1：RPS（请求数/秒，对应 RPM 上限）
	var rpsLimiter *rate.Limiter
	if opts.RPS > 0 {
		rpsLimiter = rate.NewLimiter(rate.Limit(opts.RPS), 1)
	}
	// 限速 2：TPM（tokens/分钟，对应 TPM 上限）—— 这是 Gemini paid tier 真正的瓶颈
	var tpmLimiter *rate.Limiter
	if opts.TPM > 0 {
		tps := float64(opts.TPM) / 60.0
		// burst 设成 TPM 上限：允许一次性预订一整分钟的配额作为冷启动
		tpmLimiter = rate.NewLimiter(rate.Limit(tps), opts.TPM)
	}

	waitLimiter := func(tokens int) error {
		if rpsLimiter != nil {
			if r := rpsLimiter.Reserve(); r.OK() {
				if d := r.Delay(); d > 0 {
					progress("rate-wait", d)
					select {
					case <-ctx.Done():
						r.Cancel()
						return ctx.Err()
					case <-time.After(d):
					}
				}
			}
		}
		if tpmLimiter != nil && tokens > 0 {
			// WaitN 会等到有 tokens 个配额可用
			r := tpmLimiter.ReserveN(time.Now(), tokens)
			if r.OK() {
				if d := r.Delay(); d > 0 {
					progress("tpm-wait", d, tokens)
					select {
					case <-ctx.Done():
						r.Cancel()
						return ctx.Err()
					case <-time.After(d):
					}
				}
			}
		}
		return nil
	}

	// 3. 删除已不存在的
	if len(toDelete) > 0 {
		progress("delete", len(toDelete))
		n, err := http.DeleteFiles(ctx, cfg.CodebaseID, toDelete)
		if err != nil {
			return nil, fmt.Errorf("delete: %w", err)
		}
		rep.ChunksDeleted += n
		for _, p := range toDelete {
			delete(state.Files, p)
		}
	}

	// 4. 并发 upsert：先把所有 batch 在主 goroutine 组装好（顺序读文件），
	//    然后丢进 channel 由 N 个 worker 并发推送。全局 limiter 控速；state 共享变量加锁。
	if len(toPush) > 0 {
		progress("upsert-start", len(toPush))
		totalBatches := estimateBatches(toPush, opts.BatchSize, opts.BatchBytes)

		type uploadJob struct {
			Idx     int
			Files   []model.PushFile
			Pending []pending
			Bytes   int64
		}
		jobs := make(chan uploadJob, opts.Concurrency*2)

		// state mutex：worker 并发更新 state.Files / 落盘 / 错误计数
		var stateMu gosync.Mutex

		// 容错：单批失败不让整体退出。仅当 ctx 被取消时退出。
		var ctxErr error

		var wg gosync.WaitGroup
		for w := 0; w < opts.Concurrency; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobs {
					if err := waitLimiter(estimateTokens(job.Bytes)); err != nil {
						stateMu.Lock()
						ctxErr = err
						stateMu.Unlock()
						return
					}
					progress("upsert-batch", job.Idx, totalBatches, len(job.Files), job.Bytes)
					t0 := time.Now()
					stats, err := http.Upsert(ctx, cfg.CodebaseID, job.Files)
					if err != nil {
						stateMu.Lock()
						rep.FailedBatches = append(rep.FailedBatches, FailedBatch{
							Idx: job.Idx, Files: len(job.Files), Err: err.Error(),
						})
						stateMu.Unlock()
						progress("batch-failed", job.Idx, err)
						continue // 单批失败不退出 worker
					}
					progress("upsert-batch-done", job.Idx, totalBatches,
						stats.ChunksNewEmbedded, stats.ChunksReused, stats.ChunksDeleted, time.Since(t0))

					stateMu.Lock()
					rep.UpsertResults = append(rep.UpsertResults, *stats)
					for _, p := range job.Pending {
						state.Files[p.Entry.RelativePath] = FileState{
							SHA256:  p.Hash,
							Size:    p.Entry.Size,
							ModTime: p.Entry.ModTime,
						}
					}
					state.LastSync = time.Now()
					if err := SaveState(root, state); err != nil {
						progress("save-state-warn", err)
					}
					stateMu.Unlock()
				}
			}()
		}

		// 主 goroutine：组装 batch 并丢进 jobs channel
		batch := make([]model.PushFile, 0, opts.BatchSize)
		batchBytes := int64(0)
		batchPending := make([]pending, 0, opts.BatchSize)
		batchIdx := 0
		dispatch := func() {
			if len(batch) == 0 {
				return
			}
			batchIdx++
			job := uploadJob{
				Idx:     batchIdx,
				Files:   append([]model.PushFile(nil), batch...),
				Pending: append([]pending(nil), batchPending...),
				Bytes:   batchBytes,
			}
			batch = batch[:0]
			batchBytes = 0
			batchPending = batchPending[:0]
			select {
			case jobs <- job:
			case <-ctx.Done():
			}
		}

	pushLoop:
		for _, p := range toPush {
			select {
			case <-ctx.Done():
				break pushLoop
			default:
			}
			content, err := os.ReadFile(p.Entry.AbsPath)
			if err != nil {
				progress("read-skip", p.Entry.RelativePath, err)
				continue
			}
			if isLikelyBinary(content) {
				progress("binary-skip", p.Entry.RelativePath)
				continue
			}
			if !utf8.Valid(content) {
				// 非 UTF-8（minified 库、二进制混入）会让 grpc marshal 失败拒收整批
				progress("utf8-skip", p.Entry.RelativePath)
				continue
			}
			sz := int64(len(content))
			if batchBytes+sz > opts.BatchBytes && len(batch) > 0 {
				dispatch()
			}
			batch = append(batch, model.PushFile{
				RelativePath: p.Entry.RelativePath,
				Content:      string(content),
			})
			batchPending = append(batchPending, p)
			batchBytes += sz
			if len(batch) >= opts.BatchSize {
				dispatch()
			}
		}
		dispatch()
		close(jobs)
		wg.Wait()

		// ctx 被取消才硬失败；批级错误已在 rep.FailedBatches 累计
		if ctxErr != nil {
			return nil, ctxErr
		}
		// 失败率守卫：> 50% 批失败说明系统性问题，应该让 CLI 报错让用户排查
		if totalBatches > 0 && len(rep.FailedBatches)*2 > totalBatches {
			return rep, fmt.Errorf("失败率过高 (%d/%d 批失败)，疑似系统性故障；请检查服务端日志",
				len(rep.FailedBatches), totalBatches)
		}
	}

	stateMu := &gosync.Mutex{}
	stateMu.Lock()
	state.LastSync = time.Now()
	if err := SaveState(root, state); err != nil {
		stateMu.Unlock()
		return nil, fmt.Errorf("save state: %w", err)
	}
	stateMu.Unlock()

	// 5. 推送完成后显式 flush 一次：让本次写入的 chunks 立即可被搜索。
	// 中间不 flush 是为了避免每批 small sealed segment → Milvus compaction storm。
	if len(toPush) > 0 {
		progress("flush-start")
		if err := http.Flush(ctx, cfg.CodebaseID); err != nil {
			progress("flush-warn", err)
		} else {
			progress("flush-done")
		}
	}

	rep.Duration = time.Since(t0)
	return rep, nil
}

type pending struct {
	Entry FileEntry
	Hash  string
}

// estimateBatches 预估 toPush 会被切成多少 upsert 批（用于进度条总数）
func estimateBatches(items []pending, batchSize int, batchBytes int64) int {
	if len(items) == 0 {
		return 0
	}
	count, n := 0, 0
	bytes := int64(0)
	for _, p := range items {
		// 用文件 size 做近似（实际推送以 content len 计；接近但不绝对相等）
		sz := p.Entry.Size
		if n > 0 && (n+1 > batchSize || bytes+sz > batchBytes) {
			count++
			n, bytes = 0, 0
		}
		n++
		bytes += sz
	}
	if n > 0 {
		count++
	}
	if count == 0 {
		count = 1
	}
	return count
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// isLikelyBinary 用前 8KB 是否含 NUL 字节做粗判断
func isLikelyBinary(b []byte) bool {
	n := len(b)
	if n > 8192 {
		n = 8192
	}
	for i := 0; i < n; i++ {
		if b[i] == 0 {
			return true
		}
	}
	return false
}
