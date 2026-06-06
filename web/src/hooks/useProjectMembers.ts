import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { ProjectMember } from '../lib/types'

// Lists the project's roster. Used by the task assignee picker, where
// task.create requires a valid project member.
export function useProjectMembers(projectId: string | null) {
  return useQuery<ProjectMember[]>({
    queryKey: ['project.member.list', projectId ?? ''],
    queryFn: async () => {
      const res = await rpcCall<{ members: ProjectMember[] }>('project.member.list', {
        projectId: projectId ?? '',
      })
      return res.members ?? []
    },
    enabled: !!projectId,
    refetchInterval: 30_000,
  })
}
