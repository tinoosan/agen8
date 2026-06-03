import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall, onNotification } from '../lib/rpc'
import type { Task, ProjectSpaceSummary } from '../lib/types'
import { buildMemberLabelMap, resolveMemberLabel } from '../lib/memberLabels'

interface TaskListResult {
  tasks: Task[]
  totalCount?: number
}

export function useProjectTasks(spaces: ProjectSpaceSummary[]) {
  const spaceIds = [...new Set(spaces.map(space => space.spaceId).filter(Boolean))].sort()
  const memberLabels = buildMemberLabelMap(spaces)
  return useQuery<Task[]>({
    queryKey: ['project.tasks.board', spaceIds.join(',')],
    queryFn: async () => {
      const seen = new Map<string, Task>()
      const fetches = spaceIds.map(spaceId =>
        rpcCall<TaskListResult>('task.list', {
          spaceId,
          limit: 200,
        }),
      )
      const results = await Promise.all(fetches)
      for (const result of results) {
        for (const task of result.tasks ?? []) {
          // Dedupe: prefer newer version (by completedAt or createdAt)
          const existing = seen.get(task.id)
          if (!existing) {
            seen.set(task.id, task)
          } else {
            const existDate = existing.completedAt ?? existing.createdAt
            const newDate = task.completedAt ?? task.createdAt
            if ((newDate ?? '') > (existDate ?? '')) {
              seen.set(task.id, task)
            }
          }
        }
      }
      return Array.from(seen.values()).map(task => ({
        ...task,
        assignedToLabel: resolveMemberLabel(task.assignedTo, memberLabels),
      }))
    },
    enabled: spaceIds.length > 0,
    refetchInterval: 10_000,
    staleTime: 2000,
  })
}

// Task-related SSE event types that indicate a board state change.
const TASK_EVENT_PREFIXES = ['task.', 'oa.']

/**
 * Subscribes to SSE event.append notifications and invalidates the board task
 * query when a task-related event arrives. Drops polling interval from 3s to
 * 10s since SSE handles near-real-time delivery.
 *
 * Mirrors the pattern used by useOpActionSSE / useEscalationSSE.
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
