import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { ExecutionLocation, LocationAddress } from '../lib/types'

export interface LocationAuthInput {
  mode?: string
  credentialId?: string
}

export function useLocations() {
  return useQuery<ExecutionLocation[]>({
    queryKey: ['location.list'],
    queryFn: async () => {
      const result = await rpcCall<{ locations: ExecutionLocation[] }>('location.list', {})
      return result.locations ?? []
    },
    refetchInterval: 5000,
    retry: false,
  })
}

export function useCreateLocation() {
  const queryClient = useQueryClient()
  return useMutation<ExecutionLocation, Error, { kind: string; label: string; address?: LocationAddress; auth?: LocationAuthInput }>({
    mutationFn: async (params) => {
      const result = await rpcCall<{ location: ExecutionLocation }>('location.create', params)
      return result.location
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['location.list'] })
    },
  })
}

export function useProbeLocation() {
  const queryClient = useQueryClient()
  return useMutation<ExecutionLocation, Error, string>({
    mutationFn: async (locationId) => {
      const result = await rpcCall<{ location: ExecutionLocation }>('location.probe', { locationId })
      return result.location
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['location.list'] })
    },
  })
}

export function useInstallCodex() {
  const queryClient = useQueryClient()
  return useMutation<ExecutionLocation, Error, string>({
    mutationFn: async (locationId) => {
      const result = await rpcCall<{ location: ExecutionLocation }>('location.installCodex', { locationId })
      return result.location
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['location.list'] })
    },
  })
}

export function useInstallClaude() {
  const queryClient = useQueryClient()
  return useMutation<ExecutionLocation, Error, string>({
    mutationFn: async (locationId) => {
      const result = await rpcCall<{ location: ExecutionLocation }>('location.installClaude', { locationId })
      return result.location
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['location.list'] })
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
      void queryClient.invalidateQueries({ queryKey: ['location.list'] })
    },
  })
}
