export function formatCost(usd: number): string {
  if (usd === 0) return '$0.00'
  if (usd < 0.01) return `$${usd.toFixed(4)}`
  return `$${usd.toFixed(2)}`
}

export function formatTokens(n: number): string {
  if (n === 0) return '0'
  if (n < 1000) return String(n)
  if (n < 1_000_000) return `${(n / 1000).toFixed(1)}k`
  return `${(n / 1_000_000).toFixed(2)}M`
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(1)}s`
  const m = Math.floor(s / 60)
  const rem = Math.round(s % 60)
  return rem > 0 ? `${m}m ${rem}s` : `${m}m`
}

export function formatPercent(ratio: number): string {
  if (ratio === 0) return '0%'
  if (ratio >= 1) return '100%'
  return `${(ratio * 100).toFixed(0)}%`
}

export interface RelativeTimeOptions {
  /** Text for missing/unparseable input. Defaults to '' (some callers want '—' or 'unknown'). */
  fallback?: string
  /** Show "Ns ago" under a minute instead of "just now" — for high-churn feeds. Default false. */
  seconds?: boolean
}

export function formatRelative(iso?: string, opts?: RelativeTimeOptions): string {
  const fallback = opts?.fallback ?? ''
  if (!iso) return fallback
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return fallback
  const sec = Math.max(0, Math.round((Date.now() - then) / 1000))
  if (opts?.seconds && sec < 60) return sec < 1 ? 'just now' : `${sec}s ago`
  if (sec < 45) return 'just now'
  const min = Math.round(sec / 60)
  if (min < 60) return `${min}m ago`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr}h ago`
  const day = Math.round(hr / 24)
  if (day < 30) return `${day}d ago`
  const mo = Math.round(day / 30)
  if (mo < 12) return `${mo}mo ago`
  return `${Math.round(mo / 12)}y ago`
}

export interface DateFormatOptions {
  /** Text for missing/unparseable input. Defaults to ''. */
  fallback?: string
  /** 'medium' (default) -> "Jun 6, 2026"; 'numeric' -> locale-default "6/6/2026". */
  style?: 'medium' | 'numeric'
}

export function formatDate(iso?: string, opts?: DateFormatOptions): string {
  const fallback = opts?.fallback ?? ''
  if (!iso) return fallback
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return fallback
  return opts?.style === 'numeric'
    ? date.toLocaleDateString()
    : date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
}
