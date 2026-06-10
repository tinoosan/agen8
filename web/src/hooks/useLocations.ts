import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall, rpcUnwrap, rpcUnwrapList } from '../lib/rpc'
import { qk } from '../lib/queryKeys'
import type { ExecutionLocation, LocationAddress } from '../lib/types'

export interface LocationAuthInput {
  mode?: string
  credentialId?: string
}

export function useLocations() {
  return useQuery<ExecutionLocation[]>({
    queryKey: qk.locations,
    queryFn: async () => {
      return rpcUnwrapList<ExecutionLocation>('location.list', {}, 'locations')
    },
    refetchInterval: 5000,
    retry: false,
  })
}

export function useCreateLocation() {
  const queryClient = useQueryClient()
  return useMutation<ExecutionLocation, Error, { kind: string; label: string; address?: LocationAddress; auth?: LocationAuthInput }>({
    mutationFn: async (params) => {
      return rpcUnwrap<ExecutionLocation>('location.create', params, 'location')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.locations })
    },
  })
}

export function useProbeLocation() {
  const queryClient = useQueryClient()
  return useMutation<ExecutionLocation, Error, string>({
    mutationFn: async (locationId) => {
      return rpcUnwrap<ExecutionLocation>('location.probe', { locationId }, 'location')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.locations })
    },
  })
}

export function useSetLocationGitDiff() {
  const queryClient = useQueryClient()
  return useMutation<ExecutionLocation, Error, { locationId: string; gitDiffEnabled: boolean }>({
    mutationFn: async ({ locationId, gitDiffEnabled }) => {
      return rpcUnwrap<ExecutionLocation>('location.update', { locationId, gitDiffEnabled }, 'location')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.locations })
    },
  })
}

export function useDeleteLocation() {
  const queryClient = useQueryClient()
  return useMutation<void, Error, string>({
    mutationFn: async (locationId) => {
      await rpcCall<Record<string, never>>('location.delete', { locationId })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.locations })
    },
  })
}
