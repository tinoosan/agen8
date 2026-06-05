import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall, onNotification } from '../lib/rpc'
import type { Task } from '../lib/types'
import { normalizeTaskMembers } from '../lib/taskMembers'

interface TaskListResult {
  tasks: Task[]
  totalCount?: number
}

export function useProjectTasks(projectId: string | null) {
  return useQuery<Task[]>({
    queryKey: ['project.tasks.board', projectId ?? ''],
    queryFn: async () => {
      const result = await rpcCall<TaskListResult>('task.list', {
        projectId,
        limit: 500,
        sortBy: 'updated_at',
        sortDesc: true,
      })
      return (result.tasks ?? []).map(normalizeTaskMembers)
    },
    enabled: !!projectId,
    refetchInterval: 10_000,
    staleTime: 2000,
  })
}

// Task-related SSE event types that indicate a board state change.
const TASK_EVENT_PREFIXES = ['task.']

/**
 * Subscribes to SSE event.append notifications and invalidates the board task
 * query when a task-related event arrives. Drops polling interval from 3s to
 * 10s since SSE handles near-real-time delivery.
 *
 * Keeps project task views fresh without tying the board to spaces.
 */
export function useProjectTasksSSE() {
  const queryClient = useQueryClient()
  useEffect(() => {
    const unsub = onNotification('event.append', (notif: Record<string, unknown>) => {
      const event = notif?.event as Record<string, unknown> | undefined
      const type = (event?.type as string) ?? ''
      const isTaskEvent = TASK_EVENT_PREFIXES.some(prefix => type.startsWith(prefix))
      if (isTaskEvent) {
        queryClient.invalidateQueries({ queryKey: ['project.tasks.board'] })
      }
    })
    return unsub
  }, [queryClient])
}
