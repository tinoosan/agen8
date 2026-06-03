import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { Project } from '../lib/types'

export function useProjects(options?: { includeArchived?: boolean }) {
  const includeArchived = options?.includeArchived === true
  return useQuery<Project[]>({
    queryKey: ['project.list', includeArchived ? 'withArchived' : 'active'],
    queryFn: async () => {
      const res = await rpcCall<{ projects: Project[] }>(
        'project.list',
        {}
      )
      return res.projects ?? []
    },
    refetchInterval: 5000,
    retry: false,
    select: (projects) => includeArchived ? projects : projects.filter((project) => project.status !== 'archived'),
  })
}
