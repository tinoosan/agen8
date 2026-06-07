import { useCallback, useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall, rpcUnwrapList } from '../lib/rpc'
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
    refetchInterval: 30_000,
  })

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
