import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { onNotification, rpcCall } from '../lib/rpc'
import type { EventRecord } from '../lib/types'

interface LogsQueryResult {
  events: EventRecord[]
  next?: number
}

interface UseLogEventsOptions {
  sessionId?: string | null
  runId?: string | null
  types?: string[]
  limit?: number
  sortDesc?: boolean
}

export function useLogEvents({
  sessionId,
  runId,
  types,
  limit = 500,
  sortDesc = true,
}: UseLogEventsOptions) {
  const queryClient = useQueryClient()
  const key = ['logs.query', sessionId ?? null, runId ?? null, types ?? null, limit, sortDesc]

  const query = useQuery<EventRecord[]>({
    queryKey: key,
    queryFn: async () => {
      const all: EventRecord[] = []
      let afterSeq = 0
      let remaining = limit

      while (remaining > 0) {
        const pageSize = Math.min(remaining, 500)
        const params: Record<string, unknown> = {
          limit: pageSize,
          afterSeq,
          sortDesc,
        }
        if (sessionId) params.sessionId = sessionId
        if (runId) params.runId = runId
        if (types && types.length > 0) params.types = types

        const res = await rpcCall<LogsQueryResult>('logs.query', params)
        const page = res.events ?? []
        if (page.length === 0) break
        all.push(...page)
        if (!res.next || res.next <= afterSeq) break
        afterSeq = res.next
        remaining -= page.length
      }

      return all
    },
    enabled: !!(sessionId || runId),
    refetchInterval: 3000,
    staleTime: 2000,
    retry: false,
  })

  useEffect(() => {
    if (!sessionId && !runId) return

    const unsub = onNotification('event.append', () => {
      queryClient.invalidateQueries({ queryKey: key })
    })
    return unsub
  }, [sessionId, runId, queryClient, key])

  return query
}
