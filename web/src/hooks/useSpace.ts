import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { Space, SpaceListResult, SpaceMember, SpaceMemberListResult, SpaceMemberRemoveResult, SpaceUpdateResult } from '../lib/types'

interface SpaceDeleteResult {
  spaceId: string
}

interface SpaceMemberRegisterParams {
  spaceId: string
  projectId?: string
  displayName?: string
  requestedMemberType: string
  harnessKind: string
  model: string
  effort: string
  harnessPermissionMode?: string
  harnessConfigRef?: string
}

interface SpaceMemberRegisterResult {
  member: SpaceMember
  grantedMemberType: string
}

interface SpaceGetResult {
  space: Space
}

/**
 * Fetch a single space by ID, including space-scoped state such as planMode.
 *
 * Use this when you need the *effective* space state — planMode here
 * reflects the actual execution mode for this space.
 */
export function useSpace(spaceId: string | null) {
  return useQuery<Space>({
    queryKey: ['space.get', spaceId],
    queryFn: async () => {
      const res = await rpcCall<SpaceGetResult>('space.get', { spaceId })
      return res.space
    },
    enabled: !!spaceId,
    staleTime: 5000,
    refetchInterval: 10000,
    retry: false,
  })
}

export function useSpaceList(params: {
  projectId?: string | null
  spaceId?: string | null
  status?: string | null
  limit?: number
  enabled?: boolean
} = {}) {
  const { projectId, spaceId, status, limit, enabled = true } = params
  return useQuery<Space[]>({
    queryKey: ['space.list', projectId ?? '', spaceId ?? '', status ?? ''],
    queryFn: async () => {
      const body: Record<string, string | number> = {}
      if (projectId) body.projectId = projectId
      if (spaceId) body.spaceId = spaceId
      if (status) body.status = status
      if (limit) body.limit = limit
      const res = await rpcCall<SpaceListResult>('space.list', body)
      return res.spaces ?? []
    },
    enabled: enabled && !!(projectId || spaceId),
    staleTime: 10_000,
    retry: false,
  })
}

export function useSpaceMemberList(params: {
  spaceId?: string | null
  projectId?: string | null
  memberType?: string | null
  lifecycleState?: string | null
  limit?: number
  enabled?: boolean
  includeRemoved?: boolean
} = {}) {
  const { spaceId, projectId, memberType, lifecycleState, limit, enabled = true, includeRemoved = false } = params
  return useQuery<SpaceMember[]>({
    queryKey: ['space.member.list', spaceId ?? '', projectId ?? '', memberType ?? '', lifecycleState ?? '', limit ?? '', includeRemoved ? 'includeRemoved' : 'activeOnly'],
    queryFn: async () => {
      const body: Record<string, string | number> = {}
      if (spaceId) body.spaceId = spaceId
      if (projectId) body.projectId = projectId
      if (memberType) body.memberType = memberType
      if (lifecycleState) body.lifecycleState = lifecycleState
      if (limit) body.limit = limit
      const res = await rpcCall<SpaceMemberListResult>('space.member.list', body)
      const members = res.members ?? []
      if (includeRemoved || lifecycleState === 'removed') return members
      return members.filter(member => (member.lifecycleState ?? '').toLowerCase() !== 'removed')
    },
    enabled: enabled && !!(spaceId || projectId),
    staleTime: 5000,
    refetchInterval: 10000,
    retry: false,
  })
}

/**
 * Mutate space-scoped state (title, planMode, or routing metadata).
 *
 * - planMode changes affect only this space's execution mode, not the
 *   template definition's default.
 * - Title and routing changes affect the sidebar and project surfaces,
 *   so the space and project-space lists are invalidated after every update.
 */
export function useSpaceUpdate() {
  const queryClient = useQueryClient()
  return useMutation<
    Space,
    Error,
    {
      spaceId: string
      title?: string
      planMode?: string
      setDefault?: boolean
      customization?: import('../lib/types').SpaceCustomization
    }
  >({
    mutationFn: async (params) => {
      const res = await rpcCall<SpaceUpdateResult>('space.update', params)
      return res.space
    },
    onSuccess: (space) => {
      queryClient.setQueryData(['space.get', space.id], space)
      // Patch every space.list cache entry that contains this space so
      // the sidebar reflects the change instantly without waiting for a
      // network round-trip. Falls back to invalidate for the cases
      // where the entry wasn't in cache yet.
      queryClient.setQueriesData<Space[]>({ queryKey: ['space.list'] }, old => {
        if (!Array.isArray(old)) return old
        return old.map(s => (s.id === space.id ? space : s))
      })
      void queryClient.invalidateQueries({ queryKey: ['space.list'] })
    },
  })
}

/**
 * Permanently delete a space and all its associated data (channels, messages).
 *
 * On success, invalidates space.list so the sidebar
 * removes the deleted entry. The caller is responsible for navigating away if
 * the deleted space was currently focused.
 */
export function useSpaceDelete() {
  const queryClient = useQueryClient()
  return useMutation<string, Error, { spaceId: string }>({
    mutationFn: async ({ spaceId }) => {
      const res = await rpcCall<SpaceDeleteResult>('space.delete', { spaceId })
      return res.spaceId
    },
    onSuccess: (_deletedId, variables) => {
      queryClient.removeQueries({ queryKey: ['space.get', variables.spaceId] })
      void queryClient.invalidateQueries({ queryKey: ['space.list'] })
    },
  })
}

export function useSpaceMemberRegister() {
  const queryClient = useQueryClient()
  return useMutation<SpaceMemberRegisterResult, Error, SpaceMemberRegisterParams>({
    mutationFn: async (params) => {
      return await rpcCall<SpaceMemberRegisterResult>('space.member.register', params)
    },
    onSuccess: (_result, variables) => {
      void queryClient.invalidateQueries({ queryKey: ['space.member.list', variables.spaceId] })
      void queryClient.invalidateQueries({ queryKey: ['channel.list', variables.spaceId] })
      void queryClient.invalidateQueries({ queryKey: ['space.get', variables.spaceId] })
    },
  })
}

export function useSpaceMemberRemove() {
  const queryClient = useQueryClient()
  return useMutation<SpaceMemberRemoveResult, Error, { memberId: string; spaceId: string }>({
    mutationFn: async ({ memberId }) => {
      return await rpcCall<SpaceMemberRemoveResult>('space.member.remove', { memberId })
    },
    onSuccess: (_result, variables) => {
      void queryClient.invalidateQueries({ queryKey: ['space.member.list', variables.spaceId] })
      void queryClient.invalidateQueries({ queryKey: ['channel.list', variables.spaceId] })
      void queryClient.invalidateQueries({ queryKey: ['space.get', variables.spaceId] })
    },
  })
}
