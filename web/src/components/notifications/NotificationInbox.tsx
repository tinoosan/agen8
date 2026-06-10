import { useState } from 'react'
import { useLocation } from 'wouter'
import { Bell, Check, X } from 'lucide-react'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { formatRelative } from '../../lib/format'
import type { NotificationItem } from '../../lib/types'
import { SEVERITY_META } from './severity'
import {
  useNotifications,
  useMarkNotificationRead,
  useMarkAllNotificationsRead,
  useDismissNotification,
} from '../../hooks/useNotifications'

function NotificationRow({
  item,
  onActivate,
  onDismiss,
}: {
  item: NotificationItem
  onActivate: (n: NotificationItem) => void
  onDismiss: (id: string) => void
}) {
  const meta = SEVERITY_META[item.severity] ?? SEVERITY_META.info
  const unread = !item.readAt
  const { Icon } = meta

  return (
    <div
      className={cn(
        'group relative flex gap-2.5 px-3 py-2.5 border-b border-[var(--border)] last:border-b-0 transition-colors',
        unread ? 'bg-[var(--bg-active)]/40' : 'bg-transparent',
        item.link && 'cursor-pointer hover:bg-[var(--bg-hover)]',
      )}
      onClick={item.link ? () => onActivate(item) : undefined}
      role={item.link ? 'button' : undefined}
      tabIndex={item.link ? 0 : undefined}
      onKeyDown={
        item.link
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                onActivate(item)
              }
            }
          : undefined
      }
    >
      <Icon size={15} className="mt-0.5 shrink-0" style={{ color: meta.color }} aria-hidden="true" />
      <div className="min-w-0 flex-1">
        <div className="flex items-start gap-2">
          <span className="flex-1 min-w-0 text-[0.8125rem] font-medium text-[var(--text-1)] leading-snug">
            {item.title}
          </span>
          {unread && (
            <span
              className="mt-1 h-1.5 w-1.5 shrink-0 rounded-full"
              style={{ backgroundColor: meta.color }}
              aria-label="Unread"
            />
          )}
        </div>
        {item.body && (
          <p className="mt-0.5 text-[0.75rem] text-[var(--text-3)] leading-snug">{item.body}</p>
        )}
        <span className="mt-1 block text-[0.6875rem] text-[var(--text-3)]">
          {formatRelative(item.createdAt)}
        </span>
      </div>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation()
          onDismiss(item.id)
        }}
        aria-label="Dismiss notification"
        className="absolute right-2 top-2 h-6 w-6 flex items-center justify-center rounded-[6px] border-none bg-transparent cursor-pointer text-[var(--text-3)] opacity-0 group-hover:opacity-100 hover:bg-[var(--bg-hover)] hover:text-[var(--text-1)] transition-all"
      >
        <X size={13} />
      </button>
    </div>
  )
}

/**
 * NotificationInbox — bell trigger + popover list, scoped to the active project.
 *
 * The unread tally rides along with the list query (one round-trip), and the
 * list itself is refreshed by the same SSE signal that moves the task board
 * (see the `task.` rule in useRealtimeSync). Clicking a row marks it read and
 * deep-links to its subject; dismissing removes it. Renders nothing when there
 * is no project context — notifications are per-project.
 */
export default function NotificationInbox({ projectId }: { projectId: string | null }) {
  const [open, setOpen] = useState(false)
  const [, navigate] = useLocation()
  const { data } = useNotifications(projectId)
  const markRead = useMarkNotificationRead()
  const markAllRead = useMarkAllNotificationsRead()
  const dismiss = useDismissNotification()

  if (!projectId) return null

  const notifications = data?.notifications ?? []
  const unreadCount = data?.unreadCount ?? 0

  const handleActivate = (n: NotificationItem) => {
    if (!n.readAt) markRead.mutate({ id: n.id })
    if (n.link?.url) {
      setOpen(false)
      navigate(n.link.url)
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={unreadCount > 0 ? `Notifications (${unreadCount} unread)` : 'Notifications'}
          className="relative h-9 w-9 md:h-7 md:w-7 flex items-center justify-center rounded-[8px] md:rounded-[6px] border-none bg-transparent cursor-pointer text-[var(--text-2)] md:text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] transition-colors shrink-0"
        >
          <Bell size={18} className="md:hidden" />
          <Bell size={15} className="hidden md:block" />
          {unreadCount > 0 && (
            <span
              className="absolute -right-0.5 -top-0.5 min-w-[16px] h-4 px-1 flex items-center justify-center rounded-full text-[0.625rem] font-semibold leading-none text-white"
              style={{ backgroundColor: 'var(--red)' }}
            >
              {unreadCount > 99 ? '99+' : unreadCount}
            </span>
          )}
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        sideOffset={6}
        className="w-[min(22rem,calc(100vw-1.5rem))] p-0 overflow-hidden"
      >
        <div className="flex items-center justify-between px-3 py-2.5 border-b border-[var(--border)]">
          <span className="text-[0.8125rem] font-semibold text-[var(--text-1)]">Notifications</span>
          {unreadCount > 0 && (
            <button
              type="button"
              onClick={() => markAllRead.mutate({ projectId })}
              className="flex items-center gap-1 text-[0.75rem] text-[var(--text-3)] hover:text-[var(--accent)] border-none bg-transparent cursor-pointer p-0 transition-colors"
            >
              <Check size={12} />
              Mark all read
            </button>
          )}
        </div>
        {notifications.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 px-4 py-10 text-center">
            <Bell size={22} className="text-[var(--text-3)] opacity-50" />
            <span className="text-[0.8125rem] text-[var(--text-3)]">You're all caught up</span>
          </div>
        ) : (
          <div className="max-h-[24rem] overflow-y-auto overscroll-contain">
            {notifications.map((n) => (
              <NotificationRow
                key={n.id}
                item={n}
                onActivate={handleActivate}
                onDismiss={(id) => dismiss.mutate({ id })}
              />
            ))}
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}
