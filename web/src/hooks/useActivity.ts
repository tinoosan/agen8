import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
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
  includeChildRuns?: boolean
  limit?: number
}

export function useActivity({ threadId, teamId, includeChildRuns = true, limit = 200 }: UseActivityOptions) {
  const queryClient = useQueryClient()
  const key = ['activity.list', threadId, teamId ?? null, includeChildRuns, limit]

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
          threadId,
          teamId: teamId ?? undefined,
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
    enabled: !!threadId,
    refetchInterval: 1500,
    staleTime: 1000,
    retry: false,
  })

  useEffect(() => {
    if (!threadId) return

    const unsub = onNotification('event.append', (notif) => {
      const params = notif.params as { event?: { runId?: string } } | undefined
      const runId = params?.event?.runId
      if (!runId) return
      queryClient.invalidateQueries({ queryKey: key })
    })

    return unsub
  }, [threadId, queryClient, key])

  return query
}
