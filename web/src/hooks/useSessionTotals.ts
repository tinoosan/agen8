import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { SessionTotals } from '../lib/types'

const DETACHED = 'detached-control'

export function useSessionTotals(teamId: string | null) {
  return useQuery<SessionTotals>({
    queryKey: ['session.getTotals', teamId],
    queryFn: () =>
      rpcCall<SessionTotals>('session.getTotals', { threadId: DETACHED, teamId }),
    enabled: !!teamId,
    refetchInterval: 3000,
    retry: false,
  })
}
