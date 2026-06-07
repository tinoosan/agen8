import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall, rpcUnwrap, rpcUnwrapList } from '../lib/rpc'
import { qk } from '../lib/queryKeys'
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

/* ── Shared decision-filter helpers ──────────────────────────────────────
 *
 * The three decision read hooks (useRecentDecisions, useDecisionLog,
 * useDecisionStats) all derive the same things from a DecisionListFilter:
 *  - request params: the optional content filters folded onto the RPC params
 *  - query-key parts: the normalized ('' / 'newest' / joined-tags) variants
 *    that make cache identity stable.
 * Centralizing both here keeps the filter→params and filter→key mappings in
 * one place so they can't drift apart.
 *
 * ──────────────────────────────────────────────────────────────────────── */

// Folds the content filters (source/tags/query/since/until) onto an existing
// params object and returns it. Sort and paging are intentionally left to the
// caller — only useDecisionLog/useRecentDecisions send sort, and stats never
// pages.
function applyDecisionContentFilters(
  params: Record<string, unknown>,
  filter?: DecisionListFilter,
): Record<string, unknown> {
  if (filter?.source) params.source = filter.source
  if (filter?.tags?.length) params.tags = filter.tags
  if (filter?.query) params.query = filter.query
  if (filter?.since) params.since = filter.since
  if (filter?.until) params.until = filter.until
  return params
}

// Normalizes a filter into the scalar parts used to build query keys. Keeping
// the '' / 'newest' / joined-tags normalization here means every hook's cache
// identity is computed the same way.
function decisionFilterKeyParts(filter?: DecisionListFilter) {
  return {
    source: filter?.source ?? '',
    query: filter?.query ?? '',
    since: filter?.since ?? '',
    until: filter?.until ?? '',
    sort: filter?.sort ?? 'newest',
    tagsKey: (filter?.tags ?? []).join(','),
  }
}

export function useRecentDecisions(
  projectId: string | null,
  filter?: DecisionListFilter,
  options?: DecisionQueryOptions,
) {
  const { source, query, since, until, sort } = decisionFilterKeyParts(filter)
  return useQuery<DecisionView[]>({
    queryKey: qk.decisionList(projectId ?? '', source, query, since, until, sort),
    queryFn: async () => {
      const params = applyDecisionContentFilters({ projectId: projectId ?? '', limit: 50 }, filter)
      if (filter?.sort) params.sort = filter.sort
      return rpcUnwrapList<DecisionView>('decision.list', params, 'decisions')
    },
    enabled: !!projectId,
    refetchInterval: options?.refetchInterval ?? 15_000,
  })
}

// Fetches a single decision by id for the routed detail page.
export function useDecision(decisionId: string | null) {
  return useQuery<DecisionView>({
    queryKey: qk.decisionGet(decisionId),
    queryFn: async () => {
      return rpcUnwrap<DecisionView>('decision.get', { decisionId }, 'decision')
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
  const { source, query, since, until, sort, tagsKey } = decisionFilterKeyParts(filter)
  const offset = Math.max(0, (filter.page - 1) * filter.pageSize)

  return useQuery<{ decisions: DecisionView[]; total: number }>({
    queryKey: qk.decisionLog(projectId ?? '', source, tagsKey, query, since, until, sort, filter.page, filter.pageSize),
    queryFn: async () => {
      const baseParams = applyDecisionContentFilters({ projectId: projectId ?? '', sort }, filter)

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
  const { source, query, since, until, tagsKey } = decisionFilterKeyParts(filter)

  return useQuery<DecisionStats>({
    queryKey: qk.decisionStats(projectId ?? '', source, tagsKey, query, since, until),
    queryFn: async () => {
      const params = applyDecisionContentFilters({ projectId: projectId ?? '' }, filter)
      return rpcCall<DecisionStats>('decision.stats', params)
    },
    enabled: !!projectId,
    refetchInterval: 15_000,
  })
}

export function useExportDecisions() {
  return useMutation({
    mutationFn: async (params: Omit<DecisionListFilter, 'sort'> & { projectId: string; sort?: 'newest' | 'oldest' }) => {
      return rpcUnwrapList<DecisionView>('decision.export', params, 'decisions')
    },
  })
}

export function useDeleteDecision() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (decisionId: string) => {
      return rpcUnwrap<boolean>('decision.delete', { decisionId }, 'deleted')
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: qk.decisionLogAll }),
        queryClient.invalidateQueries({ queryKey: qk.decisionsAll }),
        queryClient.invalidateQueries({ queryKey: qk.decisionStatsRoot }),
        queryClient.invalidateQueries({ queryKey: qk.decisionGetAll }),
      ])
    },
  })
}
