import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { AgentListResult, RuntimeGetSessionStateResult, RuntimeRunState } from '../lib/types'

const DETACHED = 'detached-control'

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
  parentRunId?: string
  spawnIndex?: number
  startedAt?: string
}

export function useAgentList(sessionId: string | null) {
  return useQuery<EnrichedAgent[]>({
    queryKey: ['agent.list.enriched', sessionId],
    queryFn: async () => {
      const [agentRes, runtimeRes] = await Promise.all([
        rpcCall<AgentListResult>('agent.list', { threadId: DETACHED, sessionId }),
        sessionId
          ? rpcCall<RuntimeGetSessionStateResult>('runtime.getSessionState', { sessionId })
          : Promise.resolve({ sessionId: '', runs: [] as RuntimeRunState[] }),
      ])

      const runtimeByRun = new Map<string, RuntimeRunState>()
      for (const run of runtimeRes.runs ?? []) {
        runtimeByRun.set(run.runId, run)
      }

      return (agentRes.agents ?? []).map((agent): EnrichedAgent => {
        const rt = runtimeByRun.get(agent.runId)
        return {
          runId: agent.runId,
          role: agent.role || (agent.spawnIndex ? `Subagent-${agent.spawnIndex}` : agent.runId.slice(0, 12)),
          status: agent.status ?? 'unknown',
          effectiveStatus: rt?.effectiveStatus || agent.status || 'unknown',
          profile: agent.profile,
          model: rt?.model,
          workerPresent: rt?.workerPresent ?? false,
          runTotalTokens: rt?.runTotalTokens ?? 0,
          runTotalCostUSD: rt?.runTotalCostUSD ?? 0,
          parentRunId: agent.parentRunId,
          spawnIndex: agent.spawnIndex,
          startedAt: agent.createdAt,
        }
      })
    },
    enabled: !!sessionId,
    refetchInterval: 2000,
    retry: false,
  })
}
