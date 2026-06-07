import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import { qk } from '../lib/queryKeys'
import type { Project } from '../lib/types'

export function useProjects(options?: { includeArchived?: boolean }) {
  const includeArchived = options?.includeArchived === true
  return useQuery<Project[]>({
    queryKey: qk.projects(includeArchived),
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
