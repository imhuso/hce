import { useState, useEffect, useCallback } from 'react';
import { searchCode, listIndexes, healthCheck } from '@/api';
import type { SearchResult, IndexInfo } from '@/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

function App() {
  const [online, setOnline] = useState(false);

  // 搜索
  const [codebaseId, setCodebaseId] = useState('');
  const [query, setQuery] = useState('');
  const [topK, setTopK] = useState(5);
  const [results, setResults] = useState<SearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchMsg, setSearchMsg] = useState('');

  // 索引列表（管理）
  const [indexes, setIndexes] = useState<IndexInfo[]>([]);
  const [manageMsg, setManageMsg] = useState('');

  // 健康检查
  useEffect(() => {
    const check = async () => {
      try { await healthCheck(); setOnline(true); } catch { setOnline(false); }
    };
    check();
    const t = setInterval(check, 10000);
    return () => clearInterval(t);
  }, []);

  // 加载已索引集合
  const loadIndexes = useCallback(async () => {
    try {
      const res = await listIndexes();
      if (res.code === 0 && res.data) {
        setIndexes(res.data);
        setManageMsg(res.data.length === 0 ? '尚未有任何索引集合（用本机 hce-cli sync 推送）' : '');
      } else {
        setManageMsg(`❌ ${res.message}`);
      }
    } catch (e) { setManageMsg(`❌ ${e}`); }
  }, []);

  useEffect(() => { if (online) loadIndexes(); }, [online, loadIndexes]);

  const handleSearch = async () => {
    if (!codebaseId || !query) return;
    setSearching(true); setSearchMsg(''); setResults([]);
    try {
      const res = await searchCode(codebaseId, query, topK);
      if (res.code === 0 && res.data) {
        setResults(res.data);
        setSearchMsg(res.data.length > 0 ? `找到 ${res.data.length} 条结果` : '未找到相关代码');
      } else { setSearchMsg(`❌ ${res.message}`); }
    } catch (e) { setSearchMsg(`❌ ${e}`); }
    finally { setSearching(false); }
  };

  return (
    <div className="min-h-screen bg-background">
      {/* 顶部栏 */}
      <header className="sticky top-0 z-50 border-b bg-background/80 backdrop-blur-sm">
        <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-6">
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-bold tracking-tight bg-gradient-to-r from-blue-500 to-violet-500 bg-clip-text text-transparent">HCE</h1>
            <span className="text-sm text-muted-foreground">代码语义索引服务</span>
          </div>
          <Badge variant={online ? 'default' : 'destructive'} className="gap-1.5">
            <span className={`h-2 w-2 rounded-full ${online ? 'bg-green-400 animate-pulse' : 'bg-red-400'}`} />
            {online ? '在线' : '离线'}
          </Badge>
        </div>
      </header>

      {/* 主内容 */}
      <main className="mx-auto max-w-6xl px-6 py-8">
        <Tabs defaultValue="search">
          <TabsList className="mb-6">
            <TabsTrigger value="search">🔍 语义搜索</TabsTrigger>
            <TabsTrigger value="manage">📦 索引管理</TabsTrigger>
          </TabsList>

          {/* 搜索页 */}
          <TabsContent value="search">
            <Card>
              <CardHeader><CardTitle className="text-base">语义搜索</CardTitle></CardHeader>
              <CardContent className="space-y-4">
                <Input
                  placeholder="codebase_id（在你的项目里跑 hce-cli status 查看）"
                  value={codebaseId}
                  onChange={e => setCodebaseId(e.target.value)}
                />
                <div className="flex gap-3">
                  <Input
                    className="flex-1"
                    placeholder="用自然语言描述你要找的代码..."
                    value={query}
                    onChange={e => setQuery(e.target.value)}
                    onKeyDown={e => e.key === 'Enter' && handleSearch()}
                  />
                  <select
                    className="h-9 rounded-md border bg-background px-3 text-sm"
                    value={topK}
                    onChange={e => setTopK(Number(e.target.value))}
                  >
                    {[3, 5, 10, 20].map(n => <option key={n} value={n}>Top {n}</option>)}
                  </select>
                  <Button onClick={handleSearch} disabled={searching || !codebaseId || !query}>
                    {searching ? '搜索中...' : '搜索'}
                  </Button>
                </div>
                {searchMsg && (
                  <p className={`text-sm ${results.length > 0 ? 'text-green-500' : 'text-muted-foreground'}`}>{searchMsg}</p>
                )}
              </CardContent>
            </Card>

            {results.length > 0 && (
              <div className="mt-6 space-y-4">
                {results.map((r, i) => (
                  <Card key={i} className="overflow-hidden">
                    <div className="flex items-center justify-between border-b px-4 py-2.5 bg-muted/30">
                      <code className="text-sm font-semibold text-blue-500">{r.relative_path}</code>
                      <div className="flex items-center gap-3 text-xs text-muted-foreground">
                        <span>L{r.start_line}-{r.end_line}</span>
                        <Badge variant="outline">{r.language}</Badge>
                        <Badge variant="secondary" className="text-green-600">{(r.score * 100).toFixed(1)}%</Badge>
                      </div>
                    </div>
                    <pre className="overflow-x-auto p-4 text-[13px] leading-relaxed font-mono bg-zinc-950 text-zinc-200">
                      {r.content.split('\n').map((line, j) => (
                        <div key={j} className="flex">
                          <span className="w-12 shrink-0 text-right pr-4 text-zinc-600 select-none">{r.start_line + j}</span>
                          <span>{line}</span>
                        </div>
                      ))}
                    </pre>
                  </Card>
                ))}
              </div>
            )}

            {!searching && results.length === 0 && !searchMsg && (
              <div className="mt-16 text-center text-muted-foreground">
                <p className="text-4xl mb-4">🔍</p>
                <p>填入 codebase_id 与查询，回车搜索<br />（codebase 必须先用本机 hce-cli sync 推送上来）</p>
              </div>
            )}
          </TabsContent>

          {/* 索引管理页 */}
          <TabsContent value="manage" className="space-y-6">
            <Card>
              <CardHeader>
                <div className="flex items-center justify-between">
                  <CardTitle className="text-base">服务端已索引集合</CardTitle>
                  <Button variant="ghost" size="sm" onClick={loadIndexes}>刷新</Button>
                </div>
              </CardHeader>
              <CardContent>
                {manageMsg && (
                  <p className={`text-sm ${manageMsg.startsWith('✅') ? 'text-green-500' : manageMsg.startsWith('❌') ? 'text-red-500' : 'text-muted-foreground'} mb-3`}>{manageMsg}</p>
                )}
                {indexes.length === 0 ? (
                  <p className="text-sm text-muted-foreground text-center py-8">暂无</p>
                ) : (
                  <div className="divide-y">
                    {indexes.map((idx) => (
                      <div key={idx.collection} className="flex items-center justify-between py-3 group">
                        <div className="min-w-0 flex-1">
                          <code className="text-sm font-mono text-blue-500 truncate block">{idx.collection}</code>
                          <p className="mt-1 text-xs text-muted-foreground">{idx.num_chunks} 个 chunk</p>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader><CardTitle className="text-base">如何索引代码库</CardTitle></CardHeader>
              <CardContent className="space-y-3 text-sm text-muted-foreground">
                <p>浏览器读不到本地文件，索引必须由命令行客户端 <code className="text-blue-500">hce-cli</code> 推送到服务端：</p>
                <pre className="bg-muted/40 rounded-md p-3 text-[12px] font-mono leading-relaxed text-foreground">
{`# 在你的项目根
hce-cli sync              # 扫描 + 推送变更（首次会自动创建 .hce/）
hce-cli status            # 查看 codebase_id
hce-cli search "..."      # 自动 sync 后再搜
hce-cli clear             # 清除当前 codebase 的索引`}
                </pre>
                <p>每次 <code>search</code> 会先自动 sync，无变更时零 EMB 调用。把上方搜索框里的 <code>codebase_id</code> 填成 <code>hce-cli status</code> 显示的那个 id 即可。</p>
                <p>注意把 <code>.hce/</code> 加入 <code>.gitignore</code>。</p>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </main>
    </div>
  );
}

export default App;
