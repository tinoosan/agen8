import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

/**
 * StatTile — an aggregate "tile" for the list pages (Missions / Tasks /
 * Decisions): an uppercase label with a small semantic icon, a big tabular
 * number, and an optional sub-line. When given onClick it acts as a filter
 * shortcut (and shows an accent border when active); otherwise it's a
 * non-interactive readout. Shared so the three pages read identically.
 */
export default function StatTile({
  label,
  value,
  sub,
  tone,
  icon: Icon,
  onClick,
  active,
}: {
  label: string
  value: string | number
  sub?: string
  /** Semantic colour for the icon and the value. */
  tone: string
  icon: LucideIcon
  onClick?: () => void
  active?: boolean
}) {
  const interactive = Boolean(onClick)
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={interactive ? Boolean(active) : undefined}
      disabled={!interactive}
      className={cn(
        'flex flex-col gap-1.5 rounded-[14px] border bg-[var(--bg-elevated)] px-4 py-3 text-left',
        interactive ? 'cursor-pointer transition-colors hover:bg-[var(--bg-hover)]' : 'cursor-default',
        active ? 'border-[var(--accent)]' : 'border-[var(--border)]',
      )}
    >
      <div className="flex items-center gap-1.5">
        <Icon size={13} style={{ color: tone }} aria-hidden />
        <span className="text-[0.6875rem] font-semibold uppercase tracking-[0.05em] text-[var(--text-3)]">
          {label}
        </span>
      </div>
      <span className="text-[1.5rem] font-semibold leading-none tabular-nums" style={{ color: tone }}>
        {value}
      </span>
      {sub && <span className="text-[0.75rem] text-[var(--text-3)]">{sub}</span>}
    </button>
  )
}
