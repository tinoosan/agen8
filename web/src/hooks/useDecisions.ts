import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { DecisionView, DecisionSource } from '../lib/types'

export interface DecisionListFilter {
  source?: DecisionSource
  spaceId?: string
  tags?: string[]
  query?: string
  since?: string // ISO 8601
  until?: string // ISO 8601
  sort?: 'newest' | 'oldest'
}

interface DecisionQueryOptions {
  refetchInterval?: number | false
}

export function useRecentDecisions(
  projectId: string | null,
  filter?: DecisionListFilter,
  options?: DecisionQueryOptions,
) {
  const source = filter?.source ?? ''
  const spaceId = filter?.spaceId ?? ''
  const query = filter?.query ?? ''
  const since = filter?.since ?? ''
  const until = filter?.until ?? ''
  const sort = filter?.sort ?? 'newest'
  return useQuery<DecisionView[]>({
    queryKey: ['decision.list', projectId ?? '', source, spaceId, query, since, until, sort],
    queryFn: async () => {
      const params: Record<string, unknown> = {
        projectId: projectId ?? '',
        limit: 50,
      }
      if (filter?.source) params.source = filter.source
      if (filter?.spaceId) params.spaceId = filter.spaceId
      if (filter?.tags?.length) params.tags = filter.tags
      if (filter?.query) params.query = filter.query
      if (filter?.since) params.since = filter.since
      if (filter?.until) params.until = filter.until
      if (filter?.sort) params.sort = filter.sort
      const res = await rpcCall<{ decisions: DecisionView[] }>('decision.list', params)
      return res.decisions
    },
    enabled: !!projectId,
    refetchInterval: options?.refetchInterval ?? 15_000,
  })
}

export interface DecisionLogFilter extends DecisionListFilter {
  page: number
  pageSize: number
}

export function useDecisionLog(projectId: string | null, filter: DecisionLogFilter) {
  const source = filter.source ?? ''
  const spaceId = filter.spaceId ?? ''
  const query = filter.query ?? ''
  const since = filter.since ?? ''
  const until = filter.until ?? ''
  const sort = filter.sort ?? 'newest'
  const tagsKey = (filter.tags ?? []).join(',')
  const offset = Math.max(0, (filter.page - 1) * filter.pageSize)

  return useQuery<{ decisions: DecisionView[]; total: number }>({
    queryKey: ['decision.log', projectId ?? '', source, spaceId, tagsKey, query, since, until, sort, filter.page, filter.pageSize],
    queryFn: async () => {
      const baseParams: Record<string, unknown> = {
        projectId: projectId ?? '',
        sort,
      }
      if (filter.source) baseParams.source = filter.source
      if (filter.spaceId) baseParams.spaceId = filter.spaceId
      if (filter.tags?.length) baseParams.tags = filter.tags
      if (filter.query) baseParams.query = filter.query
      if (filter.since) baseParams.since = filter.since
      if (filter.until) baseParams.until = filter.until

      const [listRes, countRes] = await Promise.all([
        rpcCall<{ decisions: DecisionView[] }>('decision.list', {
          ...baseParams,
          limit: filter.pageSize,
          offset,
        }),
        rpcCall<{ count: number }>('decision.count', baseParams),
      ])
      return {
        decisions: listRes.decisions ?? [],
        total: countRes.count ?? 0,
      }
    },
    enabled: !!projectId,
    refetchInterval: 15_000,
  })
}

export function useExportDecisions() {
  return useMutation({
    mutationFn: async (params: Omit<DecisionListFilter, 'sort'> & { projectId: string; sort?: 'newest' | 'oldest' }) => {
      const res = await rpcCall<{ decisions: DecisionView[] }>('decision.export', params)
      return res.decisions ?? []
    },
  })
}

export function useDeleteDecision() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (decisionId: string) => {
      const res = await rpcCall<{ deleted: boolean }>('decision.delete', { decisionId })
      return res.deleted
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['decision.log'] }),
        queryClient.invalidateQueries({ queryKey: ['decision.list'] }),
      ])
    },
  })
}
