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

export function useAgentList(sessionIds: string[]) {
  const normalized = [...new Set(sessionIds.map((id) => id.trim()).filter(Boolean))].sort()

  return useQuery<EnrichedAgent[]>({
    queryKey: ['agent.list.enriched', normalized],
    queryFn: async () => {
      const results = await Promise.all(
        normalized.map(async (sessionId) => {
          const [agentRes, runtimeRes] = await Promise.all([
            rpcCall<AgentListResult>('agent.list', { threadId: DETACHED, sessionId }),
            rpcCall<RuntimeGetSessionStateResult>('runtime.getSessionState', { sessionId }),
          ])
          return { agentRes, runtimeRes }
        }),
      )

      const runtimeByRun = new Map<string, RuntimeRunState>()
      for (const result of results) {
        for (const run of result.runtimeRes.runs ?? []) {
          runtimeByRun.set(run.runId, run)
        }
      }

      const enrichedByRun = new Map<string, EnrichedAgent>()
      for (const result of results) {
        for (const agent of result.agentRes.agents ?? []) {
          const rt = runtimeByRun.get(agent.runId)
          enrichedByRun.set(agent.runId, {
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
          })
        }
      }

      return [...enrichedByRun.values()]
    },
    enabled: normalized.length > 0,
    refetchInterval: 2000,
    retry: false,
  })
}
