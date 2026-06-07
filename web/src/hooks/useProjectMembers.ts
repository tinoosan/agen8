import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall, rpcUnwrapList } from '../lib/rpc'
import { qk } from '../lib/queryKeys'
import type { ProjectMember } from '../lib/types'

// Lists the project's roster. Used by the task assignee picker, where
// task.create requires a valid project member.
export function useProjectMembers(projectId: string | null) {
  return useQuery<ProjectMember[]>({
    queryKey: qk.projectMembers(projectId),
    queryFn: async () => {
      return rpcUnwrapList<ProjectMember>('project.member.list', { projectId: projectId ?? '' }, 'members')
    },
    enabled: !!projectId,
    refetchInterval: 30_000,
  })
}

// Soft-removes a member (lifecycleState active → removed). The server keeps the
// record and everything it authored (decisions, tasks, graph history); it just
// drops off the active roster. There is no UI to restore, so callers confirm
// first. Invalidates every cached roster so the active/removed split refreshes.
export function useRemoveMember() {
  const queryClient = useQueryClient()
  return useMutation<{ member: ProjectMember }, Error, { memberId: string }>({
    mutationFn: (params) =>
      rpcCall<{ member: ProjectMember }>('project.member.remove', params),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: qk.projectMembersAll })
    },
  })
}
