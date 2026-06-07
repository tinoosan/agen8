import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import { qk } from '../lib/queryKeys'
import type { NotificationsListResult } from '../lib/types'

/**
 * Reads the notification inbox for a project.
 *
 * Notifications are server-derived from the live task snapshot (see the
 * notification service): the backend reconciles the desired set into the
 * `notifications` table on every read, so this single query returns both the
 * active list and the unread tally in one round-trip. Freshness is driven by
 * SSE — the `task.` invalidation rule in useRealtimeSync re-runs this query
 * whenever a task event lands — with a slow poll as a disconnect backstop.
 */
export function useNotifications(projectId: string | null) {
  return useQuery<NotificationsListResult>({
    queryKey: qk.notifications(projectId),
    queryFn: () =>
      rpcCall<NotificationsListResult>('notification.list', { projectId }),
    enabled: !!projectId,
    refetchInterval: 30_000,
    staleTime: 2000,
  })
}

/* ── Notification mutation hooks ─────────────────────────────────────────
 *
 * All mutations invalidate ['notification.list'] (prefix match, every project)
 * so the bell badge and any open inbox refresh after a write.
 *
 * ──────────────────────────────────────────────────────────────────────── */

export function useMarkNotificationRead() {
  const queryClient = useQueryClient()
  return useMutation<{ ok: boolean }, Error, { id: string }>({
    mutationFn: (params) => rpcCall<{ ok: boolean }>('notification.markRead', params),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: qk.notificationsAll })
    },
  })
}

export function useMarkAllNotificationsRead() {
  const queryClient = useQueryClient()
  return useMutation<{ ok: boolean; count: number }, Error, { projectId: string }>({
    mutationFn: (params) =>
      rpcCall<{ ok: boolean; count: number }>('notification.markAllRead', params),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: qk.notificationsAll })
    },
  })
}

export function useDismissNotification() {
  const queryClient = useQueryClient()
  return useMutation<{ ok: boolean }, Error, { id: string }>({
    mutationFn: (params) => rpcCall<{ ok: boolean }>('notification.dismiss', params),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: qk.notificationsAll })
    },
  })
}
