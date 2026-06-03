import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { SpaceTotals } from '../lib/types'

export function useSpaceTotals(spaceId: string | null) {
  return useQuery<SpaceTotals>({
    queryKey: ['space.getTotals', spaceId],
    queryFn: () => rpcCall<SpaceTotals>('space.getTotals', { spaceId }),
    enabled: !!spaceId,
    refetchInterval: 3000,
    retry: false,
  })
}
