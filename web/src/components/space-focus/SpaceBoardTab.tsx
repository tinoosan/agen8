import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertCircle } from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { rpcCall } from '../../lib/rpc'
import type { Task } from '../../lib/types'
import { effectiveStatus, isSystemTask, relativeTime, taskIdShort } from '../../pages/boardHelpers'
import { resolveMemberIdentity } from '../../lib/memberIdentity'
import { useSpaceMemberList } from '../../hooks/useSpace'

interface SpaceBoardTabProps {
  spaceId: string
  initialTaskId?: string
  onOpenTask?: (task: Task, status: BoardStatus) => void
}

type BoardStatus = 'pending' | 'blocked' | 'active' | 'in_review' | 'succeeded' | 'failed' | 'canceled'

/** Swimlane status groups — collapsed from 7 individual statuses to 4 meaningful buckets. */
interface SwimColumn {
  id: string
  label: string
  statuses: BoardStatus[]
  dotColor: string
}

const SWIM_COLUMNS: SwimColumn[] = [
  { id: 'waiting', label: 'Waiting', statuses: ['pending', 'blocked'], dotColor: 'var(--text-3)' },
  { id: 'active', label: 'Active', statuses: ['active', 'in_review'], dotColor: 'var(--blue)' },
  { id: 'done', label: 'Done', statuses: ['succeeded'], dotColor: 'var(--green)' },
  { id: 'failed', label: 'Failed', statuses: ['failed', 'canceled'], dotColor: 'var(--red)' },
]

function taskTitle(task: Task): string {
  const title = task.title?.trim()
  if (title) return title
  const description = task.description?.trim()
  if (description) return description.split('\n')[0] ?? description
  return 'Untitled task'
}

function statusForTask(task: Task): BoardStatus {
  return effectiveStatus(task) as BoardStatus
}

function memberTypeLabel(memberType: string): string {
  const mt = (memberType ?? '').toLowerCase()
  if (mt === 'coordinator') return 'Coord'
  if (mt === 'worker') return 'Worker'
  return memberType.replace(/_/g, ' ')
}

/** Compact task chip for swimlane cells. */
function SwimTaskCard({
  task,
  status,
  selected,
  onClick,
}: {
  task: Task
  status: BoardStatus
  selected: boolean
  onClick: () => void
}) {
  const title = taskTitle(task)
  const isBlocked = status === 'blocked'
  const isReview = status === 'in_review'
  const isDone = status === 'succeeded'

  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'swim-task',
        isBlocked && 'swim-task-blocked',
        isReview && 'swim-task-review',
        isDone && 'swim-task-done',
        selected && 'swim-task-selected',
      )}
    >
      <span className="swim-task-head">
        <span className="swim-task-id">{taskIdShort(task.id)}</span>
        <span className="swim-task-time">{relativeTime(task.completedAt || task.createdAt)}</span>
        {isBlocked && <span className="swim-badge-blocked">Blocked</span>}
        {isReview && <span className="swim-badge-review">Review</span>}
      </span>
      <span className="swim-task-title">{title}</span>
    </button>
  )
}

interface AgentRow {
  memberId: string
  displayName: string
  memberType: string
  /** Tasks bucketed by swim column id */
  buckets: Map<string, Array<{ task: Task; status: BoardStatus }>>
}

export default function SpaceBoardTab({ spaceId, initialTaskId, onOpenTask }: SpaceBoardTabProps) {
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)
  const openedInitialTaskIdRef = useRef<string | null>(null)

  const {
    data: tasks,
    isLoading: tasksLoading,
    error: tasksError,
  } = useQuery<Task[]>({
    queryKey: ['task.list', spaceId],
    queryFn: async () => {
      const response = await rpcCall<{ tasks: Task[] }>('task.list', {
        spaceId,
        limit: 200,
      })
      return response.tasks ?? []
    },
    enabled: !!spaceId,
    refetchInterval: 10000,
    retry: false,
  })

  const membersQuery = useSpaceMemberList({ spaceId, enabled: !!spaceId })
  const members = useMemo(() => membersQuery.data ?? [], [membersQuery.data])

  const visibleTasks = useMemo(() => (tasks ?? []).filter((task) => !isSystemTask(task)), [tasks])

  /** Build one AgentRow per member, plus an "Unassigned" catch-all. */
  const agentRows = useMemo(() => {
    // Initialize a row per member, keyed by member ID so task.assignedTo lookups match
    const rowMap = new Map<string, AgentRow>()
    for (const m of members) {
      const buckets = new Map<string, Array<{ task: Task; status: BoardStatus }>>()
      for (const col of SWIM_COLUMNS) buckets.set(col.id, [])
      rowMap.set(m.id, {
        memberId: m.id,
        displayName: m.displayName,
        memberType: m.memberType,
        buckets,
      })
    }

    // Unassigned catch-all
    const unassignedBuckets = new Map<string, Array<{ task: Task; status: BoardStatus }>>()
    for (const col of SWIM_COLUMNS) unassignedBuckets.set(col.id, [])
    const unassignedRow: AgentRow = {
      memberId: '',
      displayName: 'Unassigned',
      memberType: '',
      buckets: unassignedBuckets,
    }

    // Place each task into the right agent row + status bucket
    for (const task of visibleTasks) {
      const status = statusForTask(task)
      const column = SWIM_COLUMNS.find((col) => col.statuses.includes(status)) ?? SWIM_COLUMNS[0]
      const assignee = task.assignedTo ?? ''
      const row = rowMap.get(assignee)
      if (row) {
        row.buckets.get(column.id)?.push({ task, status })
      } else {
        unassignedRow.buckets.get(column.id)?.push({ task, status })
      }
    }

    // Collect rows: members first, unassigned last (only if it has tasks)
    const rows = Array.from(rowMap.values())
    const unassignedHasTasks = Array.from(unassignedRow.buckets.values()).some((arr) => arr.length > 0)
    if (unassignedHasTasks) rows.push(unassignedRow)
    return rows
  }, [members, visibleTasks])

  const isLoading = tasksLoading || membersQuery.isLoading
  const error = tasksError || membersQuery.error

  useEffect(() => {
    if (!initialTaskId) {
      openedInitialTaskIdRef.current = null
      return
    }
    if (openedInitialTaskIdRef.current === initialTaskId) return
    const task = visibleTasks.find((candidate) => candidate.id === initialTaskId)
    if (!task) return
    const status = statusForTask(task)
    openedInitialTaskIdRef.current = initialTaskId
    setSelectedTaskId(task.id)
    onOpenTask?.(task, status)
  }, [initialTaskId, onOpenTask, visibleTasks])

  if (isLoading) {
    return (
      <div className="space-board-loading">
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-20 w-full" />
        <Skeleton className="h-20 w-full" />
        <Skeleton className="h-20 w-full" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="space-board-error">
        <AlertCircle className="h-5 w-5 text-destructive" />
        <p>Unable to load board.</p>
      </div>
    )
  }

  if (agentRows.length === 0) {
    return (
      <div className="space-board-tab">
        <div className="space-board-main">
          <div className="swim-empty-state">
            <p>No agents in this space yet.</p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="space-board-tab">
      <div className="space-board-main">
        <div className="swimlanes">
          {/* Header row */}
          <div className="swim-header-row">
            <div className="swim-header-label">Member</div>
            {SWIM_COLUMNS.map((col) => (
              <div key={col.id} className="swim-header-label swim-header-status">
                <span className="swim-header-dot" style={{ backgroundColor: col.dotColor }} />
                {col.label}
              </div>
            ))}
          </div>

          {/* Agent swimlanes */}
          {agentRows.map((row) => {
            const identity = resolveMemberIdentity(row.memberId || null)
            const Icon = identity.icon
            return (
              <div key={row.memberId || '_unassigned'} className="swim-lane">
                {/* Agent identity cell */}
                <div className="swim-agent">
                  <div
                    className="swim-agent-avatar"
                    style={{
                      backgroundColor: `color-mix(in srgb, ${identity.colorVar} 18%, transparent)`,
                      color: identity.colorVar,
                    }}
                  >
                    <Icon size={13} strokeWidth={2} />
                  </div>
                  <div className="swim-agent-info">
                    <span className="swim-agent-name">{row.displayName}</span>
                    {row.memberType && (
                      <span className="swim-agent-role">{memberTypeLabel(row.memberType)}</span>
                    )}
                  </div>
                </div>

                {/* Status cells */}
                {SWIM_COLUMNS.map((col) => {
                  const bucket = row.buckets.get(col.id) ?? []
                  return (
                    <div key={col.id} className="swim-cell">
                      {bucket.length > 0 && (
                        bucket.map(({ task, status }) => (
                          <SwimTaskCard
                            key={task.id}
                            task={task}
                            status={status}
                            selected={task.id === selectedTaskId}
                            onClick={() => {
                              setSelectedTaskId(task.id)
                              onOpenTask?.(task, status)
                            }}
                          />
                        ))
                      )}
                    </div>
                  )
                })}
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
