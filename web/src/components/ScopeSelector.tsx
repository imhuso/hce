import { useMemo, useRef, useState } from "react"
import { Popover as PopoverPrimitive } from "radix-ui"
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
 * The codebase "scope" picker in the hero search line. The menu is rendered in a
 * portal (Radix Popover) so it escapes every ancestor's overflow clip and
 * stacking context — it can never be occluded by the search bar or cards below.
 */
export function ScopeSelector({ value, onChange, options }: Props) {
  const [open, setOpen] = useState(false)
  const [filter, setFilter] = useState("")
  const inputRef = useRef<HTMLInputElement>(null)

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
    <PopoverPrimitive.Root
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (!o) setFilter("")
      }}
    >
      <PopoverPrimitive.Trigger asChild>
        <button
          type="button"
          className={cn(
            "elevate group inline-flex h-9 max-w-[60vw] items-center gap-2 rounded-lg border border-border/80 bg-card px-3 text-sm font-medium ring-1 ring-transparent transition-colors hover:border-brand/50 hover:bg-muted/50",
            open && "border-brand/60 ring-brand/25"
          )}
        >
          <Boxes className="size-4 shrink-0 text-brand" />
          {pretty ? (
            <span className="min-w-0 truncate">
              <span className="font-semibold">{pretty.name}</span>
              {pretty.hash && <span className="ml-1 font-mono text-muted-foreground">· {pretty.hash}</span>}
            </span>
          ) : (
            <span className="text-muted-foreground">选择代码库</span>
          )}
          <ChevronsUpDown className="size-3.5 shrink-0 text-muted-foreground" />
        </button>
      </PopoverPrimitive.Trigger>

      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          align="start"
          sideOffset={8}
          collisionPadding={12}
          onOpenAutoFocus={(e) => {
            e.preventDefault()
            inputRef.current?.focus()
          }}
          className="z-[60] w-80 max-w-[calc(100vw-1.5rem)] overflow-hidden rounded-xl bg-popover p-1 text-popover-foreground shadow-2xl ring-1 ring-border/80 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
        >
          <input
            ref={inputRef}
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && canUseTyped) pick(typed)
            }}
            placeholder="筛选，或粘贴一个 codebase_id…"
            spellCheck={false}
            autoComplete="off"
            className="mb-1 h-8 w-full rounded-lg border border-input bg-transparent px-2.5 font-mono text-sm outline-none placeholder:font-sans placeholder:text-muted-foreground focus-visible:border-brand/60 focus-visible:ring-3 focus-visible:ring-brand/20"
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
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  )
}
