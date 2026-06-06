import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { DecisionView, DecisionSource, DecisionStats } from '../lib/types'

export interface DecisionListFilter {
  source?: DecisionSource
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
  const query = filter?.query ?? ''
  const since = filter?.since ?? ''
  const until = filter?.until ?? ''
  const sort = filter?.sort ?? 'newest'
  return useQuery<DecisionView[]>({
    queryKey: ['decision.list', projectId ?? '', source, query, since, until, sort],
    queryFn: async () => {
      const params: Record<string, unknown> = {
        projectId: projectId ?? '',
        limit: 50,
      }
      if (filter?.source) params.source = filter.source
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

// Fetches a single decision by id for the routed detail page.
export function useDecision(decisionId: string | null) {
  return useQuery<DecisionView>({
    queryKey: ['decision.get', decisionId ?? ''],
    queryFn: async () => {
      const res = await rpcCall<{ decision: DecisionView }>('decision.get', { decisionId })
      return res.decision
    },
    enabled: !!decisionId,
    refetchInterval: 15_000,
  })
}

export interface DecisionLogFilter extends DecisionListFilter {
  page: number
  pageSize: number
}

export function useDecisionLog(projectId: string | null, filter: DecisionLogFilter) {
  const source = filter.source ?? ''
  const query = filter.query ?? ''
  const since = filter.since ?? ''
  const until = filter.until ?? ''
  const sort = filter.sort ?? 'newest'
  const tagsKey = (filter.tags ?? []).join(',')
  const offset = Math.max(0, (filter.page - 1) * filter.pageSize)

  return useQuery<{ decisions: DecisionView[]; total: number }>({
    queryKey: ['decision.log', projectId ?? '', source, tagsKey, query, since, until, sort, filter.page, filter.pageSize],
    queryFn: async () => {
      const baseParams: Record<string, unknown> = {
        projectId: projectId ?? '',
        sort,
      }
      if (filter.source) baseParams.source = filter.source
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

// useDecisionStats summarizes the same filtered set the log view shows. It
// takes the content filters (source/tags/query/since/until) but not sort or
// page — aggregates span every matching row, so paging is irrelevant.
export function useDecisionStats(projectId: string | null, filter?: DecisionListFilter) {
  const source = filter?.source ?? ''
  const query = filter?.query ?? ''
  const since = filter?.since ?? ''
  const until = filter?.until ?? ''
  const tagsKey = (filter?.tags ?? []).join(',')

  return useQuery<DecisionStats>({
    queryKey: ['decision.stats', projectId ?? '', source, tagsKey, query, since, until],
    queryFn: async () => {
      const params: Record<string, unknown> = { projectId: projectId ?? '' }
      if (filter?.source) params.source = filter.source
      if (filter?.tags?.length) params.tags = filter.tags
      if (filter?.query) params.query = filter.query
      if (filter?.since) params.since = filter.since
      if (filter?.until) params.until = filter.until
      return rpcCall<DecisionStats>('decision.stats', params)
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
