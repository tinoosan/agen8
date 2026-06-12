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
  kind: 'waiting' | 'needs_approval' | 'asking'
  message?: string
  since: string
  updatedAt: string
}

/**
 * Grace period before a `waiting` entry is shown. Stop fires at EVERY turn
 * end, so during an active back-and-forth "waiting" reappears seconds after
 * each reply — a wait only deserves attention once it has actually lasted.
 * Approval prompts block the agent outright, so they show immediately.
 */
export const WAITING_GRACE_MS = 45_000

/** Entries worth surfacing right now: approvals always, waits past the grace period. */
export function displayableAttention(entries: AttentionEntry[], now = Date.now()): AttentionEntry[] {
  return entries.filter((entry) => {
    if (entry.kind !== 'waiting') return true
    return now - new Date(entry.since).getTime() >= WAITING_GRACE_MS
  })
}

/**
 * Live list of harness sessions waiting on the human. Refreshed by the
 * `attention.*` SSE rule in useRealtimeSync; the 15s poll is the fallback for
 * missed SSE events (daemon restarts drop the stream and SSE has no replay),
 * and doubles as the re-render tick that surfaces entries crossing the
 * grace period.
 */
export function useAttention(projectId: string | null) {
  return useQuery<AttentionEntry[]>({
    queryKey: qk.attention(projectId),
    queryFn: async () => {
      const res = await rpcCall<{ entries: AttentionEntry[] }>('attention.list', { projectId })
      return res.entries ?? []
    },
    enabled: !!projectId,
    staleTime: 10_000,
    refetchInterval: 15_000,
  })
}
