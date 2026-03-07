import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { ModelListResult } from '../lib/types'

const DETACHED = 'detached-control'

export function useModelList(threadId: string | null, provider?: string, query?: string) {
  return useQuery<ModelListResult>({
    queryKey: ['model.list', threadId, provider, query],
    queryFn: () =>
      rpcCall<ModelListResult>('model.list', { threadId: threadId ?? DETACHED, provider, query }),
    enabled: !!threadId,
    staleTime: 30000,
    retry: false,
  })
}
