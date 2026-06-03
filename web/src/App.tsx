import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import {
  Search, Loader2, RefreshCw, Terminal, ArrowLeft, Clock, Boxes,
  ChevronsDownUp, ChevronsUpDown, Copy, Check,
} from 'lucide-react';
import { searchCode, listIndexes, healthCheck, clearIndex } from '@/api';
import type { SearchResult, IndexInfo } from '@/api';
import {
  loadCodebases, rememberCodebase, collectionName, prettyId,
} from '@/lib/codebases';
import type { RememberedCodebase } from '@/lib/codebases';
import { loadRecent, pushRecent } from '@/lib/recent';
import type { RecentSearch } from '@/lib/recent';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { ThemeToggle } from '@/components/ThemeToggle';
import { BrandMark } from '@/components/BrandMark';
import { ScopeSelector } from '@/components/ScopeSelector';
import { CodebaseCard } from '@/components/CodebaseCard';
import type { ResolvedIndex } from '@/components/CodebaseCard';
import { ResultCard } from '@/components/ResultCard';
import { ConfirmDeleteDialog } from '@/components/ConfirmDeleteDialog';

type Status = { text: string; tone: 'success' | 'muted' | 'error' } | null;

const TOP_K_OPTIONS = [3, 5, 10, 20];

function App() {
  const [online, setOnline] = useState<boolean | null>(null);

  const [activeId, setActiveId] = useState('');
  const [query, setQuery] = useState('');
  const [topK, setTopK] = useState(5);
  const [results, setResults] = useState<SearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [status, setStatus] = useState<Status>(null);
  const [searchedId, setSearchedId] = useState('');

  const [codebases, setCodebases] = useState<RememberedCodebase[]>(loadCodebases);
  const [recent, setRecent] = useState<RecentSearch[]>(loadRecent);

  const [indexes, setIndexes] = useState<IndexInfo[]>([]);
  const [loadingIndexes, setLoadingIndexes] = useState(false);
  const [indexError, setIndexError] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; numChunks?: number } | null>(null);
  const [collapsed, setCollapsed] = useState<Record<number, boolean>>({});
  const [copiedAll, setCopiedAll] = useState(false);

  const queryRef = useRef<HTMLInputElement>(null);

  const loadIndexes = useCallback(async () => {
    setLoadingIndexes(true);
    setIndexError('');
    try {
      const res = await listIndexes();
      if (res.code === 0 && res.data) {
        setIndexes(res.data);
        // 首次加载时自动选中最大的具名代码库，省去一步
        const top = [...res.data]
          .filter((i) => i.codebase_id)
          .sort((a, b) => Number(b.num_chunks) - Number(a.num_chunks))[0];
        if (top) setActiveId((prev) => prev || top.codebase_id);
      } else {
        setIndexError(res.message || '加载失败');
      }
    } catch (e) {
      setIndexError(String(e));
    } finally {
      setLoadingIndexes(false);
    }
  }, []);

  // 健康检查；首次上线 / 掉线重连时拉取索引
  const wasOnline = useRef(false);
  useEffect(() => {
    let alive = true;
    const check = async () => {
      try {
        await healthCheck();
        if (!alive) return;
        setOnline(true);
        if (!wasOnline.current) { wasOnline.current = true; loadIndexes(); }
      } catch {
        if (!alive) return;
        setOnline(false);
        wasOnline.current = false;
      }
    };
    check();
    const t = setInterval(check, 10000);
    return () => { alive = false; clearInterval(t); };
  }, [loadIndexes]);

  const runSearch = useCallback(async (id: string, q: string) => {
    id = id.trim();
    q = q.trim();
    if (!id || !q) return;
    setSearching(true);
    setStatus(null);
    setResults([]);
    setSearchedId(id);
    try {
      const res = await searchCode(id, q, topK);
      if (res.code === 0 && res.data) {
        setCollapsed({});
        setResults(res.data);
        setStatus(
          res.data.length > 0
            ? { text: `找到 ${res.data.length} 条相关代码`, tone: 'success' }
            : { text: '没有匹配的代码', tone: 'muted' },
        );
        setCodebases(rememberCodebase(id));
        setRecent(pushRecent(id, q));
        if (online) loadIndexes();
      } else {
        setStatus({ text: res.message || '搜索失败', tone: 'error' });
      }
    } catch (e) {
      setStatus({ text: `请求失败：${e}`, tone: 'error' });
    } finally {
      setSearching(false);
    }
  }, [topK, online, loadIndexes]);

  const handleSelectCodebase = (id: string) => {
    setActiveId(id);
    if (query.trim()) runSearch(id, query);
    else queryRef.current?.focus();
  };

  const handleRecent = (r: RecentSearch) => {
    setActiveId(r.id);
    setQuery(r.query);
    runSearch(r.id, r.query);
  };

  const handleDelete = async (): Promise<string | undefined> => {
    if (!deleteTarget) return '无效的删除目标';
    try {
      const res = await clearIndex(deleteTarget.id);
      if (res.code !== 0) return res.message || '删除失败';
      if (activeId === deleteTarget.id) setActiveId('');
      await loadIndexes();
      return undefined;
    } catch (e) {
      return String(e);
    }
  };

  // 把 localStorage 记住的 id 映射到匿名 collection（服务端尚未命名时的兜底）
  const knownByCollection = useMemo(() => {
    const m = new Map<string, string>();
    for (const c of codebases) m.set(collectionName(c.id), c.id);
    return m;
  }, [codebases]);

  const resolved = useMemo<ResolvedIndex[]>(() => {
    return indexes
      .map((idx) => {
        const id = idx.codebase_id || knownByCollection.get(idx.collection) || '';
        return { collection: idx.collection, numChunks: Number(idx.num_chunks), id, named: !!id, languages: idx.languages };
      })
      .sort((a, b) => Number(b.named) - Number(a.named) || b.numChunks - a.numChunks);
  }, [indexes, knownByCollection]);

  const scopeOptions = useMemo(
    () => resolved.filter((r) => r.named).map((r) => ({ id: r.id, numChunks: r.numChunks })),
    [resolved],
  );

  const hasResults = results.length > 0 || (status !== null && searching === false && searchedId !== '');
  const searchedPretty = searchedId ? prettyId(searchedId) : null;

  const anyExpanded = results.some((_, i) => !collapsed[i]);
  const toggleCollapseAll = () => {
    if (anyExpanded) setCollapsed(Object.fromEntries(results.map((_, i) => [i, true])));
    else setCollapsed({});
  };
  const copyAll = async () => {
    const text = results.map((r) => `// ${r.relative_path}:${r.start_line}\n${r.content}`).join('\n\n');
    try {
      await navigator.clipboard.writeText(text);
      setCopiedAll(true);
      setTimeout(() => setCopiedAll(false), 1500);
    } catch { /* ignore */ }
  };
  const backToList = () => { setResults([]); setStatus(null); setSearchedId(''); setCollapsed({}); };

  return (
    <div className="relative min-h-screen overflow-x-hidden bg-background text-foreground">
      {/* 氛围层：技术网格 + 品牌辉光 */}
      <div aria-hidden className="pointer-events-none absolute inset-0 -z-10">
        <div className="absolute inset-x-0 top-0 h-[640px] bg-grid opacity-70" />
        <div className="glow-1 absolute left-1/2 top-[-200px] h-[460px] w-[900px] max-w-[130vw] -translate-x-1/2 blur-2xl" />
        <div className="glow-2 absolute right-[-140px] top-[60px] h-[340px] w-[340px] blur-3xl" />
      </div>

      {/* 顶部栏 */}
      <header className="sticky top-0 z-30 border-b border-border/70 bg-background/60 backdrop-blur-xl">
        <div className="mx-auto flex h-15 max-w-5xl items-center justify-between gap-3 px-4 py-2.5 sm:px-6">
          <div className="flex items-center gap-2.5">
            <BrandMark />
            <div className="flex items-baseline gap-2">
              <span className="text-[15px] font-semibold tracking-tight">HCE</span>
              <span className="hidden text-[13px] text-muted-foreground sm:inline">代码语义检索</span>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Badge
              variant="outline"
              className="gap-1.5 border-border/80 bg-card/60"
              title={online === null ? '连接中…' : online ? '服务端在线' : '连不上服务端'}
            >
              <span className={`size-1.5 rounded-full ${online === null ? 'bg-muted-foreground' : online ? 'bg-emerald-400 shadow-[0_0_8px] shadow-emerald-400/70' : 'bg-red-400'}`} />
              <span className="text-muted-foreground">{online === null ? '连接中' : online ? '在线' : '离线'}</span>
            </Badge>
            <ThemeToggle />
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-5xl px-4 pb-16 sm:px-6">
        {/* Hero 搜索 */}
        <section className="pt-12 sm:pt-16">
          <div className="mx-auto max-w-2xl">
            <div className="animate-rise flex justify-center" style={{ animationDelay: '40ms' }}>
              <span className="inline-flex items-center gap-1.5 rounded-full border border-border/70 bg-card/50 px-3 py-1 text-xs font-medium text-muted-foreground backdrop-blur">
                <span className="size-1.5 rounded-full bg-brand shadow-[0_0_10px] shadow-brand/70" />
                语义检索引擎
              </span>
            </div>
            <h2 className="animate-rise mt-5 text-center text-[1.7rem] font-semibold leading-[1.1] tracking-[-0.02em] text-balance sm:text-[2.6rem]" style={{ animationDelay: '90ms' }}>
              用自然语言检索你的代码库
            </h2>
            <div className="animate-rise mt-4 flex flex-wrap items-center justify-center gap-x-2 gap-y-1 text-sm text-muted-foreground" style={{ animationDelay: '140ms' }}>
              <span>在</span>
              <ScopeSelector value={activeId} onChange={setActiveId} options={scopeOptions} />
              <span>中检索</span>
            </div>

            <div className="animate-rise mt-5 flex flex-col gap-2 sm:flex-row sm:items-center" style={{ animationDelay: '190ms' }}>
              <div className="relative w-full min-w-0 sm:flex-1">
                <Search className="pointer-events-none absolute left-3.5 top-1/2 size-4.5 -translate-y-1/2 text-muted-foreground" />
                <input
                  ref={queryRef}
                  className="elevate h-12 w-full rounded-xl border border-border/80 bg-card pl-11 pr-3 text-base outline-none transition-all placeholder:text-muted-foreground focus-visible:border-brand/60 focus-visible:ring-4 focus-visible:ring-brand/20"
                  placeholder="描述你要找的逻辑，例如「登录鉴权」「文件上传接口」…"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && runSearch(activeId, query)}
                />
              </div>
              <div className="flex gap-2">
                <select
                  className="elevate h-12 flex-1 rounded-xl border border-border/80 bg-card px-3 text-sm outline-none focus-visible:border-brand/60 focus-visible:ring-4 focus-visible:ring-brand/20 sm:flex-none"
                  value={topK}
                  onChange={(e) => setTopK(Number(e.target.value))}
                  title="返回结果数量"
                >
                  {TOP_K_OPTIONS.map((n) => <option key={n} value={n}>Top {n}</option>)}
                </select>
                <Button
                  className="h-12 flex-1 px-6 text-base shadow-lg shadow-brand/25 sm:flex-none"
                  onClick={() => runSearch(activeId, query)}
                  disabled={searching || !activeId.trim() || !query.trim()}
                >
                  {searching ? <Loader2 className="animate-spin" /> : <Search />}
                  搜索
                </Button>
              </div>
            </div>

            {status && (
              <p
                className={`mt-3 text-center text-sm ${
                  status.tone === 'success'
                    ? 'text-emerald-600 dark:text-emerald-400'
                    : status.tone === 'error' ? 'text-destructive' : 'text-muted-foreground'
                }`}
              >
                {status.text}
              </p>
            )}
          </div>
        </section>

        {/* 结果 / 加载骨架 */}
        {searching && (
          <div className="mt-8 space-y-4">
            {[0, 1, 2].map((i) => (
              <div key={i} className="overflow-hidden rounded-xl ring-1 ring-foreground/10">
                <div className="flex items-center gap-3 border-b bg-muted/40 px-4 py-2.5">
                  <Skeleton className="size-5 rounded-md" />
                  <Skeleton className="h-4 w-48" />
                  <Skeleton className="ml-auto h-4 w-14" />
                </div>
                <div className="space-y-2 p-4">
                  <Skeleton className="h-3 w-5/6" />
                  <Skeleton className="h-3 w-2/3" />
                  <Skeleton className="h-3 w-3/4" />
                </div>
              </div>
            ))}
          </div>
        )}

        {!searching && hasResults && (
          <div className="mt-8">
            <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
              <p className="text-sm text-muted-foreground">
                {results.length > 0 && searchedPretty ? (
                  <>在 <span className="font-semibold text-foreground">{searchedPretty.name}</span> 找到 {results.length} 条</>
                ) : status?.tone === 'error' ? '搜索出错' : '无结果'}
              </p>
              <div className="flex items-center gap-0.5">
                {results.length > 0 && (
                  <>
                    <Button variant="ghost" size="sm" onClick={toggleCollapseAll}>
                      {anyExpanded ? <ChevronsDownUp /> : <ChevronsUpDown />}
                      {anyExpanded ? '全部收起' : '全部展开'}
                    </Button>
                    <Button variant="ghost" size="sm" onClick={copyAll}>
                      {copiedAll ? <Check className="text-emerald-500" /> : <Copy />}复制全部
                    </Button>
                  </>
                )}
                <Button variant="ghost" size="sm" onClick={backToList}>
                  <ArrowLeft />返回
                </Button>
              </div>
            </div>
            <div className="space-y-4">
              {results.map((r, i) => (
                <ResultCard
                  key={`${r.relative_path}-${r.start_line}-${i}`}
                  r={r}
                  rank={i + 1}
                  collapsed={!!collapsed[i]}
                  onToggleCollapse={() => setCollapsed((c) => ({ ...c, [i]: !c[i] }))}
                />
              ))}
            </div>
          </div>
        )}

        {/* 闲置态：最近搜索 + 代码库 */}
        {!searching && !hasResults && (
          <>
            {recent.length > 0 && (
              <section className="mt-10">
                <div className="mb-2.5 flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  <Clock className="size-3.5" />最近搜索
                </div>
                <div className="flex flex-wrap gap-2">
                  {recent.map((r, i) => {
                    const p = prettyId(r.id);
                    return (
                      <button
                        key={i}
                        type="button"
                        onClick={() => handleRecent(r)}
                        className="group inline-flex max-w-full items-center gap-2 rounded-full border border-border bg-card px-3 py-1.5 text-sm transition-colors hover:border-ring hover:bg-muted/60"
                      >
                        <Search className="size-3.5 shrink-0 text-muted-foreground" />
                        <span className="truncate">{r.query}</span>
                        <span className="shrink-0 text-xs text-muted-foreground">· {p.name}</span>
                      </button>
                    );
                  })}
                </div>
              </section>
            )}

            <section className="mt-10">
              <div className="mb-3 flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <h3 className="text-sm font-semibold">代码库</h3>
                  {indexes.length > 0 && (
                    <span className="text-xs text-muted-foreground tabular-nums">{indexes.length}</span>
                  )}
                </div>
                <Button variant="ghost" size="sm" onClick={loadIndexes} disabled={loadingIndexes}>
                  <RefreshCw className={loadingIndexes ? 'animate-spin' : ''} />刷新
                </Button>
              </div>

              {indexError && <p className="text-sm text-destructive">❌ {indexError}</p>}

              {loadingIndexes && indexes.length === 0 && !indexError && (
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {[0, 1, 2].map((i) => <Skeleton key={i} className="h-28 rounded-xl" />)}
                </div>
              )}

              {!loadingIndexes && indexes.length === 0 && !indexError && (
                <div className="rounded-xl border border-dashed border-border px-6 py-12 text-center">
                  <span className="mx-auto flex size-12 items-center justify-center rounded-xl bg-muted text-muted-foreground">
                    <Boxes className="size-6" />
                  </span>
                  <p className="mt-4 font-medium">还没有任何索引</p>
                  <p className="mt-1 text-sm text-muted-foreground">在你的项目里跑 <code className="rounded bg-muted px-1 py-0.5">hce sync</code> 推送上来</p>
                </div>
              )}

              {indexes.length > 0 && (
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {resolved.map((r, i) => (
                    <div key={r.collection} className="animate-rise h-full [&>*]:h-full" style={{ animationDelay: `${Math.min(i, 10) * 45}ms` }}>
                      <CodebaseCard
                        index={r}
                        active={r.named && r.id === activeId}
                        onSelect={() => handleSelectCodebase(r.id)}
                        onDelete={() => setDeleteTarget({ id: r.id, numChunks: r.numChunks })}
                      />
                    </div>
                  ))}
                </div>
              )}
            </section>

            {/* CLI 提示 */}
            <section className="elevate mt-10 overflow-hidden rounded-xl bg-card ring-1 ring-border/70">
              <div className="flex items-center gap-2 border-b border-border/70 px-4 py-3">
                <Terminal className="size-4 text-brand" />
                <h3 className="text-sm font-semibold">如何把代码库索引进来</h3>
              </div>
              <div className="space-y-2.5 px-4 py-4 text-sm text-muted-foreground">
                <p>浏览器读不到本地文件，索引由命令行客户端 <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-brand">hce</code> 推送：</p>
                <pre className="bg-code overflow-x-auto rounded-lg p-3.5 font-mono text-[12.5px] leading-relaxed text-zinc-200 ring-1 ring-white/5">
{`# 在项目根目录
hce sync       # 扫描 + 推送变更
hce search ".." # 自动 sync 后搜索（也会让它出现在上面）`}
                </pre>
                <p>同步或搜索过的代码库会自动出现在「代码库」里，点一下即可在网页上搜，无需再复制 codebase_id。</p>
              </div>
            </section>
          </>
        )}
      </main>

      <footer className="mx-auto max-w-5xl px-4 pb-10 sm:px-6">
        <div className="flex flex-col items-center justify-between gap-3 border-t border-border/60 pt-6 text-xs text-muted-foreground sm:flex-row">
          <span className="flex items-center gap-1.5">
            <span className="size-1.5 rounded-full bg-brand" />
            HCE · 代码语义检索引擎
          </span>
          <span className="font-mono tracking-tight">本地 Milvus 混合检索 · 多 provider embedding</span>
        </div>
      </footer>

      <ConfirmDeleteDialog
        key={deleteTarget?.id ?? 'none'}
        open={deleteTarget !== null}
        onOpenChange={(o) => !o && setDeleteTarget(null)}
        codebaseId={deleteTarget?.id ?? ''}
        numChunks={deleteTarget?.numChunks}
        onConfirm={handleDelete}
      />
    </div>
  );
}

export default App;
