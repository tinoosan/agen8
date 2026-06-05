import { useMemo } from 'react'
import { useQueries } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { ContextLink } from '../lib/types'

interface GraphLinksResult {
  contextLinks: ContextLink[]
}

export interface ContextLinkEntity {
  entityType: string
  entityId: string
}

/**
 * Fetches all graph links involving the provided KR/mission IDs (as targets)
 * and the provided leaf entity IDs (as sources).
 *
 * Querying both directions gives full coverage:
 *   - graph.linksByTarget(KRs, missions)  → task/decision → KR/mission
 *   - graph.linksBySource(leaf nodes)     → decision → task (made_during), etc.
 *
 * Returns a deduplicated flat array of ContextLink records.
 */
export function useContextLinks(
  krIds: string[],
  missionIds: string[],
  leafSources: ContextLinkEntity[],
  options?: { refetchInterval?: number | false },
): {
  contextLinks: ContextLink[]
  isLoading: boolean
} {
  const targetQueries = useMemo(() => {
    const q: { method: string; params: Record<string, string> }[] = []
    for (const id of krIds)
      q.push({ method: 'graph.linksByTarget', params: { targetType: 'key_result', targetId: id } })
    for (const id of missionIds)
      q.push({ method: 'graph.linksByTarget', params: { targetType: 'mission', targetId: id } })
    for (const { entityType, entityId } of leafSources)
      q.push({ method: 'graph.linksBySource', params: { sourceType: entityType, sourceId: entityId } })
    return q
  }, [krIds, missionIds, leafSources])

  const queries = useQueries({
    queries: targetQueries.map(({ method, params }) => ({
      queryKey: [method, ...Object.values(params)],
      queryFn: async (): Promise<ContextLink[]> => {
        const res = await rpcCall<GraphLinksResult>(method, params)
        return res.contextLinks ?? []
      },
      enabled: Object.values(params).every(v => !!v),
      refetchInterval: options?.refetchInterval ?? 30_000,
      staleTime: 20_000,
      refetchOnWindowFocus: false,
    })),
  })

  const isLoading = queries.some(q => q.isLoading && !q.data)

  const contextLinks = useMemo(() => {
    const seen = new Set<string>()
    const result: ContextLink[] = []
    for (const q of queries) {
      for (const link of q.data ?? []) {
        if (!seen.has(link.id)) {
          seen.add(link.id)
          result.push(link)
        }
      }
    }
    return result
  }, [queries])

  return { contextLinks, isLoading }
}
