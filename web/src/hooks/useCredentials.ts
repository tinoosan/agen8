import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import { qk } from '../lib/queryKeys'
import type { CredentialKind, CredentialStatus, CredentialView, RpcList } from '../lib/types'

interface CredentialResult {
  credential: CredentialView
}

export interface CredentialListParams {
  kind?: CredentialKind
  status?: CredentialStatus
}

export interface CredentialCreateParams {
  kind: CredentialKind
  label: string
  storageKind?: 'local_encrypted' | 'ssh_agent'
  secrets: Record<string, string>
}

export interface CredentialUpdateParams {
  credentialId: string
  label?: string
  status?: CredentialStatus
  storageKind?: 'local_encrypted' | 'ssh_agent'
  secrets?: Record<string, string>
}

export interface CredentialDeleteParams {
  credentialId: string
}

export function useCredentials(params: CredentialListParams = {}) {
  return useQuery<CredentialView[]>({
    queryKey: qk.credentials(params),
    queryFn: async () => {
      const result = await rpcCall<RpcList<'credentials', CredentialView>>('credential.list', params)
      return result.credentials ?? []
    },
    refetchInterval: 5000,
  })
}

export function useCredentialCreate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (params: CredentialCreateParams) =>
      rpcCall<CredentialResult>('credential.create', params),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.credentialsAll })
    },
  })
}

export function useCredentialUpdate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (params: CredentialUpdateParams) =>
      rpcCall<CredentialResult>('credential.update', params),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.credentialsAll })
    },
  })
}

export async function fetchCredential(credentialId: string): Promise<CredentialView> {
  const result = await rpcCall<CredentialResult>('credential.get', { credentialId })
  return result.credential
}

export function useCredentialDelete() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (params: CredentialDeleteParams) =>
      rpcCall('credential.delete', params),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.credentialsAll })
    },
  })
}
