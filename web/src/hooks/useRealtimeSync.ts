import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { onNotification } from '../lib/rpc'
import { qk } from '../lib/queryKeys'

/**
 * Single source of truth for SSE-driven cache invalidation.
 *
 * Every domain mutation — by this browser, another tab, another device, or an
 * agent — fans out from the server as one `event.append` notification carrying a
 * dotted `event.type` (task.created, decision.logged, mission.activated, …). The
 * daemon's /events stream is already project-scoped server-side, so the client
 * only matches on the type prefix and invalidates the affected query roots.
 *
 * Centralizing here (mounted once in <App/>) replaced the per-component SSE
 * subscriptions that previously lived in useProjectTasks/usePins: one
 * EventSource consumer, one place that maps event → cache. Adding a surface to
 * live updates is now a one-line rule below plus a matching `event.type` prefix
 * emitted by the backend — see docs/architecture/realtime-events.html.
 */

type QueryRoot = readonly unknown[]

const INVALIDATION_RULES: { prefix: string; roots: QueryRoot[] }[] = [
  // Notifications are derived from the live task snapshot, so any task event may
  // create/clear one — refresh the inbox on the same signal that moves the board.
  { prefix: 'task.', roots: [qk.tasksBoardAll, qk.taskGetAll, qk.notificationsAll] },
  {
    prefix: 'decision.',
    roots: [qk.decisionsAll, qk.decisionGetAll, qk.decisionLogAll, qk.decisionStatsRoot],
  },
  {
    prefix: 'mission.',
    roots: [qk.missionsAll, qk.missionGetAll, qk.sidebarGlobalMissionsRoot],
  },
  {
    // KR progress moves mission rollups too, so missions are invalidated here as well.
    prefix: 'kr.',
    roots: [
      qk.keyResultsAll,
      qk.keyResultGetAll,
      qk.keyResultsListAllRoot,
      qk.keyResultsByMissionSetRoot,
      qk.keyResultProgressHistoryRoot,
      qk.missionsAll,
    ],
  },
  { prefix: 'space.member.', roots: [qk.projectMembersAll] },
  { prefix: 'pin.', roots: [qk.pinsAll] },
]

/**
 * Subscribes to the SSE notification stream and invalidates the matching query
 * roots whenever a domain event arrives. Pass `enabled=false` (e.g. while
 * unauthenticated) to skip opening the stream entirely.
 */
export function useRealtimeInvalidation(enabled = true) {
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!enabled) return
    return onNotification('event.append', (notif: Record<string, unknown>) => {
      const event = notif?.event as Record<string, unknown> | undefined
      const type = (event?.type as string) ?? ''
      if (!type) return
      const rule = INVALIDATION_RULES.find((r) => type.startsWith(r.prefix))
      if (!rule) return
      for (const root of rule.roots) {
        queryClient.invalidateQueries({ queryKey: root })
      }
    })
  }, [enabled, queryClient])
}
