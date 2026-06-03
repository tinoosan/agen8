import { useLocation } from 'wouter'
import { CheckCheck, ExternalLink, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { useNavigation } from '../../lib/routing'
import { useNotifications, useMarkRead, useMarkAllRead, useDismissNotification } from '../../hooks/useNotifications'
import type { NotificationItem, NotificationSeverity } from '../../lib/types'

interface NotificationDropdownProps {
  userId: string | null
  onClose: () => void
}

function formatTriggerLabel(trigger: string): string | null {
  switch (trigger) {
    case 'schedule_expiring':
      return 'Schedule expiring'
    case 'schedule_expired':
      return 'Schedule expired'
    case 'schedule_completed':
      return 'Schedule completed'
    default:
      return null
  }
}

const severityDotClass: Record<NotificationSeverity, string> = {
  critical: 'bg-[var(--danger,#ef4444)]',
  warning: 'bg-[var(--warning,#f59e0b)]',
  info: 'bg-[var(--info,#3b82f6)]',
}

/** Severity indicator dot with appropriate colour. */
function SeverityDot({ severity }: { severity: NotificationSeverity }) {
  return (
    <span
      className={cn(
        'inline-block w-2 h-2 rounded-full shrink-0 mt-1',
        severityDotClass[severity] ?? severityDotClass.info,
      )}
    />
  )
}

/** Human-readable relative time (e.g. "2 minutes ago"). */
function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

/**
 * Notification dropdown panel. Shows recent notifications with severity icon,
 * title, body snippet, relative timestamp, and deep-link.
 * Supports mark-as-read, mark-all-read, and dismiss.
 */
export default function NotificationDropdown({ userId, onClose }: NotificationDropdownProps) {
  const [, navigate] = useLocation()
  const { projectId } = useNavigation()
  const { data: notifications = [], isLoading } = useNotifications({ userId, limit: 20 })
  const markRead = useMarkRead()
  const markAllRead = useMarkAllRead()
  const dismissNotification = useDismissNotification()

  function handleClick(n: NotificationItem) {
    if (!n.readAt) {
      markRead.mutate(n.id)
    }
    if (n.link?.url) {
      navigate(n.link.url)
      onClose()
    }
  }

  function handleViewAll() {
    if (projectId) {
      navigate(`/project/${encodeURIComponent(projectId)}/notifications`)
    }
    onClose()
  }

  return (
    <div className="w-[380px] max-h-[480px] bg-[var(--bg-surface)] border border-[var(--border)] rounded-lg shadow-lg flex flex-col overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border)]">
        <span className="font-semibold text-sm">Notifications</span>
        <div className="flex gap-2 items-center">
          {notifications.length > 0 && userId && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => markAllRead.mutate(userId)}
              title="Mark all as read"
              className="text-[var(--text-secondary)] text-xs h-auto px-2 py-1"
            >
              <CheckCheck size={14} className="mr-1" />
              Mark all read
            </Button>
          )}
        </div>
      </div>

      {/* Notification list */}
      <div className="overflow-y-auto flex-1">
        {isLoading && (
          <div className="py-6 text-center text-[var(--text-tertiary)]">
            <span className="spinner spinner-sm" />
          </div>
        )}

        {!isLoading && notifications.length === 0 && (
          <div className="py-8 px-4 text-center text-[var(--text-tertiary)] text-[13px]">
            No notifications yet
          </div>
        )}

        {notifications.map((n) => (
          <div
            key={n.id}
            onClick={() => handleClick(n)}
            className={cn(
              'flex gap-2.5 px-4 py-2.5 border-b border-[var(--border-subtle,#f3f4f6)] transition-colors',
              n.link?.url ? 'cursor-pointer hover:bg-[var(--bg-hover,#f3f4f6)]' : 'cursor-default',
              n.readAt ? 'bg-transparent' : 'bg-[var(--bg-muted,#f9fafb)]',
            )}
          >
            <SeverityDot severity={n.severity} />
            <div className="flex-1 min-w-0">
              <div className={cn('text-[13px] leading-snug', n.readAt ? 'font-normal' : 'font-semibold')}>
                {n.title}
              </div>
              {formatTriggerLabel(n.trigger) && (
                <div className="mt-1">
                  <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold uppercase bg-[color-mix(in_srgb,var(--warning,#f59e0b)_12%,transparent)] text-[var(--warning,#f59e0b)]">
                    {formatTriggerLabel(n.trigger)}
                  </span>
                </div>
              )}
              {n.body && (
                <div className="text-[12px] text-[var(--text-secondary)] mt-0.5 truncate">
                  {n.body}
                </div>
              )}
              <div className="flex items-center gap-2 mt-1 text-[11px] text-[var(--text-tertiary)]">
                <span>{relativeTime(n.createdAt)}</span>
                {n.link?.url && (
                  <span className="flex items-center gap-0.5">
                    <ExternalLink size={10} />
                    View in {n.link.surface}
                  </span>
                )}
              </div>
            </div>
            <button
              onClick={(e) => {
                e.stopPropagation()
                dismissNotification.mutate(n.id)
              }}
              title="Dismiss"
              className="text-[var(--text-tertiary)] p-0.5 opacity-50 hover:opacity-100 shrink-0 transition-opacity bg-transparent border-none cursor-pointer"
            >
              <X size={14} />
            </button>
          </div>
        ))}
      </div>

      {/* Footer */}
      {notifications.length > 0 && (
        <div className="px-4 py-2 border-t border-[var(--border)] text-center">
          <button
            onClick={handleViewAll}
            className="text-[12px] text-[var(--accent,#3b82f6)] bg-transparent border-none cursor-pointer hover:underline"
          >
            View all notifications
          </button>
        </div>
      )}
    </div>
  )
}
