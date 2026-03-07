import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo } from 'react'
import { rpcCall, onNotification } from '../lib/rpc'
import type { ActivityEvent } from '../lib/types'

interface ActivityListResult {
  activities: ActivityEvent[]
  totalCount?: number
  nextOffset?: number
}

interface UseActivityOptions {
  threadId: string | null
  teamId?: string | null
  runId?: string | null
  includeChildRuns?: boolean
  limit?: number
}

export function useActivity({ threadId, teamId, runId, includeChildRuns = true, limit = 200 }: UseActivityOptions) {
  const queryClient = useQueryClient()
  const tId = threadId ?? null
  const tmId = teamId ?? null
  const rId = runId ?? null

  // Stable query key — useMemo prevents new array identity on every render,
  // which would cause the useEffect below to churn SSE subscriptions.
  const key = useMemo(
    () => ['activity.list', tId, tmId, rId, includeChildRuns, limit],
    [tId, tmId, rId, includeChildRuns, limit],
  )

  const query = useQuery<ActivityEvent[]>({
    queryKey: key,
    queryFn: async () => {
      const all: ActivityEvent[] = []
      const seen = new Set<string>()
      let offset = 0
      let remaining = Math.max(limit, 1)

      while (remaining > 0) {
        const pageSize = Math.min(remaining, 500)
        const res = await rpcCall<ActivityListResult>('activity.list', {
          threadId: tId ?? undefined,
          teamId: tmId ?? undefined,
          runId: rId ?? undefined,
          includeChildRuns,
          limit: pageSize,
          offset,
          sortDesc: false,
        })
        const page = res.activities ?? []
        for (const activity of page) {
          if (!activity.id || seen.has(activity.id)) continue
          seen.add(activity.id)
          all.push(activity)
        }
        if (!res.nextOffset || page.length === 0 || res.nextOffset <= offset) {
          break
        }
        remaining -= page.length
        offset = res.nextOffset
      }

      return all
    },
    enabled: !!tId,
    refetchInterval: 1500,
    staleTime: 1000,
    retry: false,
  })

  // Invalidate query when we receive an event.append SSE notification.
  useEffect(() => {
    if (!tId) return

    const unsub = onNotification('event.append', () => {
      queryClient.invalidateQueries({ queryKey: key })
    })

    return unsub
  }, [tId, queryClient, key])

  return query
}
