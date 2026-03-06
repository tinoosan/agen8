import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { rpcCall, onNotification } from '../lib/rpc'
import type { ActivityEvent } from '../lib/types'

interface ActivityListResult {
  events: ActivityEvent[]
}

export function useActivity(teamId: string | null) {
  const queryClient = useQueryClient()
  const key = ['activity.list', teamId]

  const query = useQuery<ActivityEvent[]>({
    queryKey: key,
    queryFn: async () => {
      const res = await rpcCall<ActivityListResult>('activity.list', {
        teamId,
        limit: 100,
      })
      return res.events ?? []
    },
    enabled: !!teamId,
    staleTime: Infinity,
    retry: false,
  })

  useEffect(() => {
    if (!teamId) return

    const unsub = onNotification('event.append', (notif) => {
      const params = notif.params as { event?: ActivityEvent; teamId?: string } | undefined
      if (!params?.event) return
      if (params.teamId && params.teamId !== teamId) return

      queryClient.setQueryData<ActivityEvent[]>(key, (prev) => {
        const next = [...(prev ?? []), params.event!]
        // Keep last 200 events
        return next.length > 200 ? next.slice(next.length - 200) : next
      })
    })

    return unsub
  }, [teamId, queryClient])

  return query
}
