import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { ProjectTeamSummary } from '../lib/types'

export function useProjectTeams() {
  return useQuery<ProjectTeamSummary[]>({
    queryKey: ['project.listTeams'],
    queryFn: async () => {
      const res = await rpcCall<{ teams: ProjectTeamSummary[] }>(
        'project.listTeams',
        {}
      )
      return res.teams ?? []
    },
    refetchInterval: 2000,
    retry: false,
  })
}

export function useProjectContext() {
  return useQuery({
    queryKey: ['project.getContext'],
    queryFn: () =>
      rpcCall<{ context: { config: { projectId: string }; state: { activeTeamId: string } } }>(
        'project.getContext',
        {}
      ),
    refetchInterval: 5000,
    retry: false,
  })
}
