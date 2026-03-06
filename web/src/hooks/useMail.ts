import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { Task } from '../lib/types'

interface TaskListParams {
  teamId?: string
  status?: string
  limit?: number
}

interface TaskListResult {
  tasks: Task[]
}

export function useMail(teamId: string | null) {
  const queryClient = useQueryClient()
  const key = ['task.list', teamId]

  const query = useQuery<Task[]>({
    queryKey: key,
    queryFn: async () => {
      const res = await rpcCall<TaskListResult>('task.list', {
        teamId: teamId ?? undefined,
        limit: 100,
      } satisfies TaskListParams)
      return res.tasks ?? []
    },
    enabled: !!teamId,
    refetchInterval: 3000,
    staleTime: 2000,
  })

  const claimMutation = useMutation({
    mutationFn: (taskId: string) =>
      rpcCall('task.claim', { taskId }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: key }),
  })

  const completeMutation = useMutation({
    mutationFn: ({ taskId, summary }: { taskId: string; summary?: string }) =>
      rpcCall('task.complete', { taskId, summary }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: key }),
  })

  const inbox = query.data?.filter(t => t.status === 'pending' || t.status === 'claimed') ?? []
  const outbox = query.data?.filter(t => t.status === 'done' || t.status === 'failed') ?? []
  const badgeCount = inbox.length

  return {
    query,
    inbox,
    outbox,
    badgeCount,
    claim: claimMutation.mutate,
    complete: completeMutation.mutate,
  }
}
