import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type {
  RuntimeConfig,
  ConfigUpdateResult,
  ProjectSettings,
  ProjectConfigUpdateResult,
} from '../lib/types'

// ── Runtime config ──────────────────────────────────

export function useConfig() {
  return useQuery<RuntimeConfig>({
    queryKey: ['config.get'],
    queryFn: () => rpcCall<RuntimeConfig>('config.get'),
    staleTime: 30_000,
    retry: 1,
  })
}

export function useConfigUpdate() {
  const queryClient = useQueryClient()
  return useMutation<ConfigUpdateResult, Error, Partial<RuntimeConfig>>({
    mutationFn: (params) =>
      rpcCall<ConfigUpdateResult>('config.update', params),
    onSuccess: (data) => {
      queryClient.setQueryData(['config.get'], data.config)
    },
  })
}

// ── Project config ──────────────────────────────────

export function useProjectConfig(enabled = true) {
  return useQuery<ProjectSettings>({
    queryKey: ['config.getProject'],
    queryFn: () => rpcCall<ProjectSettings>('config.getProject'),
    staleTime: 30_000,
    retry: 1,
    enabled,
  })
}

export function useProjectConfigUpdate() {
  const queryClient = useQueryClient()
  return useMutation<
    ProjectConfigUpdateResult,
    Error,
    Partial<ProjectSettings>
  >({
    mutationFn: (params) =>
      rpcCall<ProjectConfigUpdateResult>('config.updateProject', params),
    onSuccess: (data) => {
      queryClient.setQueryData(['config.getProject'], data.config)
    },
  })
}
