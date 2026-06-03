import { Database, FolderGit2, Search, Trash2 } from "lucide-react"

import { cn } from "@/lib/utils"
import { prettyId } from "@/lib/codebases"
import { langMeta } from "@/lib/langColors"

export interface ResolvedIndex {
  collection: string
  numChunks: number
  id: string
  named: boolean
  languages?: Record<string, number>
}

interface Props {
  index: ResolvedIndex
  active: boolean
  onSelect: () => void
  onDelete?: () => void
}

function LanguageBar({ languages }: { languages: Record<string, number> }) {
  const entries = Object.entries(languages).sort((a, b) => b[1] - a[1])
  const total = entries.reduce((s, [, v]) => s + v, 0)
  if (total === 0) return null

  return (
    <div className="space-y-2">
      <div className="flex h-1.5 w-full overflow-hidden rounded-full bg-foreground/5">
        {entries.map(([lang, count]) => {
          const m = langMeta(lang)
          return (
            <span
              key={lang}
              style={{ width: `${(count / total) * 100}%`, backgroundColor: m.color }}
              title={`${m.name} ${Math.round((count / total) * 100)}%`}
            />
          )
        })}
      </div>
      <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
        {entries.slice(0, 3).map(([lang, count]) => {
          const m = langMeta(lang)
          return (
            <span key={lang} className="inline-flex items-center gap-1.5">
              <span className="size-2 rounded-full" style={{ backgroundColor: m.color }} />
              <span className="text-foreground/80">{m.name}</span>
              <span className="tabular-nums">{Math.round((count / total) * 100)}%</span>
            </span>
          )
        })}
        {entries.length > 3 && <span className="text-muted-foreground">+{entries.length - 3}</span>}
      </div>
    </div>
  )
}

export function CodebaseCard({ index, active, onSelect, onDelete }: Props) {
  const p = index.named ? prettyId(index.id) : null
  const hasLangs = !!index.languages && Object.keys(index.languages).length > 0

  return (
    <div
      role={index.named ? "button" : undefined}
      tabIndex={index.named ? 0 : undefined}
      onClick={index.named ? onSelect : undefined}
      onKeyDown={index.named ? (e) => (e.key === "Enter" || e.key === " ") && onSelect() : undefined}
      title={
        index.named
          ? "点击：选它来搜索"
          : "本机未知此库的 codebase_id（id 经单向哈希，服务端不存原文）。用 hce 搜索 / 同步一次即可自动命名后在此管理。"
      }
      className={cn(
        "elevate group relative flex flex-col gap-3 rounded-xl bg-card p-4 ring-1 transition-all duration-200",
        index.named
          ? "cursor-pointer ring-border/70 hover:-translate-y-0.5 hover:ring-brand/45 hover:shadow-xl hover:shadow-brand/10"
          : "cursor-help ring-border/50 opacity-65",
        active && "ring-2 ring-brand/70 shadow-lg shadow-brand/15 hover:ring-brand/70"
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <span
          className={cn(
            "flex size-9 shrink-0 items-center justify-center rounded-lg",
            index.named ? "bg-brand/12 text-brand ring-1 ring-brand/20" : "bg-muted text-muted-foreground"
          )}
        >
          {index.named ? <FolderGit2 className="size-4.5" /> : <Database className="size-4.5" />}
        </span>
        {index.named && onDelete && (
          <button
            type="button"
            aria-label="删除此索引"
            title="删除此索引"
            onClick={(e) => {
              e.stopPropagation()
              onDelete()
            }}
            className="rounded-md p-1.5 text-muted-foreground opacity-0 transition hover:bg-destructive/10 hover:text-destructive focus-visible:opacity-100 group-hover:opacity-100"
          >
            <Trash2 className="size-4" />
          </button>
        )}
      </div>

      <div className="min-w-0">
        {p ? (
          <div className="flex items-baseline gap-1.5">
            <span className="truncate font-semibold">{p.name}</span>
            {p.hash && <span className="shrink-0 font-mono text-xs text-muted-foreground">{p.hash}</span>}
          </div>
        ) : (
          <code className="block truncate text-sm text-muted-foreground" title={index.collection}>
            {index.collection}
          </code>
        )}
        <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
          <span className="tabular-nums">{index.numChunks.toLocaleString()} chunk</span>
          {!index.named && (
            <span className="rounded border border-border px-1 py-px text-[10px] uppercase tracking-wide">匿名</span>
          )}
        </div>
      </div>

      {hasLangs ? (
        <LanguageBar languages={index.languages!} />
      ) : index.named ? (
        <div className="flex items-center gap-1 text-xs font-medium text-brand opacity-0 transition-opacity group-hover:opacity-100">
          <Search className="size-3.5" />
          选它来搜索
        </div>
      ) : null}
    </div>
  )
}
