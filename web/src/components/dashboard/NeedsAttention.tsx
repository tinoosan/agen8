import { useMemo } from 'react'
import { useLocation } from 'wouter'
import { AlertTriangle, ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'
import { formatRelative } from '../../lib/format'
import type { NotificationItem, NotificationSeverity } from '../../lib/types'
import { SEVERITY_META } from '../notifications/severity'
import { isStandingNotification } from '../../lib/notifications'
import {
  useNotifications,
  useMarkNotificationRead,
} from '../../hooks/useNotifications'

/* Critical first, then warning, then info — so the most pressing alert leads. */
const SEVERITY_RANK: Record<NotificationSeverity, number> = {
  critical: 0,
  warning: 1,
  info: 2,
}

/* ── One alert row — severity icon + title/body + time + disclosure ────────── */

function AttentionRow({
  item,
  first,
  onActivate,
}: {
  item: NotificationItem
  first: boolean
  onActivate: (n: NotificationItem) => void
}) {
  const meta = SEVERITY_META[item.severity] ?? SEVERITY_META.info
  const { Icon } = meta
  const interactive = Boolean(item.link)

  return (
    <div>
      {/* Hairline inset to start under the icon, the way grouped lists read. */}
      {!first && <div className="ml-[44px] h-px bg-[var(--border)]" />}
      <div
        className={cn(
          'flex items-start gap-3 px-4 py-3 transition-colors',
          interactive && 'cursor-pointer hover:bg-[var(--bg-hover)]',
        )}
        onClick={interactive ? () => onActivate(item) : undefined}
        role={interactive ? 'button' : undefined}
        tabIndex={interactive ? 0 : undefined}
        aria-label={interactive ? `${item.title} — open` : undefined}
        onKeyDown={
          interactive
            ? (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  onActivate(item)
                }
              }
            : undefined
        }
      >
        <Icon
          size={16}
          className="mt-0.5 shrink-0"
          style={{ color: meta.color }}
          aria-hidden="true"
        />
        {/* Stacks tight by default; on a wider card the timestamp moves to the
         * right of the text instead of below it (@container, not viewport). */}
        <div className="min-w-0 flex-1 @min-[440px]:flex @min-[440px]:items-start @min-[440px]:gap-3">
          <div className="min-w-0 flex-1">
            <div className="text-[0.875rem] font-semibold tracking-[-0.01em] text-[var(--text-1)] leading-snug">
              {item.title}
            </div>
            {item.body && (
              <p className="mt-0.5 text-[0.8125rem] text-[var(--text-3)] leading-snug">
                {item.body}
              </p>
            )}
            <span className="mt-1 block text-[0.6875rem] tabular-nums text-[var(--text-3)] @min-[440px]:hidden">
              {formatRelative(item.createdAt)}
            </span>
          </div>
          <span className="hidden shrink-0 text-[0.6875rem] tabular-nums text-[var(--text-3)] @min-[440px]:mt-0.5 @min-[440px]:block">
            {formatRelative(item.createdAt)}
          </span>
        </div>
        {interactive && (
          <ChevronRight size={16} className="mt-0.5 shrink-0 text-[var(--text-3)]" aria-hidden />
        )}
      </div>
    </div>
  )
}

/* ── Main exported component ───────────────────────────────────────────────
 *
 * NeedsAttention — a prominent dashboard card surfacing standing alerts only:
 * the condition-based nudges the backend auto-clears once resolved (high
 * backlog, a task stuck in the queue, a task overrunning its threshold). One-off
 * events (task completed / entered review) stay in the bell inbox and are
 * filtered out here. The card hides entirely when nothing needs attention, so it
 * never adds noise to a calm dashboard. */
export default function NeedsAttention({ projectId }: { projectId: string | null }) {
  const { data } = useNotifications(projectId)
  const [, navigate] = useLocation()
  const markRead = useMarkNotificationRead()

  const alerts = useMemo(() => {
    const list = (data?.notifications ?? []).filter(isStandingNotification)
    return [...list].sort((a, b) => {
      const rank = SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity]
      if (rank !== 0) return rank
      // Newer first within the same severity.
      return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
    })
  }, [data?.notifications])

  if (!projectId) return null
  // Hide entirely when nothing needs attention — an attention card with nothing
  // in it is just noise.
  if (alerts.length === 0) return null

  const handleActivate = (n: NotificationItem) => {
    if (!n.readAt) markRead.mutate({ id: n.id })
    if (n.link?.url) navigate(n.link.url)
  }

  return (
    <section className="dashboard-section @container mb-8" aria-label="Needs attention">
      <div className="dashboard-section-heading mb-2">
        <div className="dashboard-section-heading-main">
          <div className="flex items-center gap-2">
            <AlertTriangle size={14} className="text-[var(--amber)]" />
            <span className="dashboard-section-title">Needs attention</span>
          </div>
          <p className="dashboard-section-caption">Standing alerts that clear once resolved.</p>
        </div>
        <div className="dashboard-section-meta">
          <span className="dashboard-section-counter">
            {alerts.length} {alerts.length === 1 ? 'alert' : 'alerts'}
          </span>
        </div>
      </div>
      <div className="max-w-[720px] overflow-hidden rounded-[18px] border border-[color-mix(in_srgb,var(--amber)_35%,var(--border))] bg-[var(--bg-elevated)]">
        {alerts.map((n, i) => (
          <AttentionRow key={n.id} item={n} first={i === 0} onActivate={handleActivate} />
        ))}
      </div>
    </section>
  )
}
