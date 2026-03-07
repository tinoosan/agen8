import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { onNotification, rpcCall } from '../lib/rpc'
import type { EventRecord } from '../lib/types'

interface EventsListPaginatedResult {
  events: EventRecord[]
  next?: number
}

const THINKING_TYPES = ['model.thinking.start', 'model.thinking.summary', 'model.thinking.end']

export function useThinkingEvents(runId: string | null, limit = 500) {
  const queryClient = useQueryClient()
  const key = ['events.listPaginated.thinking', runId ?? null, limit]

  const query = useQuery<EventRecord[]>({
    queryKey: key,
    queryFn: async () => {
      if (!runId) return []
      const all: EventRecord[] = []
      let afterSeq = 0

      while (all.length < limit) {
        const pageSize = Math.min(500, limit - all.length)
        const res = await rpcCall<EventsListPaginatedResult>('events.listPaginated', {
          runId,
          afterSeq,
          limit: pageSize,
          types: THINKING_TYPES,
          sortDesc: false,
        })
        const page = res.events ?? []
        if (page.length === 0) break
        all.push(...page)
        if (!res.next || res.next <= afterSeq) break
        afterSeq = res.next
      }

      return all
    },
    enabled: !!runId,
    refetchInterval: 3000,
    staleTime: 2000,
    retry: false,
  })

  useEffect(() => {
    if (!runId) return
    const unsub = onNotification('event.append', (notif) => {
      const params = notif.params as { event?: { runId?: string; type?: string } } | undefined
      const event = params?.event
      if (!event?.runId || event.runId !== runId) return
      if (!event.type || !THINKING_TYPES.includes(event.type)) return
      queryClient.invalidateQueries({ queryKey: key })
    })
    return unsub
  }, [runId, queryClient, key])

  return query
}
