import { useEffect, useMemo, useRef, useState } from "react"
import { Check, ChevronsUpDown, Boxes, Plus } from "lucide-react"

import { cn } from "@/lib/utils"
import { prettyId } from "@/lib/codebases"

export interface ScopeOption {
  id: string
  numChunks: number
}

interface Props {
  value: string
  onChange: (id: string) => void
  options: ScopeOption[]
}

/**
 * The codebase "scope" picker that sits inside the hero search line. Lists the
 * named, searchable codebases from the server and lets the user paste an id that
 * isn't listed yet — so they never have to copy an id into a bare text box.
 */
export function ScopeSelector({ value, onChange, options }: Props) {
  const [open, setOpen] = useState(false)
  const [filter, setFilter] = useState("")
  const rootRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener("mousedown", onDown)
    return () => document.removeEventListener("mousedown", onDown)
  }, [open])

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase()
    const list = q ? options.filter((o) => o.id.toLowerCase().includes(q)) : options
    return [...list].sort((a, b) => b.numChunks - a.numChunks)
  }, [filter, options])

  const typed = filter.trim()
  const canUseTyped = typed.length > 0 && !options.some((o) => o.id === typed)

  const pick = (id: string) => {
    onChange(id)
    setOpen(false)
    setFilter("")
  }

  const pretty = value ? prettyId(value) : null

  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="listbox"
        aria-expanded={open}
        className={cn(
          "elevate group inline-flex h-9 max-w-[60vw] items-center gap-2 rounded-lg border border-border/80 bg-card px-3 text-sm font-medium ring-1 ring-transparent transition-colors hover:border-brand/50 hover:bg-muted/50",
          open && "border-brand/60 ring-brand/25"
        )}
      >
        <Boxes className="size-4 shrink-0 text-brand" />
        {pretty ? (
          <span className="min-w-0 truncate">
            <span className="font-semibold">{pretty.name}</span>
            {pretty.hash && <span className="ml-1 text-muted-foreground">· {pretty.hash}</span>}
          </span>
        ) : (
          <span className="text-muted-foreground">选择代码库</span>
        )}
        <ChevronsUpDown className="size-3.5 shrink-0 text-muted-foreground" />
      </button>

      {open && (
        <div className="absolute left-0 z-40 mt-2 w-80 max-w-[80vw] overflow-hidden rounded-xl bg-popover p-1 text-popover-foreground shadow-xl ring-1 ring-foreground/10 animate-in fade-in-0 zoom-in-95">
          <input
            autoFocus
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && canUseTyped) pick(typed)
              else if (e.key === "Escape") setOpen(false)
            }}
            placeholder="筛选，或粘贴一个 codebase_id…"
            spellCheck={false}
            autoComplete="off"
            className="mb-1 h-8 w-full rounded-lg border border-input bg-transparent px-2.5 font-mono text-sm outline-none placeholder:font-sans placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/40"
          />
          <div className="max-h-64 overflow-auto">
            {filtered.length === 0 && !canUseTyped && (
              <p className="px-2 py-3 text-center text-xs text-muted-foreground">没有匹配的代码库</p>
            )}
            {filtered.map((o) => {
              const p = prettyId(o.id)
              const active = o.id === value
              return (
                <button
                  key={o.id}
                  type="button"
                  onClick={() => pick(o.id)}
                  className={cn(
                    "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted",
                    active && "bg-muted"
                  )}
                >
                  <Check className={cn("size-4 shrink-0", active ? "text-brand" : "text-transparent")} />
                  <span className="min-w-0 flex-1 truncate text-sm">
                    <span className="font-medium">{p.name}</span>
                    {p.hash && <span className="ml-1 font-mono text-xs text-muted-foreground">{p.hash}</span>}
                  </span>
                  <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                    {o.numChunks.toLocaleString()}
                  </span>
                </button>
              )
            })}
            {canUseTyped && (
              <button
                type="button"
                onClick={() => pick(typed)}
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted"
              >
                <Plus className="size-4 shrink-0 text-emerald-500" />
                <span className="truncate text-sm">
                  使用 <span className="font-mono">{typed}</span>
                </span>
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
