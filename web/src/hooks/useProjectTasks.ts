import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
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
    // SSE drives freshness (useRealtimeInvalidation in <App/>); this slow poll is
    // only a self-healing backstop for events missed during a disconnect.
    refetchInterval: 30_000,
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
    // SSE-backstop poll; see useProjectTasks above.
    refetchInterval: 30_000,
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

// Reassigns a task to another project member. The backend requeues a
// non-terminal task on reassignment (claim cleared, status back to pending), so
// the board and the routed detail page both need to refresh.
export function useAssignTask() {
  const queryClient = useQueryClient()
  return useMutation<{ task: Task }, Error, { taskId: string; assignedTo: string }>({
    mutationFn: (params) => rpcCall<{ task: Task }>('task.assign', params),
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: qk.tasksBoardAll })
      queryClient.invalidateQueries({ queryKey: qk.taskGet(vars.taskId) })
    },
  })
}
