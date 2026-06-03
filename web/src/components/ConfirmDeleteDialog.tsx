import { useState } from "react"
import { Loader2, ShieldAlert } from "lucide-react"

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  codebaseId: string
  numChunks?: number
  /** Perform the deletion. Resolve to an error string on failure, or undefined on success. */
  onConfirm: () => Promise<string | undefined>
}

/**
 * Destructive delete guarded against mis-clicks: the user must type the exact
 * codebase_id to arm the button. Dropping a collection is irreversible (re-index
 * means a full re-embed), so the friction here is intentional.
 *
 * The parent remounts this via `key={codebaseId}`, so ephemeral state starts
 * fresh for every target without a reset effect.
 */
export function ConfirmDeleteDialog({ open, onOpenChange, codebaseId, numChunks, onConfirm }: Props) {
  const [typed, setTyped] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  const armed = typed.trim() === codebaseId && !busy

  const handleConfirm = async () => {
    if (!armed) return
    setBusy(true)
    setError("")
    const err = await onConfirm()
    if (err) {
      setError(err)
      setBusy(false)
    } else {
      onOpenChange(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !busy && onOpenChange(o)}>
      <DialogContent showClose={!busy}>
        <DialogHeader>
          <div className="flex size-9 items-center justify-center rounded-full bg-destructive/10 text-destructive">
            <ShieldAlert className="size-5" />
          </div>
          <DialogTitle>删除索引</DialogTitle>
          <DialogDescription>
            将从服务端永久删除这个 codebase 的全部向量索引
            {typeof numChunks === "number" && (
              <>
                （
                <span className="font-semibold text-foreground tabular-nums">{numChunks}</span>
                {" 个 chunk）"}
              </>
            )}
            。此操作<span className="font-semibold text-destructive">不可恢复</span>，重新索引需要再跑一遍 <code className="rounded bg-muted px-1 py-0.5 text-xs">hce-cli sync</code> 并重新 embedding。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">
            请输入下面的 codebase_id 以确认：
          </p>
          <code className="block truncate rounded-md bg-muted px-3 py-2 font-mono text-sm" title={codebaseId}>
            {codebaseId}
          </code>
          <Input
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleConfirm()}
            placeholder="在此粘贴 / 输入上面的 id"
            className="font-mono"
            spellCheck={false}
            autoComplete="off"
            disabled={busy}
            autoFocus
          />
          {error && <p className="text-sm text-destructive">❌ {error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
            取消
          </Button>
          <Button variant="destructive" onClick={handleConfirm} disabled={!armed}>
            {busy ? (
              <>
                <Loader2 className="animate-spin" />
                删除中…
              </>
            ) : (
              "确认删除"
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
