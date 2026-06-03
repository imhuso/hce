import { cn } from "@/lib/utils"

/** HCE logotype mark — a magnifier with a semantic "node" core, on a brand tile. */
export function BrandMark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "relative flex size-8 items-center justify-center rounded-[10px] bg-gradient-to-br from-brand to-brand-2 text-white shadow-lg shadow-brand/30",
        className
      )}
    >
      <svg viewBox="0 0 24 24" fill="none" className="size-[18px]">
        <circle cx="10" cy="10" r="6.25" stroke="currentColor" strokeWidth="2" />
        <path d="M14.7 14.7 L20 20" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" />
        <circle cx="10" cy="10" r="2.1" fill="currentColor" />
      </svg>
      <span className="pointer-events-none absolute inset-0 rounded-[10px] ring-1 ring-inset ring-white/20" />
    </span>
  )
}
