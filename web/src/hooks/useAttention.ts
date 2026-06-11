import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import { qk } from '../lib/queryKeys'

/** One harness session currently waiting on the human (see attention.list). */
export interface AttentionEntry {
  sessionRef: string
  projectId?: string
  memberId?: string
  memberName?: string
  harness?: string
  kind: 'waiting' | 'needs_approval'
  message?: string
  since: string
  updatedAt: string
}

/**
 * Live list of harness sessions waiting on the human. Refreshed by the
 * `attention.*` SSE rule in useRealtimeSync; the staleTime is a fallback for
 * missed events (the backend state is ephemeral and TTL'd anyway).
 */
export function useAttention(projectId: string | null) {
  return useQuery<AttentionEntry[]>({
    queryKey: qk.attention(projectId),
    queryFn: async () => {
      const res = await rpcCall<{ entries: AttentionEntry[] }>('attention.list', { projectId })
      return res.entries ?? []
    },
    enabled: !!projectId,
    staleTime: 30_000,
    refetchInterval: 60_000,
  })
}
