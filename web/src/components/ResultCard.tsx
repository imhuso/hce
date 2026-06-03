import { useMemo, useState } from "react"
import { Check, ChevronDown, Copy, WrapText } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { highlightLines, TOK_CLASS } from "@/lib/highlight"
import type { SearchResult } from "@/api"

const CLAMP_LINES = 20

function scoreTone(score: number): string {
  if (score >= 0.8) return "text-emerald-600 dark:text-emerald-400"
  if (score >= 0.6) return "text-amber-600 dark:text-amber-400"
  return "text-muted-foreground"
}

interface Props {
  r: SearchResult
  rank: number
  collapsed: boolean
  onToggleCollapse: () => void
}

export function ResultCard({ r, rank, collapsed, onToggleCollapse }: Props) {
  const [copied, setCopied] = useState(false)
  const [copiedPath, setCopiedPath] = useState(false)
  const [wrap, setWrap] = useState(false)
  const [expanded, setExpanded] = useState(false)

  const lines = useMemo(() => highlightLines(r.content, r.language), [r.content, r.language])
  const pct = Math.round(r.score * 100)
  const long = lines.length > CLAMP_LINES + 4
  const visible = !long || expanded ? lines : lines.slice(0, CLAMP_LINES)

  const slash = r.relative_path.lastIndexOf("/")
  const dir = slash >= 0 ? r.relative_path.slice(0, slash + 1) : ""
  const file = slash >= 0 ? r.relative_path.slice(slash + 1) : r.relative_path

  const flash = (set: (v: boolean) => void) => {
    set(true)
    setTimeout(() => set(false), 1500)
  }
  const copyCode = async (e: React.MouseEvent) => {
    e.stopPropagation()
    try {
      await navigator.clipboard.writeText(r.content)
      flash(setCopied)
    } catch { /* ignore */ }
  }
  const copyPath = async (e: React.MouseEvent) => {
    e.stopPropagation()
    try {
      await navigator.clipboard.writeText(`${r.relative_path}:${r.start_line}`)
      flash(setCopiedPath)
    } catch { /* ignore */ }
  }

  return (
    <div className="overflow-hidden rounded-xl bg-card ring-1 ring-foreground/10 transition-shadow hover:ring-foreground/20">
      {/* 头部：整行可点击收起 / 展开 */}
      <div
        role="button"
        tabIndex={0}
        aria-expanded={!collapsed}
        onClick={onToggleCollapse}
        onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && (e.preventDefault(), onToggleCollapse())}
        className="flex cursor-pointer select-none flex-wrap items-center gap-x-2.5 gap-y-1.5 border-b bg-muted/40 px-2.5 py-2.5 transition-colors hover:bg-muted/70"
        title={collapsed ? "展开" : "收起"}
      >
        <ChevronDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform", collapsed && "-rotate-90")} />
        <span className="flex size-5 shrink-0 items-center justify-center rounded-md bg-foreground/5 text-[11px] font-semibold text-muted-foreground tabular-nums">
          {rank}
        </span>
        <button
          type="button"
          onClick={copyPath}
          title={copiedPath ? "已复制" : "点击复制 路径:行号"}
          className="flex min-w-0 flex-1 items-baseline gap-0.5 text-left"
        >
          {dir && <span className="truncate text-sm text-muted-foreground">{dir}</span>}
          <span className={cn("shrink-0 text-sm font-semibold", copiedPath ? "text-emerald-500" : "text-blue-600 dark:text-blue-400")}>
            {file}
          </span>
          {copiedPath && <Check className="size-3 shrink-0 text-emerald-500" />}
        </button>
        <span className="text-xs text-muted-foreground tabular-nums">L{r.start_line}–{r.end_line}</span>
        <Badge variant="outline" className="font-mono">{r.language || "text"}</Badge>
        <span className={cn("flex items-center gap-1.5", scoreTone(r.score))} title={`相似度 ${pct}%`}>
          <span className="hidden h-1.5 w-14 overflow-hidden rounded-full bg-foreground/10 sm:block">
            <span className="block h-full rounded-full bg-current" style={{ width: `${pct}%` }} />
          </span>
          <span className="text-xs font-semibold tabular-nums">{pct}%</span>
        </span>
        <div className="flex items-center gap-0.5" onClick={(e) => e.stopPropagation()}>
          <Button variant="ghost" size="icon-sm" aria-label="切换自动换行" title="自动换行"
            onClick={() => setWrap((w) => !w)} className={cn(wrap && "text-blue-600 dark:text-blue-400")}>
            <WrapText />
          </Button>
          <Button variant="ghost" size="icon-sm" aria-label="复制代码片段" title="复制代码片段" onClick={copyCode}>
            {copied ? <Check className="text-emerald-500" /> : <Copy />}
          </Button>
        </div>
      </div>

      {!collapsed && (
        <div className="relative">
          <pre className="overflow-x-auto bg-zinc-950 font-mono text-[13px] leading-relaxed text-zinc-200">
            <code className="block py-3">
              {visible.map((toks, j) => (
                <div key={j} className="flex hover:bg-white/[0.04]">
                  <span className="w-12 shrink-0 select-none pr-4 text-right text-zinc-600 tabular-nums">
                    {r.start_line + j}
                  </span>
                  <span className={cn("min-w-0 pr-4", wrap ? "whitespace-pre-wrap break-words" : "whitespace-pre")}>
                    {toks.length === 0 ? " " : toks.map((tok, k) => (
                      <span key={k} className={TOK_CLASS[tok.c]}>{tok.t}</span>
                    ))}
                  </span>
                </div>
              ))}
            </code>
          </pre>
          {long && (
            <>
              {!expanded && (
                <div className="pointer-events-none absolute inset-x-0 bottom-9 h-14 bg-gradient-to-t from-zinc-950 to-transparent" />
              )}
              <button
                type="button"
                onClick={() => setExpanded((e) => !e)}
                className="block w-full border-t border-white/5 bg-zinc-950 py-2 text-center text-xs font-medium text-zinc-400 transition-colors hover:text-zinc-100"
              >
                {expanded ? "收起代码" : `展开剩余 ${lines.length - CLAMP_LINES} 行`}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  )
}
