import { formatRelative, type RelativeTimeOptions } from '@/lib/format'

/**
 * RelativeTime — "2h ago" that tells you WHEN on hover. One shared affordance
 * so every relative timestamp in the app exposes the absolute date/time the
 * same way (native title tooltip; works on desktop, harmless on touch).
 */
export default function RelativeTime({
  iso,
  options,
  className,
}: {
  iso?: string
  options?: RelativeTimeOptions
  className?: string
}) {
  const absolute = (() => {
    if (!iso) return undefined
    const d = new Date(iso)
    return Number.isNaN(d.getTime()) ? undefined : d.toLocaleString()
  })()
  return (
    <span title={absolute} className={className}>
      {formatRelative(iso, options)}
    </span>
  )
}
