import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'

export type CredentialKind = 'ssh_agent' | 'ssh_key' | 'ssh_password' | 'api_key'
export type CredentialStatus = 'active' | 'disabled' | 'invalid'
export type CredentialFieldKind = 'public' | 'secret'

export interface CredentialFieldView {
  name: string
  kind: CredentialFieldKind
  configured: boolean
}

export interface CredentialView {
  id: string
  kind: CredentialKind
  label: string
  status: CredentialStatus
  fields?: CredentialFieldView[]
  createdAt?: string
  updatedAt?: string
}

interface CredentialListResult {
  credentials: CredentialView[]
}

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

export const CREDENTIALS_KEY = 'credential.list'

export function useCredentials(params: CredentialListParams = {}) {
  return useQuery<CredentialView[]>({
    queryKey: [CREDENTIALS_KEY, params],
    queryFn: async () => {
      const result = await rpcCall<CredentialListResult>('credential.list', params)
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
      void queryClient.invalidateQueries({ queryKey: [CREDENTIALS_KEY] })
    },
  })
}

export function useCredentialUpdate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (params: CredentialUpdateParams) =>
      rpcCall<CredentialResult>('credential.update', params),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: [CREDENTIALS_KEY] })
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
      void queryClient.invalidateQueries({ queryKey: [CREDENTIALS_KEY] })
    },
  })
}
