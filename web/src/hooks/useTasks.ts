import { useMutation, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { Task } from '../lib/types'

interface TaskResult {
  task: Task
}

export interface CreateTaskInput {
  assignedTo: string
  description: string
  title?: string
  taskKind?: string
}

export interface UpdateTaskInput {
  taskId: string
  title?: string
  description?: string
  taskKind?: string
}

/**
 * Create a task in this space via `task.create`.
 *
 * The board's only task.list query is keyed by spaceId, so a single
 * invalidate refreshes every column after the new card lands.
 */
export function useCreateTask(spaceId: string) {
  const queryClient = useQueryClient()
  return useMutation<Task, Error, CreateTaskInput>({
    mutationFn: async (input) => {
      const res = await rpcCall<TaskResult>('task.create', { spaceId, ...input })
      return res.task
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['task.list', spaceId] })
    },
  })
}

/**
 * Edit a task's descriptive fields via `task.update`.
 *
 * task.update is a coordinator data edit — it cannot move a task between
 * columns or reassign it (those are lifecycle transitions), so this only
 * carries title/description/kind.
 */
export function useUpdateTask(spaceId: string) {
  const queryClient = useQueryClient()
  return useMutation<Task, Error, UpdateTaskInput>({
    mutationFn: async (input) => {
      const res = await rpcCall<TaskResult>('task.update', input)
      return res.task
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['task.list', spaceId] })
    },
  })
}

/**
 * Cancel a task via `task.cancel`. This is the board's "delete" — tasks are
 * never hard-deleted, only moved to the canceled terminal state with a reason.
 */
export function useCancelTask(spaceId: string) {
  const queryClient = useQueryClient()
  return useMutation<Task, Error, { taskId: string; reason: string }>({
    mutationFn: async (input) => {
      const res = await rpcCall<TaskResult>('task.cancel', input)
      return res.task
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['task.list', spaceId] })
    },
  })
}
