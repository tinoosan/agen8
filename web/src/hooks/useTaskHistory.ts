import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { Task } from '../lib/types'

interface TaskListResult {
  tasks: Task[]
  totalCount?: number
}

interface UseTaskHistoryOptions {
  threadId: string | null
  teamId: string | null
  limit?: number
}

export function useTaskHistory({ threadId, teamId, limit = 500 }: UseTaskHistoryOptions) {
  return useQuery<Task[]>({
    queryKey: ['task.list.history', threadId, teamId ?? null, limit],
    queryFn: async () => {
      const res = await rpcCall<TaskListResult>('task.list', {
        threadId,
        teamId: teamId ?? undefined,
        view: 'outbox',
        scope: 'team',
        limit,
      })
      return res.tasks ?? []
    },
    enabled: !!threadId && !!teamId,
    refetchInterval: 2000,
    staleTime: 1000,
    retry: false,
  })
}
