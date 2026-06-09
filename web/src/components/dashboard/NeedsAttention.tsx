import { useMemo, useState } from 'react'
import { useLocation } from 'wouter'
import { AlertTriangle, ChevronRight, Clock, Timer, Layers } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { NotificationItem, NotificationSeverity } from '../../lib/types'
import { SEVERITY_META } from '../notifications/severity'
import { isStandingNotification } from '../../lib/notifications'
import {
  useNotifications,
  useMarkNotificationRead,
} from '../../hooks/useNotifications'

/* Each standing-alert type gets one group: the type is named once in the group
 * header (icon + label + count) so individual rows aren't repetitive. Order here
 * is the tie-breaker when two groups share the same top severity. */
const GROUP_ORDER = ['task.stale_queued', 'task.overrun', 'backlog.high']
const GROUP_META: Record<string, { label: string; Icon: typeof Clock }> = {
  'task.stale_queued': { label: 'Stuck in the queue', Icon: Clock },
  'task.overrun': { label: 'Running long', Icon: Timer },
  'backlog.high': { label: 'Backlog', Icon: Layers },
}

const SEVERITY_RANK: Record<NotificationSeverity, number> = {
  critical: 0,
  warning: 1,
  info: 2,
}

/* How many rows show per group before collapsing behind a "+N more" toggle.
 * Combined with the scroll cap on the card body, this keeps the card usable
 * whether there are 3 alerts or 300. */
const ROWS_PER_GROUP = 4

interface AlertGroup {
  trigger: string
  items: NotificationItem[]
  severity: NotificationSeverity
}

/* ── One compact alert row — severity dot + lead text + duration + chevron ── */

function AlertRow({
  item,
  onActivate,
}: {
  item: NotificationItem
  onActivate: (n: NotificationItem) => void
}) {
  const meta = SEVERITY_META[item.severity] ?? SEVERITY_META.info
  const interactive = Boolean(item.link?.url)
  // Task-level alerts lead with the task name (structured metadata, not the
  // generic title); project-level alerts (backlog) fall back to their message.
  const lead = item.metadata?.taskTitle ?? item.body ?? item.title
  const duration = item.metadata?.duration

  return (
    <div
      className={cn(
        'flex items-center gap-2.5 px-4 py-2 transition-colors',
        interactive && 'cursor-pointer hover:bg-[var(--bg-hover)]',
      )}
      onClick={interactive ? () => onActivate(item) : undefined}
      role={interactive ? 'button' : undefined}
      tabIndex={interactive ? 0 : undefined}
      aria-label={interactive ? `${lead} — open` : undefined}
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
      <span
        className="h-1.5 w-1.5 shrink-0 rounded-full"
        style={{ backgroundColor: meta.color }}
        aria-hidden="true"
      />
      <span className="min-w-0 flex-1 truncate text-[0.8125rem] text-[var(--text-1)]">{lead}</span>
      {duration && (
        <span className="shrink-0 text-[0.75rem] tabular-nums text-[var(--text-3)]">{duration}</span>
      )}
      {interactive && <ChevronRight size={14} className="shrink-0 text-[var(--text-3)]" aria-hidden />}
    </div>
  )
}

/* ── One group: header (type stated once) + capped rows + expand toggle ──── */

function AlertGroupBlock({
  group,
  last,
  onActivate,
}: {
  group: AlertGroup
  last: boolean
  onActivate: (n: NotificationItem) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const meta = GROUP_META[group.trigger] ?? { label: group.trigger, Icon: AlertTriangle }
  const sev = SEVERITY_META[group.severity] ?? SEVERITY_META.info
  const { Icon } = meta
  const visible = expanded ? group.items : group.items.slice(0, ROWS_PER_GROUP)
  const overflow = group.items.length - visible.length

  return (
    <div className={cn(!last && 'border-b border-[var(--border)]')}>
      <div className="flex items-center gap-2 px-4 pt-3 pb-1">
        <Icon size={13} style={{ color: sev.color }} aria-hidden />
        <span className="text-[0.6875rem] font-semibold uppercase tracking-[0.04em] text-[var(--text-2)]">
          {meta.label}
        </span>
        {group.items.length > 1 && (
          <span className="ml-auto rounded-full bg-[var(--bg-active)] px-1.5 text-[0.6875rem] font-medium tabular-nums text-[var(--text-3)]">
            {group.items.length}
          </span>
        )}
      </div>
      {visible.map((item) => (
        <AlertRow key={item.id} item={item} onActivate={onActivate} />
      ))}
      {(overflow > 0 || expanded) && group.items.length > ROWS_PER_GROUP && (
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="w-full px-4 py-1.5 text-left text-[0.75rem] font-medium text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors"
        >
          {expanded ? 'Show less' : `+${overflow} more`}
        </button>
      )}
    </div>
  )
}

/* ── Main exported component ───────────────────────────────────────────────
 *
 * NeedsAttention — a prominent dashboard card surfacing standing alerts only:
 * the condition-based nudges the backend auto-clears once resolved (high
 * backlog, a task stuck in the queue, a task overrunning its threshold). One-off
 * events (task completed / entered review) stay in the bell inbox. Alerts are
 * grouped by type so the card de-duplicates and scales to many alerts; the card
 * hides entirely when nothing needs attention. */
export default function NeedsAttention({ projectId }: { projectId: string | null }) {
  const { data } = useNotifications(projectId)
  const [, navigate] = useLocation()
  const markRead = useMarkNotificationRead()

  const groups = useMemo<AlertGroup[]>(() => {
    const byTrigger = new Map<string, NotificationItem[]>()
    for (const n of data?.notifications ?? []) {
      if (!isStandingNotification(n)) continue
      const arr = byTrigger.get(n.trigger)
      if (arr) arr.push(n)
      else byTrigger.set(n.trigger, [n])
    }

    const built: AlertGroup[] = [...byTrigger.entries()].map(([trigger, items]) => {
      const sorted = [...items].sort((a, b) => {
        const rank = SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity]
        if (rank !== 0) return rank
        return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
      })
      const severity = sorted.reduce<NotificationSeverity>(
        (worst, i) => (SEVERITY_RANK[i.severity] < SEVERITY_RANK[worst] ? i.severity : worst),
        'info',
      )
      return { trigger, items: sorted, severity }
    })

    // Most-severe groups first, then larger groups, then the fixed type order.
    built.sort((a, b) => {
      const rank = SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity]
      if (rank !== 0) return rank
      if (b.items.length !== a.items.length) return b.items.length - a.items.length
      return GROUP_ORDER.indexOf(a.trigger) - GROUP_ORDER.indexOf(b.trigger)
    })
    return built
  }, [data?.notifications])

  const total = useMemo(() => groups.reduce((n, g) => n + g.items.length, 0), [groups])

  if (!projectId) return null
  // Hide entirely when nothing needs attention — an empty attention card is noise.
  if (groups.length === 0) return null

  const handleActivate = (n: NotificationItem) => {
    if (!n.readAt) markRead.mutate({ id: n.id })
    if (n.link?.url) navigate(n.link.url)
  }

  return (
    <section className="dashboard-section @container mb-12" aria-label="Needs attention">
      <div className="dashboard-section-heading mb-2">
        <div className="dashboard-section-heading-main">
          <div className="flex items-center gap-2">
            <AlertTriangle size={14} className="text-[var(--amber)]" />
            <span className="dashboard-section-title">Needs attention</span>
          </div>
        </div>
        <div className="dashboard-section-meta">
          <span className="dashboard-section-counter">
            {total} {total === 1 ? 'alert' : 'alerts'}
          </span>
        </div>
      </div>
      <div className="max-w-[720px] max-h-[28rem] overflow-y-auto overflow-x-hidden rounded-[18px] border border-[color-mix(in_srgb,var(--amber)_35%,var(--border))] bg-[var(--bg-elevated)]">
        {groups.map((g, i) => (
          <AlertGroupBlock
            key={g.trigger}
            group={g}
            last={i === groups.length - 1}
            onActivate={handleActivate}
          />
        ))}
      </div>
    </section>
  )
}
