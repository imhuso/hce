package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/imhuso/hce/internal/client"
)

const defaultBaseURL = "http://localhost:9528/api/v1"

const usage = `hce-cli — HCE 代码语义检索客户端

用法:
  hce-cli sync                              扫描并把变更推送到服务端
  hce-cli search <query> [opts]             语义搜索（默认先 sync，--no-sync 跳过）
  hce-cli status                            显示当前 codebase 配置 / 上次 sync
  hce-cli list                              列出服务端所有已索引集合
  hce-cli clear                             清除当前 codebase 的服务端索引
  hce-cli init [--id <name>]                显式初始化 .hce/config.json

通用选项:
  -p <path>          指定项目根（默认从当前目录向上找 .hce 或 .git；都没有用当前目录）
  --base-url <url>   覆盖服务端地址（也可用环境变量 HCE_BASE_URL）

search 选项:
  -k <int>           top_k，默认 5
  -f text|json       输出格式，默认 text
  --no-sync          跳过 sync，仅做搜索
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	sub := os.Args[1]
	args := os.Args[2:]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		cancel()
	}()

	switch sub {
	case "sync":
		os.Exit(cmdSync(ctx, args))
	case "search":
		os.Exit(cmdSearch(ctx, args))
	case "status":
		os.Exit(cmdStatus(ctx, args))
	case "list":
		os.Exit(cmdList(ctx, args))
	case "clear":
		os.Exit(cmdClear(ctx, args))
	case "init":
		os.Exit(cmdInit(ctx, args))
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n%s", sub, usage)
		os.Exit(2)
	}
}

// 通用 flag 解析：抽出 -p / --base-url / --rps
type commonFlags struct {
	Root        string
	BaseURL     string
	RPS         float64
	TPM         int
	Concurrency int
}

func parseCommon(fs *flag.FlagSet, args []string) (*commonFlags, []string) {
	cf := &commonFlags{}
	fs.StringVar(&cf.Root, "p", "", "项目根目录")
	fs.StringVar(&cf.BaseURL, "base-url", "", "服务端 base URL")
	fs.Float64Var(&cf.RPS, "rps", 4.0, "每秒最多多少次 upsert 请求（控 RPM 维度；0=不限）")
	// TPM 默认 0：服务端 provider 自己根据自身限额做内置限速，客户端无须再加一层。
	// 仅在客户端想做提前节流（避免 server 长时间排队）时手动 --tpm 设置。
	fs.IntVar(&cf.TPM, "tpm", 0, "每分钟最多多少 tokens（0=由服务端 provider 自适应限速）")
	// 默认 1：服务端按 collection 串行
	fs.IntVar(&cf.Concurrency, "concurrency", 1, "并发上传 worker 数（单 codebase 索引保持 1；多 codebase 同时索引可调高）")
	_ = fs.Parse(args)
	return cf, fs.Args()
}

func resolveContext(cf *commonFlags) (root string, cfg *client.Config, err error) {
	root = cf.Root
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", nil, err
		}
		root = client.FindProjectRoot(cwd)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", nil, err
	}
	cfg, err = client.LoadOrInit(root)
	if err != nil {
		return "", nil, err
	}
	if v := cf.BaseURL; v != "" {
		cfg.BaseURL = v
	} else if v := os.Getenv("HCE_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	return root, cfg, nil
}

func warn(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}

func cmdSync(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	cf, _ := parseCommon(fs, args)
	root, cfg, err := resolveContext(cf)
	if err != nil {
		warn("✘ %v", err)
		return 1
	}
	rep, err := runSyncWithTPM(ctx, root, cfg, true, cf.RPS, cf.TPM, cf.Concurrency)
	if err != nil {
		warn("✘ sync 失败: %v", err)
		return 1
	}
	printSyncSummary(rep, cfg)
	return 0
}

func runSyncWithTPM(ctx context.Context, root string, cfg *client.Config, verbose bool, rps float64, tpm int, concurrency int) (*client.SyncReport, error) {
	progress := func(string, ...any) {}
	if verbose {
		isTTY := isStderrTTY()
		var pb *progressBar
		progress = func(stage string, a ...any) {
			switch stage {
			case "scan-start":
				fmt.Fprintln(os.Stderr, "→ 扫描代码库...")
			case "scan-done":
				fmt.Fprintf(os.Stderr, "  扫描完成: %d 个候选文件\n", a[0])
			case "diff-start":
				fmt.Fprintf(os.Stderr, "→ 比对索引 (%d 文件)...\n", a[0])
			case "diff-progress":
				if isTTY {
					fmt.Fprintf(os.Stderr, "\r\033[K  · 已比对 %d/%d", a[0], a[1])
				}
				// 非 TTY：500 行一次的进度（由 sync.go 控制频率），但当文件数 < 1000 时不打
			case "diff-done":
				if isTTY {
					fmt.Fprint(os.Stderr, "\r\033[K") // 清掉进度行
				}
				hashed := 0
				if len(a) >= 4 {
					hashed = a[3].(int)
				}
				fmt.Fprintf(os.Stderr, "  比对完成: 新增=%d 修改=%d 未变=%d  (实际算 hash=%d)\n",
					a[0], a[1], a[2], hashed)
			case "delete":
				fmt.Fprintf(os.Stderr, "→ 删除已移除文件的索引: %d 个\n", a[0])
			case "upsert-start":
				fmt.Fprintf(os.Stderr, "→ 推送 %d 个文件到服务端...\n", a[0])
			case "upsert-batch":
				batchIdx, total := a[0].(int), a[1].(int)
				if isTTY {
					if pb == nil {
						pb = newProgressBar(total)
					}
					pb.update(batchIdx-1, fmt.Sprintf("批 #%d/%d 发送中 (%d 文件 / %s)", batchIdx, total, a[2], humanBytes(a[3].(int64))))
				} else {
					fmt.Fprintf(os.Stderr, "  · 批 #%d/%d 发送中: %d 文件 / %s ...\n",
						batchIdx, total, a[2], humanBytes(a[3].(int64)))
				}
			case "upsert-batch-done":
				batchIdx, total := a[0].(int), a[1].(int)
				dur := a[5].(time.Duration)
				if isTTY {
					if pb == nil {
						pb = newProgressBar(total)
					}
					pb.update(batchIdx, fmt.Sprintf("批 #%d/%d 完成 (新EMB=%d 复用=%d 删除=%d, %s)",
						batchIdx, total, a[2], a[3], a[4], dur.Round(time.Millisecond)))
					if batchIdx >= total {
						pb.finish()
					}
				} else {
					fmt.Fprintf(os.Stderr, "  · 批 #%d/%d 完成: 新 EMB=%d  复用=%d  删除=%d  耗时=%s\n",
						batchIdx, total, a[2], a[3], a[4], dur.Round(time.Millisecond))
				}
			case "flush-start":
				fmt.Fprintln(os.Stderr, "→ 让服务端落盘 (flush)...")
			case "flush-done":
				fmt.Fprintln(os.Stderr, "  flush 完成")
			case "flush-warn":
				fmt.Fprintf(os.Stderr, "  ⚠ flush 失败（数据已写入，但暂不可被搜索；下次 sync 时会再触发）: %v\n", a[0])
			case "rate-wait":
				d := a[0].(time.Duration)
				if d > 200*time.Millisecond {
					fmt.Fprintf(os.Stderr, "  · RPS 限速等待 %s...\n", d.Round(100*time.Millisecond))
				}
			case "tpm-wait":
				d := a[0].(time.Duration)
				tokens := a[1].(int)
				if d > 200*time.Millisecond {
					fmt.Fprintf(os.Stderr, "  · TPM 限速等待 %s（本批约 %d tokens）...\n",
						d.Round(100*time.Millisecond), tokens)
				}
			case "save-state-warn":
				fmt.Fprintf(os.Stderr, "  ⚠ state 落盘失败（不影响推送，但下次会重做）: %v\n", a[0])
			case "binary-skip":
				fmt.Fprintf(os.Stderr, "  · 跳过二进制 %s\n", a[0])
			case "utf8-skip":
				fmt.Fprintf(os.Stderr, "  · 跳过非 UTF-8 文件 %s\n", a[0])
			case "batch-failed":
				fmt.Fprintf(os.Stderr, "  ⚠ 批 #%d 失败（已计入失败统计，不影响后续批）: %v\n", a[0], a[1])
			case "read-skip", "hash-skip":
				fmt.Fprintf(os.Stderr, "  · 跳过 %v: %v\n", a[0], a[1])
			}
		}
	}
	return client.Sync(ctx, root, cfg, client.SyncOptions{
		Progress: progress, RPS: rps, TPM: tpm, Concurrency: concurrency,
	})
}

func humanBytes(n int64) string {
	const k = 1024
	switch {
	case n < k:
		return fmt.Sprintf("%d B", n)
	case n < k*k:
		return fmt.Sprintf("%.1f KiB", float64(n)/k)
	default:
		return fmt.Sprintf("%.2f MiB", float64(n)/(k*k))
	}
}

func printSyncSummary(rep *client.SyncReport, cfg *client.Config) {
	totalNew, totalReused, totalDeleted, files := 0, 0, rep.ChunksDeleted, 0
	for _, r := range rep.UpsertResults {
		totalNew += r.ChunksNewEmbedded
		totalReused += r.ChunksReused
		totalDeleted += r.ChunksDeleted
		files += r.FilesProcessed
	}
	fmt.Printf("✓ codebase=%s  耗时=%s\n", cfg.CodebaseID, rep.Duration.Round(time.Millisecond))
	fmt.Printf("  扫描=%d  新增=%d  修改=%d  删除=%d  未变=%d\n",
		rep.Scanned, rep.Added, rep.Modified, rep.Removed, rep.Unchanged)
	fmt.Printf("  推送文件=%d  新 EMB=%d  复用 chunk=%d  删除 chunk=%d\n",
		files, totalNew, totalReused, totalDeleted)
	if len(rep.FailedBatches) > 0 {
		fmt.Printf("  ⚠ 失败批次=%d  （已自动跳过；这些文件下次 sync 会重试）\n", len(rep.FailedBatches))
		max := 3
		if len(rep.FailedBatches) < max {
			max = len(rep.FailedBatches)
		}
		for _, fb := range rep.FailedBatches[:max] {
			fmt.Printf("    · 批 #%d (%d 文件): %s\n", fb.Idx, fb.Files, fb.Err)
		}
		if len(rep.FailedBatches) > max {
			fmt.Printf("    · ...还有 %d 批\n", len(rep.FailedBatches)-max)
		}
	}
}

func cmdSearch(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	var (
		topK   int
		format string
		noSync bool
	)
	// search 私有 flag 与通用 flag 注册到同一 flagset，避免两套 flagset 组合时
	// -k 等被漏解析、其值混入 query（如 "-p x -k 10 查询" 把 "10" 并进查询词）。
	fs.IntVar(&topK, "k", 10, "top_k（小项目可降到 5；大型代码库建议 10-20 以召回调用方）")
	fs.StringVar(&format, "f", "text", "输出格式：text | json")
	fs.BoolVar(&noSync, "no-sync", false, "跳过 sync")
	cf, rest := parseCommon(fs, args)
	if len(rest) == 0 {
		warn("用法: hce-cli search <query> [...]")
		return 2
	}
	query := strings.Join(rest, " ")

	root, cfg, err := resolveContext(cf)
	if err != nil {
		warn("✘ %v", err)
		return 1
	}

	if !noSync {
		if _, err := runSyncWithTPM(ctx, root, cfg, true, cf.RPS, cf.TPM, cf.Concurrency); err != nil {
			warn("⚠ sync 失败（继续用既有索引搜索）: %v", err)
		}
	}
	fmt.Fprintf(os.Stderr, "→ 搜索: %q  (top_k=%d)\n", query, topK)

	http := client.NewHTTP(cfg.BaseURL)
	if format == "json" {
		results, err := http.Search(ctx, cfg.CodebaseID, query, topK)
		if err != nil {
			warn("✘ search 失败: %v", err)
			return 1
		}
		// 直接以紧凑 JSON 行输出
		for _, r := range results {
			fmt.Printf("%s:%d-%d  score=%.3f  %s\n", r.RelativePath, r.StartLine, r.EndLine, r.Score, oneLine(r.Content))
		}
		return 0
	}
	text, err := http.SearchText(ctx, cfg.CodebaseID, query, topK)
	if err != nil {
		warn("✘ search 失败: %v", err)
		return 1
	}
	fmt.Print(text)
	return 0
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ⏎ ")
	if len(s) > 160 {
		s = s[:160] + "..."
	}
	return s
}

func cmdStatus(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	cf, _ := parseCommon(fs, args)
	root, cfg, err := resolveContext(cf)
	if err != nil {
		warn("✘ %v", err)
		return 1
	}
	state, err := client.LoadState(root)
	if err != nil {
		warn("✘ %v", err)
		return 1
	}
	http := client.NewHTTP(cfg.BaseURL)
	online := http.Health(ctx) == nil

	fmt.Printf("项目根     : %s\n", root)
	fmt.Printf("codebase_id: %s\n", cfg.CodebaseID)
	fmt.Printf("base_url   : %s  (%s)\n", cfg.BaseURL, ifTernary(online, "在线", "离线"))
	fmt.Printf("已索引文件 : %d\n", len(state.Files))
	if !state.LastSync.IsZero() {
		fmt.Printf("上次 sync  : %s\n", state.LastSync.Format(time.RFC3339))
	}
	return 0
}

func ifTernary(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}

func cmdList(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	cf, _ := parseCommon(fs, args)
	_, cfg, err := resolveContext(cf)
	if err != nil {
		warn("✘ %v", err)
		return 1
	}
	http := client.NewHTTP(cfg.BaseURL)
	items, err := http.List(ctx)
	if err != nil {
		warn("✘ list 失败: %v", err)
		return 1
	}
	if len(items) == 0 {
		fmt.Println("（无）")
		return 0
	}
	fmt.Printf("%-40s %s\n", "COLLECTION", "CHUNKS")
	for _, x := range items {
		fmt.Printf("%-40s %d\n", x.Collection, x.NumChunks)
	}
	return 0
}

func cmdClear(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("clear", flag.ContinueOnError)
	cf, _ := parseCommon(fs, args)
	root, cfg, err := resolveContext(cf)
	if err != nil {
		warn("✘ %v", err)
		return 1
	}
	http := client.NewHTTP(cfg.BaseURL)
	if err := http.Clear(ctx, cfg.CodebaseID); err != nil {
		warn("✘ clear 失败: %v", err)
		return 1
	}
	// 同步清掉本地 state
	_ = os.Remove(filepath.Join(root, client.HCEDir, client.IndexFile))
	fmt.Printf("✓ 已清除 codebase=%s 的服务端索引和本地 state\n", cfg.CodebaseID)
	return 0
}

func cmdInit(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	id := fs.String("id", "", "指定 codebase_id")
	cf, _ := parseCommon(fs, args)

	root := cf.Root
	if root == "" {
		cwd, _ := os.Getwd()
		root = cwd
	}
	abs, _ := filepath.Abs(root)
	cfg, err := client.LoadOrInit(abs)
	if err != nil {
		warn("✘ %v", err)
		return 1
	}
	if *id != "" {
		cfg.CodebaseID = *id
		if err := client.SaveConfig(abs, cfg); err != nil {
			warn("✘ %v", err)
			return 1
		}
	}
	fmt.Printf("✓ 已初始化 %s\n  codebase_id = %s\n", filepath.Join(abs, client.HCEDir), cfg.CodebaseID)
	_ = ctx
	return 0
}
// log demo 1777091206
