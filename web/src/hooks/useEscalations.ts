import { useEffect } from 'react'
import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query'
import { rpcCall, onNotification } from '../lib/rpc'
import type { EscalationView, EscalationResolution } from '../lib/types'

// ── List pending escalations ──────────────────────────────────────────

export function usePendingEscalations(projectId: string | null) {
  return useQuery<EscalationView[]>({
    queryKey: ['escalation.listPending', projectId ?? ''],
    queryFn: async () => {
      const res = await rpcCall<{ escalations: EscalationView[] }>(
        'escalation.listPending',
        { projectId: projectId ?? '' },
      )
      return res.escalations ?? []
    },
    enabled: !!projectId,
    refetchInterval: 5_000,
  })
}

// ── List all escalations (for strategy map — includes resolved/expired) ──

export function useAllEscalations(
  projectId: string | null,
  options?: { refetchInterval?: number | false },
) {
  return useQuery<EscalationView[]>({
    queryKey: ['escalation.list', projectId ?? '', 'all'],
    queryFn: async () => {
      const res = await rpcCall<{ escalations: EscalationView[] }>(
        'escalation.list',
        { projectId: projectId ?? '', limit: 100 },
      )
      return res.escalations ?? []
    },
    enabled: !!projectId,
    refetchInterval: options?.refetchInterval ?? 10_000,
  })
}

// ── List resolved/canceled escalations ───────────────────────────────

export function useResolvedEscalations(projectId: string | null) {
  return useQuery<EscalationView[]>({
    queryKey: ['escalation.list', projectId ?? '', 'resolved'],
    queryFn: async () => {
      const res = await rpcCall<{ escalations: EscalationView[] }>(
        'escalation.list',
        { projectId: projectId ?? '', status: ['resolved', 'canceled'], limit: 50 },
      )
      return res.escalations ?? []
    },
    enabled: !!projectId,
  })
}

// ── Count pending escalations ─────────────────────────────────────────

export function useEscalationCount(projectId: string | null) {
  return useQuery<number>({
    queryKey: ['escalation.countPending', projectId ?? ''],
    queryFn: async () => {
      const res = await rpcCall<{ count: number }>(
        'escalation.countPending',
        { projectId: projectId ?? '' },
      )
      return res.count ?? 0
    },
    enabled: !!projectId,
    refetchInterval: 5_000,
  })
}

// ── Resolve an escalation ─────────────────────────────────────────────

interface ResolveEscalationParams {
  escalationId: string
  resolution: EscalationResolution
  resolutionNote?: string
  resolvedBy: string
  delegatedTo?: string
}

export function useResolveEscalation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (params: ResolveEscalationParams) => {
      await rpcCall('escalation.resolve', params)
    },
    onSuccess: () => {
      // Full cross-surface invalidation (F21)
      queryClient.invalidateQueries({ queryKey: ['escalation.listPending'] })
      queryClient.invalidateQueries({ queryKey: ['escalation.list'] })
      queryClient.invalidateQueries({ queryKey: ['escalation.countPending'] })
      queryClient.invalidateQueries({ queryKey: ['escalation.get'] })
      queryClient.invalidateQueries({ queryKey: ['task.list'] }) // task may unblock
      queryClient.invalidateQueries({ queryKey: ['mission.list'] })
      queryClient.invalidateQueries({ queryKey: ['decision.list'] }) // auto-decision created
    },
  })
}

// ── SSE-driven invalidation (F21) ─────────────────────────────────────

export function useEscalationSSE() {
  const queryClient = useQueryClient()
  useEffect(() => {
    const unsub = onNotification('event.append', (notif: Record<string, unknown>) => {
      const event = notif?.event as Record<string, unknown> | undefined
      const type = (event?.type as string) ?? ''
      if (type.startsWith('escalation.')) {
        queryClient.invalidateQueries({ queryKey: ['escalation.listPending'] })
        queryClient.invalidateQueries({ queryKey: ['escalation.list'] })
        queryClient.invalidateQueries({ queryKey: ['escalation.countPending'] })
        queryClient.invalidateQueries({ queryKey: ['escalation.get'] })
        if (type === 'escalation.resolved' || type === 'escalation.canceled') {
          queryClient.invalidateQueries({ queryKey: ['task.list'] })
          queryClient.invalidateQueries({ queryKey: ['decision.list'] })
        }
      }
    })
    return unsub
  }, [queryClient])
}

// ── Cancel an escalation ──────────────────────────────────────────────

export function useCancelEscalation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (escalationId: string) => {
      await rpcCall('escalation.cancel', { escalationId })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['escalation.listPending'] })
      queryClient.invalidateQueries({ queryKey: ['escalation.countPending'] })
    },
  })
}
