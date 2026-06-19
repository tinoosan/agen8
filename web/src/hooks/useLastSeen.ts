/**
 * useLastSeen — server-side last-seen marker per (user, project).
 *
 * get:      lastseen.get   → { seenAt: ISO string | "" }
 * markSeen: lastseen.markSeen → { seenAt: ISO string }
 *
 * The component reads the PREVIOUS seenAt (captured before calling markSeen)
 * to compute the diff, so opening the dashboard does not instantly blank the
 * summary. Dismiss → markSeen → invalidate → card re-reads empty diff.
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import { qk } from '../lib/queryKeys'

interface LastSeenResult {
  seenAt: string // "" when no marker exists
}

export function useLastSeen(projectId: string | null) {
  return useQuery<LastSeenResult>({
    queryKey: qk.lastSeen(projectId),
    queryFn: async () => {
      const res = await rpcCall<LastSeenResult>('lastseen.get', { projectId: projectId ?? '' })
      return res
    },
    enabled: !!projectId,
    // Last-seen only changes when the user dismisses the card; no need for
    // frequent polling. 0 staleTime means it re-fetches when the component
    // mounts if the cache was invalidated.
    refetchInterval: false,
    staleTime: 0,
  })
}

export function useMarkSeen(projectId: string | null) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      return rpcCall<LastSeenResult>('lastseen.markSeen', { projectId: projectId ?? '' })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: qk.lastSeen(projectId) })
    },
  })
}
