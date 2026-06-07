import { useCallback, useEffect, useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { onNotification, rpcCall, rpcUnwrapList } from '../lib/rpc'
import { qk } from '../lib/queryKeys'

/**
 * Server-backed pinning.
 *
 * Replaces the old localStorage usePinnedMissions hook: pins now persist
 * per-project on the server (pin.add / pin.remove / pin.list), so they survive
 * across browsers and sessions and are shared by everyone in the project.
 *
 * The public shape ({ pinnedIds, isPinned, togglePin }) is kept compatible with
 * the old hook so existing call sites are a near drop-in, with one addition:
 * togglePin takes an optional nodeType so any node (mission, decision, …) can be
 * pinned, not just missions.
 */

export interface PinView {
  projectId: string
  nodeRef: string
  nodeType?: string
  createdAt: string
}

export type PinnableNodeType = 'mission' | 'decision' | 'task' | 'keyResult' | string

export interface UsePinsResult {
  pinnedIds: Set<string>
  isPinned: (nodeRef: string) => boolean
  togglePin: (nodeRef: string, nodeType?: PinnableNodeType) => void
  isLoading: boolean
}

export function usePins(projectId: string | null): UsePinsResult {
  const queryClient = useQueryClient()
  const key = qk.pins(projectId)

  const { data: pins, isLoading } = useQuery<PinView[]>({
    queryKey: key,
    queryFn: () => rpcUnwrapList<PinView>('pin.list', { projectId: projectId ?? '' }, 'pins'),
    enabled: !!projectId,
    // SSE (below) is the primary freshness path; this slow interval is only a
    // backstop that self-heals pins missed during a disconnect. It dropped from
    // 30s to 60s once live invalidation landed — see docs/architecture/realtime-events.html.
    refetchInterval: 60_000,
  })

  // Live cross-device sync: a pin change made anywhere — another tab, another
  // device, or an agent — arrives over SSE as an `event.append` notification
  // carrying a `pin.*` event type. On any pin event, invalidate the shared pin
  // root so every consumer refetches from the same source of truth the
  // mutations write through. The /events stream is already scoped to this
  // project server-side, so the client only needs to match the `pin.` prefix.
  useEffect(() => {
    if (!projectId) return
    return onNotification('event.append', (notif: Record<string, unknown>) => {
      const event = notif?.event as Record<string, unknown> | undefined
      const type = (event?.type as string) ?? ''
      if (type.startsWith('pin.')) {
        queryClient.invalidateQueries({ queryKey: qk.pinsAll })
      }
    })
  }, [projectId, queryClient])

  const pinnedIds = useMemo(
    () => new Set((pins ?? []).map((p) => p.nodeRef)),
    [pins],
  )

  // Optimistic add: drop the pin into the cache immediately, roll back on error.
  const addPin = useMutation({
    mutationFn: (vars: { nodeRef: string; nodeType?: string }) =>
      rpcCall('pin.add', {
        projectId: projectId ?? '',
        nodeRef: vars.nodeRef,
        nodeType: vars.nodeType ?? '',
      }),
    onMutate: async (vars) => {
      await queryClient.cancelQueries({ queryKey: key })
      const prev = queryClient.getQueryData<PinView[]>(key)
      queryClient.setQueryData<PinView[]>(key, (old) => {
        const list = old ?? []
        if (list.some((p) => p.nodeRef === vars.nodeRef)) return list
        return [
          {
            projectId: projectId ?? '',
            nodeRef: vars.nodeRef,
            nodeType: vars.nodeType,
            createdAt: new Date().toISOString(),
          },
          ...list,
        ]
      })
      return { prev }
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(key, ctx.prev)
    },
    // Invalidate the shared pin root to force every pin consumer to re-query after a mutation.
    // This is deliberately broad (`pin.list`) so Dashboard rows and command palette both
    // absorb write outcomes from the same write-behind source of truth.
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: qk.pinsAll })
    },
  })

  // Optimistic remove: strip the pin from the cache immediately, roll back on error.
  const removePin = useMutation({
    mutationFn: (vars: { nodeRef: string }) =>
      rpcCall('pin.remove', { projectId: projectId ?? '', nodeRef: vars.nodeRef }),
    onMutate: async (vars) => {
      await queryClient.cancelQueries({ queryKey: key })
      const prev = queryClient.getQueryData<PinView[]>(key)
      queryClient.setQueryData<PinView[]>(key, (old) =>
        (old ?? []).filter((p) => p.nodeRef !== vars.nodeRef),
      )
      return { prev }
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(key, ctx.prev)
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: qk.pinsAll })
    },
  })

  const isPinned = useCallback((nodeRef: string) => pinnedIds.has(nodeRef), [pinnedIds])

  const togglePin = useCallback(
    (nodeRef: string, nodeType?: PinnableNodeType) => {
      if (!projectId) return
      if (pinnedIds.has(nodeRef)) {
        removePin.mutate({ nodeRef })
      } else {
        addPin.mutate({ nodeRef, nodeType })
      }
    },
    [projectId, pinnedIds, addPin, removePin],
  )

  return { pinnedIds, isPinned, togglePin, isLoading }
}
