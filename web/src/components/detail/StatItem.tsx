import type { ReactNode } from 'react'

/* ── Stat row in a detail-page metadata grid ── */

export function StatItem({ label, value, icon }: { label: string; value: ReactNode; icon?: ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <span style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-3)' }}>
        {label}
      </span>
      <span className="flex items-center gap-1.5 text-[var(--text-1)]" style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}>
        {icon}
        {value}
      </span>
    </div>
  )
}
