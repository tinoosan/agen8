import { useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { rpcCall, onNotification } from '../lib/rpc'
import { qk } from '../lib/queryKeys'
import type { Task, AcceptanceCriterion, RpcList } from '../lib/types'
import { normalizeTaskMembers } from '../lib/taskMembers'

export function useProjectTasks(projectId: string | null) {
  return useQuery<Task[]>({
    queryKey: qk.tasksBoard(projectId),
    queryFn: async () => {
      const result = await rpcCall<RpcList<'tasks', Task> & { totalCount?: number }>('task.list', {
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

// Fetches a single task by id for the routed detail page.
export function useTask(taskId: string | null) {
  return useQuery<Task>({
    queryKey: qk.taskGet(taskId),
    queryFn: async () => {
      const result = await rpcCall<{ task: Task }>('task.get', { taskId })
      return normalizeTaskMembers(result.task)
    },
    enabled: !!taskId,
    refetchInterval: 10_000,
  })
}

/* ── Task mutation hooks ─────────────────────────────────────────────────
 *
 * All task mutations invalidate ['project.tasks.board'] (prefix match, every
 * project) plus the affected ['task.get', id] so the dashboard panel and the
 * routed detail page stay consistent after any write.
 *
 * ──────────────────────────────────────────────────────────────────────── */

type CreateTaskInput = {
  projectId: string
  assignedTo: string
  description: string
  title?: string
  acceptanceCriteria?: string[]
  taskKind?: string
  keyResultRef?: string
  missionRef?: string
}

export function useCreateTask() {
  const queryClient = useQueryClient()
  return useMutation<{ task: Task }, Error, CreateTaskInput>({
    mutationFn: (params) => rpcCall<{ task: Task }>('task.create', params),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: qk.tasksBoardAll })
    },
  })
}

type UpdateTaskInput = {
  taskId: string
  title?: string
  description?: string
  acceptanceCriteria?: AcceptanceCriterion[]
  taskKind?: string
  keyResultRef?: string
}

export function useUpdateTask() {
  const queryClient = useQueryClient()
  return useMutation<{ task: Task }, Error, UpdateTaskInput>({
    mutationFn: (params) => rpcCall<{ task: Task }>('task.update', params),
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: qk.tasksBoardAll })
      queryClient.invalidateQueries({ queryKey: qk.taskGet(vars.taskId) })
    },
  })
}

export function useCancelTask() {
  const queryClient = useQueryClient()
  return useMutation<{ task: Task }, Error, { taskId: string; reason: string }>({
    mutationFn: (params) => rpcCall<{ task: Task }>('task.cancel', params),
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: qk.tasksBoardAll })
      queryClient.invalidateQueries({ queryKey: qk.taskGet(vars.taskId) })
    },
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
        queryClient.invalidateQueries({ queryKey: qk.tasksBoardAll })
      }
    })
    return unsub
  }, [queryClient])
}
