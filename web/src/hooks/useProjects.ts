import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { ProjectRegistrySummary } from '../lib/types'

export function useProjects() {
  return useQuery<ProjectRegistrySummary[]>({
    queryKey: ['project.listProjects'],
    queryFn: async () => {
      const res = await rpcCall<{ projects: ProjectRegistrySummary[] }>(
        'project.listProjects',
        {}
      )
      return res.projects ?? []
    },
    refetchInterval: 5000,
    retry: false,
  })
}
