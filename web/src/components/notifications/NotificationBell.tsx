import { useState } from 'react'
import { Bell } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useUnreadCount } from '../../hooks/useNotifications'
import NotificationDropdown from './NotificationDropdown'

interface NotificationBellProps {
  userId: string | null
}

/**
 * Notification bell icon with unread count badge.
 * Clicking opens the NotificationDropdown panel via a Radix Popover (portaled,
 * so it escapes any ancestor overflow:hidden — e.g. the Sidebar).
 * Badge updates in real-time via SSE notification.raised events.
 */
export default function NotificationBell({ userId }: NotificationBellProps) {
  const [open, setOpen] = useState(false)
  const { data: unreadCount = 0 } = useUnreadCount(userId)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          title="Notifications"
          aria-label={`Notifications${unreadCount > 0 ? ` (${unreadCount} unread)` : ''}`}
          className={cn('relative px-2 text-[var(--text-secondary)]')}
        >
          <Bell size={16} />
          {unreadCount > 0 && (
            <span
              className="absolute top-0 right-0.5 bg-destructive text-white rounded-full text-[10px] font-semibold min-w-4 h-4 flex items-center justify-center px-1"
            >
              {unreadCount > 99 ? '99+' : unreadCount}
            </span>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        sideOffset={8}
        className="p-0 border-0 shadow-none bg-transparent w-auto"
      >
        <NotificationDropdown
          userId={userId}
          onClose={() => setOpen(false)}
        />
      </PopoverContent>
    </Popover>
  )
}
