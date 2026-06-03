import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query'
import { useEffect } from 'react'
import { rpcCall, onNotification } from '../lib/rpc'
import type {
  NotificationItem,
  NotificationsListResult,
  NotificationsUnreadCountResult,
  NotificationRule,
  NotificationsRulesListResult,
  NotificationsSourcesListResult,
} from '../lib/types'

// ── Notification list + real-time updates ───────────────────────────────

interface UseNotificationsOptions {
  userId: string | null
  source?: string
  severity?: string
  unread?: boolean
  limit?: number
}

export function useNotifications({ userId, source, severity, unread, limit = 50 }: UseNotificationsOptions) {
  const queryClient = useQueryClient()
  const key = ['notifications.list', userId, source, severity, unread, limit]

  const query = useQuery<NotificationItem[]>({
    queryKey: key,
    queryFn: async () => {
      if (!userId) return []
      const res = await rpcCall<NotificationsListResult>('notifications.list', {
        userId,
        source,
        severity,
        unread,
        limit,
      })
      return res.notifications ?? []
    },
    enabled: !!userId,
    refetchInterval: 60_000,
  })

  // Subscribe to real-time notification events via SSE
  useEffect(() => {
    const unsubRaised = onNotification('notification.raised', () => {
      queryClient.invalidateQueries({ queryKey: ['notifications.list'] })
      queryClient.invalidateQueries({ queryKey: ['notifications.unreadCount'] })
    })
    const unsubRead = onNotification('notification.read', () => {
      queryClient.invalidateQueries({ queryKey: ['notifications.list'] })
      queryClient.invalidateQueries({ queryKey: ['notifications.unreadCount'] })
    })
    // Auto-dismiss-on-resolve: server broadcasts this when an evaluator's
    // SubjectResolver clears one or more rows (e.g. ask_user resolved).
    // Without this listener the inbox row would only disappear on the
    // next 60s poll.
    const unsubDismissedBySubject = onNotification('notification.dismissed_by_subject', () => {
      queryClient.invalidateQueries({ queryKey: ['notifications.list'] })
      queryClient.invalidateQueries({ queryKey: ['notifications.unreadCount'] })
    })
    return () => {
      unsubRaised()
      unsubRead()
      unsubDismissedBySubject()
    }
  }, [queryClient])

  return query
}

// ── Unread count ────────────────────────────────────────────────────────

export function useUnreadCount(userId: string | null) {
  const queryClient = useQueryClient()

  const query = useQuery<number>({
    queryKey: ['notifications.unreadCount', userId],
    queryFn: async () => {
      if (!userId) return 0
      const res = await rpcCall<NotificationsUnreadCountResult>('notifications.unreadCount', { userId })
      return res.count
    },
    enabled: !!userId,
    refetchInterval: 30_000,
  })

  // Real-time updates
  useEffect(() => {
    const unsubRaised = onNotification('notification.raised', () => {
      queryClient.invalidateQueries({ queryKey: ['notifications.unreadCount'] })
    })
    const unsubRead = onNotification('notification.read', () => {
      queryClient.invalidateQueries({ queryKey: ['notifications.unreadCount'] })
    })
    const unsubDismissedBySubject = onNotification('notification.dismissed_by_subject', () => {
      queryClient.invalidateQueries({ queryKey: ['notifications.unreadCount'] })
    })
    return () => {
      unsubRaised()
      unsubRead()
      unsubDismissedBySubject()
    }
  }, [queryClient])

  return query
}

// ── Mutations ───────────────────────────────────────────────────────────

export function useMarkRead() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      await rpcCall('notifications.markRead', { id })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications.list'] })
      queryClient.invalidateQueries({ queryKey: ['notifications.unreadCount'] })
    },
  })
}

export function useMarkAllRead() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (userId: string) => {
      await rpcCall('notifications.markAllRead', { userId })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications.list'] })
      queryClient.invalidateQueries({ queryKey: ['notifications.unreadCount'] })
    },
  })
}

export function useDismissNotification() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      await rpcCall('notifications.dismiss', { id })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications.list'] })
      queryClient.invalidateQueries({ queryKey: ['notifications.unreadCount'] })
    },
  })
}

// ── Rules ───────────────────────────────────────────────────────────────

export function useNotificationRules(userId: string | null) {
  return useQuery<NotificationRule[]>({
    queryKey: ['notifications.rules.list', userId],
    queryFn: async () => {
      if (!userId) return []
      const res = await rpcCall<NotificationsRulesListResult>('notifications.rules.list', { userId })
      return res.rules ?? []
    },
    enabled: !!userId,
  })
}

export function useSaveRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (rule: NotificationRule) => {
      await rpcCall('notifications.rules.save', { rule })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications.rules.list'] })
    },
  })
}

export function useDeleteRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      await rpcCall('notifications.rules.delete', { id })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications.rules.list'] })
    },
  })
}

// ── Sources (for preferences auto-population) ──────────────────────────

export function useNotificationSources() {
  return useQuery<NotificationsSourcesListResult>({
    queryKey: ['notifications.sources.list'],
    queryFn: () => rpcCall<NotificationsSourcesListResult>('notifications.sources.list'),
    staleTime: 5 * 60_000,
  })
}

