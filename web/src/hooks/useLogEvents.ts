import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo } from 'react'
import { onNotification, rpcCall } from '../lib/rpc'
import type { LogEntry } from '../lib/types'

interface LogsQueryResult {
  events: LogEntry[]
  next?: number
}

interface UseLogEventsOptions {
  projectRoot?: string | null
  spaceId?: string | null
  runId?: string | null
  types?: string[]
  severities?: string[]
  categories?: string[]
  search?: string
  origin?: string
  limit?: number
  sortDesc?: boolean
}

export function useLogEvents({
  projectRoot,
  spaceId,
  runId,
  types,
  severities,
  categories,
  search,
  origin,
  limit = 200,
  sortDesc = true,
}: UseLogEventsOptions) {
  const queryClient = useQueryClient()
  const key = useMemo(
    () => ['logs.query', projectRoot ?? null, spaceId ?? null, runId ?? null, types ?? null, severities ?? null, categories ?? null, search ?? null, origin ?? null, limit, sortDesc],
    [projectRoot, spaceId, runId, types, severities, categories, search, origin, limit, sortDesc],
  )

  const query = useQuery<LogEntry[]>({
    queryKey: key,
    queryFn: async () => {
      const params: Record<string, unknown> = { limit, sortDesc }
      if (projectRoot) params.projectRoot = projectRoot
      if (spaceId) params.spaceId = spaceId
      if (runId) params.runId = runId
      if (types && types.length > 0) params.types = types
      if (severities && severities.length > 0) params.severities = severities
      if (categories && categories.length > 0) params.categories = categories
      if (search) params.search = search
      if (origin) params.origin = origin
      const res = await rpcCall<LogsQueryResult>('logs.query', params)
      return res.events ?? []
    },
    enabled: !!(projectRoot || spaceId || runId),
    refetchInterval: 3000,
    staleTime: 2000,
    retry: false,
  })

  useEffect(() => {
    if (!projectRoot && !spaceId && !runId) return
    const unsub = onNotification('event.append', () => {
      queryClient.invalidateQueries({ queryKey: key })
    })
    return unsub
  }, [projectRoot, spaceId, runId, queryClient, key])

  return query
}
