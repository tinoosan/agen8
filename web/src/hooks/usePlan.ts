import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { PlanGetResult } from '../lib/types'

const DETACHED = 'detached-control'

export function usePlan(teamId: string | null, threadId: string | null) {
  return useQuery<PlanGetResult>({
    queryKey: ['plan.get', teamId, threadId],
    queryFn: () =>
      rpcCall<PlanGetResult>('plan.get', { threadId: threadId ?? DETACHED, teamId, aggregateTeam: true }),
    enabled: !!teamId,
    refetchInterval: 5000,
    staleTime: 3000,
    retry: false,
  })
}
