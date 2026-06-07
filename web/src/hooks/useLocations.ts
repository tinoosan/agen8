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

export function useInstallCodex() {
  const queryClient = useQueryClient()
  return useMutation<ExecutionLocation, Error, string>({
    mutationFn: async (locationId) => {
      return rpcUnwrap<ExecutionLocation>('location.installCodex', { locationId }, 'location')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.locations })
    },
  })
}

export function useInstallClaude() {
  const queryClient = useQueryClient()
  return useMutation<ExecutionLocation, Error, string>({
    mutationFn: async (locationId) => {
      return rpcUnwrap<ExecutionLocation>('location.installClaude', { locationId }, 'location')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.locations })
    },
  })
}

export interface ClaudeAuthStatus {
  loggedIn: boolean
  authMethod?: string
  provider?: string
  rawJson?: string
}

export interface CodexAuthStatus {
  loggedIn: boolean
  method?: string
  output?: string
}

export interface CodexLoginResult {
  output: string
  loginUrl?: string
  logPath?: string
  pid?: string
}

export interface ClaudeLoginResult {
  output: string
  loginUrl?: string
  logPath?: string
  pid?: string
}

export interface ClaudeLoginCompleteInput {
  locationId: string
  code: string
}

export function useClaudeAuthStatus() {
  return useMutation<ClaudeAuthStatus, Error, string>({
    mutationFn: async (locationId) => {
      return rpcCall<ClaudeAuthStatus>('location.claudeAuthStatus', { locationId })
    },
  })
}

export function useCodexAuthStatus() {
  return useMutation<CodexAuthStatus, Error, string>({
    mutationFn: async (locationId) => {
      return rpcCall<CodexAuthStatus>('location.codexAuthStatus', { locationId })
    },
  })
}

export function useCodexLogin() {
  return useMutation<CodexLoginResult, Error, string>({
    mutationFn: async (locationId) => {
      return rpcCall<CodexLoginResult>('location.codexLogin', { locationId })
    },
  })
}

export function useClaudeLogin() {
  return useMutation<ClaudeLoginResult, Error, string>({
    mutationFn: async (locationId) => {
      return rpcCall<ClaudeLoginResult>('location.claudeLogin', { locationId })
    },
  })
}

export function useClaudeLoginComplete() {
  return useMutation<ClaudeLoginResult, Error, ClaudeLoginCompleteInput>({
    mutationFn: async ({ locationId, code }) => {
      return rpcCall<ClaudeLoginResult>('location.claudeLoginComplete', { locationId, code })
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
