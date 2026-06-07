import { formatSuccessRate } from '../../lib/metrics'

/* Shared leaderboard primitives, used by both the Metrics page leaderboards and
 * the per-member performance cells in the Members roster. Keeping them in one
 * place means a model row, a harness row, and a member row all read with the
 * exact same visual language (same bar, same success palette). */

/** A thin proportional bar. `value` is a 0–100 percentage (already normalized
 * by the caller against whatever max it cares about). */
export function ShareBar({ value }: { value: number }) {
  const width = Math.max(0, Math.min(100, value))
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full bg-[var(--bg-elevated)]">
      <span
        className="block h-full rounded-full bg-[var(--accent)]"
        style={{ width: `${width}%` }}
      />
    </div>
  )
}

/** Success rate as a tinted pill. Green ≥90%, amber 70–90%, red below —
 * mirrors the status palette. Renders "—" when there's no rate yet. */
export function SuccessPill({ rate }: { rate: number | null }) {
  if (rate === null) return <span className="text-[var(--text-3)]">—</span>
  const color = rate >= 0.9 ? 'var(--green)' : rate >= 0.7 ? 'var(--amber)' : 'var(--red)'
  return (
    <span
      className="inline-block rounded-[6px] px-1.5 py-px text-[0.6875rem] font-semibold tabular-nums"
      style={{ color, background: `color-mix(in srgb, ${color} 16%, transparent)` }}
    >
      {formatSuccessRate(rate)}
    </span>
  )
}
