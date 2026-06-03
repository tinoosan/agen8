import { useEffect } from 'react'
import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query'
import { rpcCall, onNotification } from '../lib/rpc'
import type { OpActionView, OpActionOutcomeStatus } from '../lib/types'

// ── Query keys ────────────────────────────────────────────────────────

const OA_KEYS = {
  listPending: (projectId: string) => ['opAction.listPending', projectId] as const,
  listStrategyMap: (projectId: string) => ['opAction.listStrategyMap', projectId] as const,
  get: (actionId: string) => ['opAction.get', actionId] as const,
  countStatus: (projectId: string) => ['opAction.countStatus', projectId] as const,
}

const STRATEGY_MAP_OA_STATUSES = [
  'pending',
  'acknowledged',
  'in_progress',
  'pending_verification',
  'blocked',
  'completed',
] as const

// ── List pending operator actions ─────────────────────────────────────

export function usePendingOpActions(
  projectId: string | null,
  options?: { refetchInterval?: number | false },
) {
  return useQuery<OpActionView[]>({
    queryKey: OA_KEYS.listPending(projectId ?? ''),
    queryFn: async () => {
      const res = await rpcCall<{ opActions: OpActionView[] }>(
        'opAction.listPending',
        { projectId: projectId ?? '' },
      )
      return res.opActions ?? []
    },
    enabled: !!projectId,
    refetchInterval: options?.refetchInterval ?? 5_000,
  })
}

// ── List strategy-map OAs (active + completed, no canceled) ──────────

export function useStrategyMapOpActions(
  projectId: string | null,
  options?: { refetchInterval?: number | false },
) {
  return useQuery<OpActionView[]>({
    queryKey: OA_KEYS.listStrategyMap(projectId ?? ''),
    queryFn: async () => {
      const res = await rpcCall<{ opActions: OpActionView[] }>(
        'opAction.list',
        { projectId: projectId ?? '', status: [...STRATEGY_MAP_OA_STATUSES], limit: 200 },
      )
      return (res.opActions ?? []).filter((oa) => oa.status !== 'canceled')
    },
    enabled: !!projectId,
    refetchInterval: options?.refetchInterval ?? 30_000,
    staleTime: 20_000,
    refetchOnWindowFocus: false,
  })
}

// ── List completed/canceled operator actions ──────────────────────────

export function useCompletedOpActions(projectId: string | null) {
  const completed = useQuery<OpActionView[]>({
    queryKey: ['opAction.list', projectId ?? '', 'completed'],
    queryFn: async () => {
      const res = await rpcCall<{ opActions: OpActionView[] }>(
        'opAction.list',
        { projectId: projectId ?? '', status: ['completed'], limit: 50 },
      )
      return res.opActions ?? []
    },
    enabled: !!projectId,
  })
  const canceled = useQuery<OpActionView[]>({
    queryKey: ['opAction.list', projectId ?? '', 'canceled'],
    queryFn: async () => {
      const res = await rpcCall<{ opActions: OpActionView[] }>(
        'opAction.list',
        { projectId: projectId ?? '', status: ['canceled'], limit: 50 },
      )
      return res.opActions ?? []
    },
    enabled: !!projectId,
  })
  return {
    data: [...(completed.data ?? []), ...(canceled.data ?? [])].sort(
      (a, b) => new Date(b.completedAt ?? b.createdAt).getTime() - new Date(a.completedAt ?? a.createdAt).getTime(),
    ),
    isLoading: completed.isLoading || canceled.isLoading,
  }
}

// ── Get single operator action (for detail panel) ─────────────────────

export function useOpAction(actionId: string | null) {
  return useQuery<OpActionView>({
    queryKey: OA_KEYS.get(actionId ?? ''),
    queryFn: async () => {
      const res = await rpcCall<{ opAction: OpActionView }>(
        'opAction.get',
        { actionId: actionId ?? '' },
      )
      return res.opAction
    },
    enabled: !!actionId,
    refetchInterval: 3_000,
  })
}

// ── Count by status ───────────────────────────────────────────────────

export function useOpActionCounts(projectId: string | null) {
  return useQuery<Record<string, number>>({
    queryKey: OA_KEYS.countStatus(projectId ?? ''),
    queryFn: async () => {
      const res = await rpcCall<{ counts: Record<string, number> }>(
        'opAction.countStatus',
        { projectId: projectId ?? '' },
      )
      return res.counts ?? {}
    },
    enabled: !!projectId,
    refetchInterval: 5_000,
  })
}

// ── Mutation helper: invalidates standard OA queries ──────────────────

function useOAMutation<T>(
  mutationFn: (params: T) => Promise<void>,
  extraInvalidations?: string[][],
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn,
    onSuccess: () => {
      // Full cross-surface invalidation (F21)
      queryClient.invalidateQueries({ queryKey: ['opAction.listPending'] })
      queryClient.invalidateQueries({ queryKey: ['opAction.listStrategyMap'] })
      queryClient.invalidateQueries({ queryKey: ['opAction.list'] })
      queryClient.invalidateQueries({ queryKey: ['opAction.countStatus'] })
      queryClient.invalidateQueries({ queryKey: ['opAction.get'] })
      queryClient.invalidateQueries({ queryKey: ['task.list'] }) // task may unblock
      for (const key of extraInvalidations ?? []) {
        queryClient.invalidateQueries({ queryKey: key })
      }
    },
  })
}

// ── SSE-driven invalidation (F21) ─────────────────────────────────────

export function useOpActionSSE() {
  const queryClient = useQueryClient()
  useEffect(() => {
    const unsub = onNotification('event.append', (notif: Record<string, unknown>) => {
      const event = notif?.event as Record<string, unknown> | undefined
      const type = (event?.type as string) ?? ''
      if (type.startsWith('oa.')) {
        queryClient.invalidateQueries({ queryKey: ['opAction.listPending'] })
        queryClient.invalidateQueries({ queryKey: ['opAction.listStrategyMap'] })
        queryClient.invalidateQueries({ queryKey: ['opAction.list'] })
        queryClient.invalidateQueries({ queryKey: ['opAction.countStatus'] })
        queryClient.invalidateQueries({ queryKey: ['opAction.get'] })
        if (type === 'oa.completed' || type === 'oa.canceled') {
          queryClient.invalidateQueries({ queryKey: ['task.list'] })
          queryClient.invalidateQueries({ queryKey: ['mission.list'] })
        }
      }
    })
    return unsub
  }, [queryClient])
}

// ── Start an operator action ──────────────────────────────────────────

export function useStartOpAction() {
  return useOAMutation<{ actionId: string }>(
    async (params) => { await rpcCall('opAction.start', params) },
  )
}

// ── Complete an operator action ───────────────────────────────────────

interface CompleteParams {
  actionId: string
  outcomeStatus: OpActionOutcomeStatus
  outcomeSummary: string
  outcomePairs?: Record<string, string>
}

export function useCompleteOpAction() {
  return useOAMutation<CompleteParams>(
    async (params) => { await rpcCall('opAction.complete', params) },
    [['mission.list']],
  )
}

// ── Verify an operator action ─────────────────────────────────────────

interface VerifyParams {
  actionId: string
  accepted: boolean
  feedback?: string
}

export function useVerifyOpAction() {
  return useOAMutation<VerifyParams>(
    async (params) => { await rpcCall('opAction.verify', params) },
  )
}

// ── Block an operator action ──────────────────────────────────────────

export function useBlockOpAction() {
  return useOAMutation<{ actionId: string; reason: string }>(
    async (params) => { await rpcCall('opAction.block', params) },
  )
}

// ── Unblock an operator action ────────────────────────────────────────

export function useUnblockOpAction() {
  return useOAMutation<{ actionId: string }>(
    async (params) => { await rpcCall('opAction.unblock', params) },
  )
}

// ── Cancel an operator action ─────────────────────────────────────────

export function useCancelOpAction() {
  return useOAMutation<{ actionId: string }>(
    async (params) => { await rpcCall('opAction.cancel', params) },
  )
}

// ── Add progress note ─────────────────────────────────────────────────

export function useAddOpActionNote() {
  return useOAMutation<{ actionId: string; text: string }>(
    async (params) => { await rpcCall('opAction.addNote', params) },
  )
}

// ── Add comment ───────────────────────────────────────────────────────

export function useAddOpActionComment() {
  return useOAMutation<{ actionId: string; author: string; text: string }>(
    async (params) => { await rpcCall('opAction.addComment', params) },
  )
}
