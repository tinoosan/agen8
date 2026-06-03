import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { SpaceMemberListResult } from '../lib/types'

export interface EnrichedAgent {
  runId: string
  role: string
  status: string
  effectiveStatus: string
  profile?: string
  model?: string
  workerPresent: boolean
  runTotalTokens: number
  runTotalCostUSD: number
  startedAt?: string
}

export function useAgentList(spaceIds: string[]) {
  const normalized = [...new Set(spaceIds.map((id) => id.trim()).filter(Boolean))].sort()

  return useQuery<EnrichedAgent[]>({
    queryKey: ['agent.list.enriched', normalized],
    queryFn: async () => {
      const results = await Promise.all(
        normalized.map((spaceId) =>
          rpcCall<SpaceMemberListResult>('space.member.list', { spaceId }),
        ),
      )

      const enrichedByRun = new Map<string, EnrichedAgent>()
      for (const result of results) {
        for (const member of result.members ?? []) {
          if ((member.lifecycleState ?? '').toLowerCase() === 'removed') continue
          const runId = member.currentRunId ?? member.id
          enrichedByRun.set(runId, {
            runId,
            role: member.displayName || member.memberType || 'Member',
            status: member.lifecycleState ?? 'unknown',
            effectiveStatus: member.lifecycleState ?? 'unknown',
            model: member.model,
            workerPresent: member.lifecycleState === 'active',
            runTotalTokens: 0,
            runTotalCostUSD: 0,
          })
        }
      }

      return [...enrichedByRun.values()]
    },
    enabled: normalized.length > 0,
    refetchInterval: 5000,
    retry: false,
  })
}
