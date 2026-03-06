import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { RuntimeGetSessionStateResult } from '../lib/types'

export function useRuntimeState(sessionId: string) {
    return useQuery<RuntimeGetSessionStateResult>({
        queryKey: ['runtimeState', sessionId],
        queryFn: () =>
            rpcCall<RuntimeGetSessionStateResult>('runtime.getSessionState', { sessionId }),
        refetchInterval: 1000,
        enabled: !!sessionId,
    })
}
