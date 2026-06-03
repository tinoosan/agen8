import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  AlertCircle,
  Check,
  ChevronDown,
  MoreHorizontal,
  Pencil,
  Plus,
  RotateCw,
  Search,
  Trash2,
  User,
} from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { rpcCall } from '../../lib/rpc'
import type { SpaceMember, Task } from '../../lib/types'
import {
  effectiveStatus,
  getAttemptCount,
  getLatestReview,
  isSystemTask,
  relativeTime,
  taskIdShort,
} from '../../pages/boardHelpers'
import { resolveMemberIdentity } from '../../lib/memberIdentity'
import { useSpaceMemberList } from '../../hooks/useSpace'
import { useCancelTask } from '../../hooks/useTasks'
import TaskFormDialog from './TaskFormDialog'
import ConfirmationDialog from '../ConfirmationDialog'

interface SpaceBoardTabProps {
  spaceId: string
  initialTaskId?: string
  onOpenTask?: (task: Task, status: BoardStatus) => void
}

type BoardStatus = 'pending' | 'blocked' | 'active' | 'in_review' | 'succeeded' | 'failed' | 'canceled'
type ColumnId = 'waiting' | 'active' | 'done' | 'failed'

interface StatusColumn {
  id: ColumnId
  label: string
  statuses: BoardStatus[]
  emptyText: string
}

/** Four status columns, collapsed from the 7 individual task statuses. */
const STATUS_COLUMNS: StatusColumn[] = [
  { id: 'waiting', label: 'Waiting', statuses: ['pending', 'blocked'], emptyText: 'Nothing queued' },
  { id: 'active', label: 'Active', statuses: ['active', 'in_review'], emptyText: 'Nothing active' },
  { id: 'done', label: 'Done', statuses: ['succeeded'], emptyText: 'Nothing done yet' },
  { id: 'failed', label: 'Failed', statuses: ['failed', 'canceled'], emptyText: 'No failures' },
]

/** Done/Failed columns collapse to this many cards until expanded. */
const COLLAPSED_LIMIT = 3

/** Member filter sentinel values that aren't member ids. */
const FILTER_ALL = 'all'
const FILTER_UNASSIGNED = 'unassigned'

interface BoardTask {
  task: Task
  status: BoardStatus
}

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

/** Recency sort key — newest activity first within a column. */
function recencyKey(task: Task): number {
  const ts = task.completedAt || task.startedAt || task.updatedAt || task.createdAt
  return ts ? new Date(ts).getTime() : 0
}

/* ── Assignee chip (member moves onto the card) ─────── */

function AssigneeChip({ task, member }: { task: Task; member?: SpaceMember }) {
  const memberId = (task.assignedTo ?? '').trim()
  const known = !!member
  const identity = resolveMemberIdentity(known ? memberId : null)
  const Icon = known ? identity.icon : User
  const color = known ? identity.colorVar : 'var(--text-3)'
  const name = known
    ? member.displayName || member.memberType || 'Member'
    : (task.assignedToLabel ?? '').trim() || 'Unassigned'

  return (
    <span className="kb-assignee">
      <span
        className="kb-av"
        style={{ backgroundColor: `color-mix(in srgb, ${color} 18%, transparent)`, color }}
      >
        <Icon size={10} strokeWidth={2} aria-hidden />
      </span>
      <span className="kb-assignee-name">{name}</span>
    </span>
  )
}

/* ── Task card ──────────────────────────────────────── */

function KanbanCard({
  task,
  columnId,
  selected,
  member,
  onClick,
  onEdit,
  onDelete,
}: {
  task: Task
  columnId: ColumnId
  selected: boolean
  member?: SpaceMember
  onClick: () => void
  onEdit: () => void
  onDelete: () => void
}) {
  const title = taskTitle(task)
  const kind = (task.taskKind ?? '').trim()
  const attempts = getAttemptCount(task)
  const approved = getLatestReview(task)?.decision === 'approved'

  // Root is a div (not a button) so the actions menu can nest a real button
  // without invalid button-in-button markup. Keyboard activation only fires
  // when the card itself is focused, not a child control.
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => {
        if ((e.key === 'Enter' || e.key === ' ') && e.target === e.currentTarget) {
          e.preventDefault()
          onClick()
        }
      }}
      className={cn(
        'kb-card',
        columnId === 'failed' && 'kb-card-failed',
        columnId === 'done' && 'kb-card-done',
        selected && 'kb-card-selected',
      )}
      data-testid={`board-card-${task.id}`}
    >
      <span className="kb-card-head">
        {kind && <span className="kb-kind">{kind}</span>}
        <span className="kb-card-id">{taskIdShort(task.id)}</span>
        <span className="kb-card-time">{relativeTime(task.completedAt || task.createdAt)}</span>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="kb-card-menu"
              aria-label="Task actions"
              data-testid={`board-card-menu-${task.id}`}
              onClick={(e) => e.stopPropagation()}
            >
              <MoreHorizontal size={13} aria-hidden />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="text-xs">
            <DropdownMenuItem onSelect={onEdit}>
              <Pencil size={12} aria-hidden /> Edit
            </DropdownMenuItem>
            <DropdownMenuItem className="kb-menu-danger" onSelect={onDelete}>
              <Trash2 size={12} aria-hidden /> Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </span>
      <span className="kb-card-title">{title}</span>
      <span className="kb-card-foot">
        <AssigneeChip task={task} member={member} />
        {(attempts > 1 || approved) && (
          <span className="kb-badges">
            {attempts > 1 && (
              <span className="kb-badge kb-badge-retry" title={`${attempts} attempts`}>
                <RotateCw size={9} aria-hidden />
                {attempts}
              </span>
            )}
            {approved && (
              <span className="kb-badge kb-badge-review" title="Approved">
                <Check size={9} aria-hidden />
              </span>
            )}
          </span>
        )}
      </span>
    </div>
  )
}

/* ── Status column ──────────────────────────────────── */

function KanbanColumn({
  col,
  items,
  expanded,
  onToggleExpand,
  selectedTaskId,
  memberById,
  onOpen,
  onEdit,
  onDelete,
}: {
  col: StatusColumn
  items: BoardTask[]
  expanded: boolean
  onToggleExpand: () => void
  selectedTaskId: string | null
  memberById: Map<string, SpaceMember>
  onOpen: (bt: BoardTask) => void
  onEdit: (task: Task) => void
  onDelete: (task: Task) => void
}) {
  const collapsible = col.id === 'done' || col.id === 'failed'
  const shown = collapsible && !expanded ? items.slice(0, COLLAPSED_LIMIT) : items
  const hidden = items.length - shown.length

  return (
    <section className={cn('kb-col', `kb-col-${col.id}`)} data-testid={`board-col-${col.id}`}>
      <header className="kb-col-head">
        <span className="kb-col-dot" aria-hidden />
        <span className="kb-col-label">{col.label}</span>
        <span className="kb-col-count">{items.length}</span>
      </header>
      {items.length === 0 ? (
        <p className="kb-col-empty">{col.emptyText}</p>
      ) : (
        <>
          {shown.map((bt) => (
            <KanbanCard
              key={bt.task.id}
              task={bt.task}
              columnId={col.id}
              selected={bt.task.id === selectedTaskId}
              member={memberById.get((bt.task.assignedTo ?? '').trim())}
              onClick={() => onOpen(bt)}
              onEdit={() => onEdit(bt.task)}
              onDelete={() => onDelete(bt.task)}
            />
          ))}
          {collapsible && hidden > 0 && (
            <button type="button" className="kb-more" onClick={onToggleExpand}>
              +{hidden} more {col.label.toLowerCase()}
            </button>
          )}
          {collapsible && expanded && items.length > COLLAPSED_LIMIT && (
            <button type="button" className="kb-more" onClick={onToggleExpand}>
              Show less
            </button>
          )}
        </>
      )}
    </section>
  )
}

/* ── Board ──────────────────────────────────────────── */

export default function SpaceBoardTab({ spaceId, initialTaskId, onOpenTask }: SpaceBoardTabProps) {
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [memberFilter, setMemberFilter] = useState<string>(FILTER_ALL)
  const [expanded, setExpanded] = useState<{ done: boolean; failed: boolean }>({
    done: false,
    failed: false,
  })
  const [dialog, setDialog] = useState<{ mode: 'create' } | { mode: 'edit'; task: Task } | null>(
    null,
  )
  const [cancelTarget, setCancelTarget] = useState<Task | null>(null)
  const cancelTask = useCancelTask(spaceId)
  const openedInitialTaskIdRef = useRef<string | null>(null)

  const {
    data: tasks,
    isLoading: tasksLoading,
    error: tasksError,
  } = useQuery<Task[]>({
    queryKey: ['task.list', spaceId],
    queryFn: async () => {
      const response = await rpcCall<{ tasks: Task[] }>('task.list', { spaceId, limit: 200 })
      return response.tasks ?? []
    },
    enabled: !!spaceId,
    refetchInterval: 10000,
    retry: false,
  })

  const membersQuery = useSpaceMemberList({ spaceId, enabled: !!spaceId })
  const members = useMemo(() => membersQuery.data ?? [], [membersQuery.data])
  const memberById = useMemo(() => new Map(members.map((m) => [m.id, m])), [members])

  const baseTasks = useMemo(
    () => (tasks ?? []).filter((task) => !isSystemTask(task)),
    [tasks],
  )

  const hasUnassigned = useMemo(
    () =>
      baseTasks.some((task) => {
        const assignee = (task.assignedTo ?? '').trim()
        return assignee === '' || !memberById.has(assignee)
      }),
    [baseTasks, memberById],
  )

  const filteredTasks = useMemo(() => {
    const query = searchQuery.trim().toLowerCase()
    return baseTasks.filter((task) => {
      const assignee = (task.assignedTo ?? '').trim()
      const known = assignee !== '' && memberById.has(assignee)
      if (memberFilter === FILTER_UNASSIGNED) {
        if (known) return false
      } else if (memberFilter !== FILTER_ALL && assignee !== memberFilter) {
        return false
      }
      if (query) {
        const haystack = `${task.title ?? ''} ${task.description ?? ''} ${task.id}`.toLowerCase()
        if (!haystack.includes(query)) return false
      }
      return true
    })
  }, [baseTasks, memberById, memberFilter, searchQuery])

  const buckets = useMemo(() => {
    const out: Record<ColumnId, BoardTask[]> = { waiting: [], active: [], done: [], failed: [] }
    for (const task of filteredTasks) {
      const status = statusForTask(task)
      const col = STATUS_COLUMNS.find((c) => c.statuses.includes(status)) ?? STATUS_COLUMNS[0]
      out[col.id].push({ task, status })
    }
    for (const id of Object.keys(out) as ColumnId[]) {
      out[id].sort((a, b) => recencyKey(b.task) - recencyKey(a.task))
    }
    return out
  }, [filteredTasks])

  const isLoading = tasksLoading || membersQuery.isLoading
  const error = tasksError || membersQuery.error

  useEffect(() => {
    if (!initialTaskId) {
      openedInitialTaskIdRef.current = null
      return
    }
    if (openedInitialTaskIdRef.current === initialTaskId) return
    const task = baseTasks.find((candidate) => candidate.id === initialTaskId)
    if (!task) return
    openedInitialTaskIdRef.current = initialTaskId
    setSelectedTaskId(task.id)
    onOpenTask?.(task, statusForTask(task))
  }, [initialTaskId, onOpenTask, baseTasks])

  const openCard = (bt: BoardTask) => {
    setSelectedTaskId(bt.task.id)
    onOpenTask?.(bt.task, bt.status)
  }

  const openCreate = () => setDialog({ mode: 'create' })
  const openEdit = (task: Task) => setDialog({ mode: 'edit', task })
  const openDelete = (task: Task) => setCancelTarget(task)

  const handleConfirmDelete = async () => {
    if (!cancelTarget) return
    try {
      await cancelTask.mutateAsync({ taskId: cancelTarget.id, reason: 'Canceled from board' })
      toast.success('Task deleted')
      setCancelTarget(null)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete task')
    }
  }

  const dialogs = (
    <>
      <TaskFormDialog
        spaceId={spaceId}
        open={dialog !== null}
        onOpenChange={(open) => {
          if (!open) setDialog(null)
        }}
        members={members}
        mode={dialog?.mode ?? 'create'}
        task={dialog?.mode === 'edit' ? dialog.task : null}
      />
      <ConfirmationDialog
        open={cancelTarget !== null}
        title="Delete task"
        message={
          cancelTarget
            ? `This cancels "${taskTitle(cancelTarget)}" and moves it to the Failed column. This can't be undone.`
            : ''
        }
        confirmLabel="Delete"
        tone="danger"
        busy={cancelTask.isPending}
        onClose={() => setCancelTarget(null)}
        onConfirm={handleConfirmDelete}
      />
    </>
  )

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

  if (baseTasks.length === 0) {
    return (
      <div className="space-board-tab">
        <div className="space-board-main">
          <div className="kb-empty">
            <p>No tasks in this space yet.</p>
            <button
              type="button"
              className="kb-new-task"
              onClick={openCreate}
              data-testid="board-new-task"
            >
              <Plus size={13} aria-hidden /> New task
            </button>
          </div>
        </div>
        {dialogs}
      </div>
    )
  }

  const memberLabel =
    memberFilter === FILTER_ALL
      ? 'All members'
      : memberFilter === FILTER_UNASSIGNED
        ? 'Unassigned'
        : memberById.get(memberFilter)?.displayName || 'Member'

  return (
    <div className="space-board-tab">
      <div className="space-board-main">
        <div className="kb-toolbar">
          <div className="kb-search">
            <Search size={13} aria-hidden />
            <input
              type="text"
              placeholder="Search tasks…"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              aria-label="Search tasks"
            />
          </div>
          <span className="kb-pill kb-pill-static">Group: Status</span>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button type="button" className="kb-pill kb-pill-btn" data-testid="board-member-filter">
                {memberLabel}
                <ChevronDown size={12} aria-hidden />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="text-xs">
              <DropdownMenuRadioGroup value={memberFilter} onValueChange={setMemberFilter}>
                <DropdownMenuRadioItem value={FILTER_ALL}>All members</DropdownMenuRadioItem>
                {members.map((m) => (
                  <DropdownMenuRadioItem key={m.id} value={m.id}>
                    {m.displayName || m.memberType || 'Member'}
                  </DropdownMenuRadioItem>
                ))}
                {hasUnassigned && (
                  <DropdownMenuRadioItem value={FILTER_UNASSIGNED}>Unassigned</DropdownMenuRadioItem>
                )}
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
          <div className="kb-spacer" />
          <div className="kb-stats">
            <span>
              <b>{buckets.active.length}</b> active
            </span>
            <span>
              <b>{buckets.done.length}</b> done
            </span>
            <span>
              <b>{buckets.failed.length}</b> failed
            </span>
          </div>
          <button
            type="button"
            className="kb-new-task"
            onClick={openCreate}
            data-testid="board-new-task"
          >
            <Plus size={13} aria-hidden /> New task
          </button>
        </div>

        {filteredTasks.length === 0 ? (
          <div className="kb-no-match">
            <span>No tasks match your filters.</span>
            <button
              type="button"
              className="kb-clear"
              onClick={() => {
                setSearchQuery('')
                setMemberFilter(FILTER_ALL)
              }}
            >
              Clear filters
            </button>
          </div>
        ) : (
          <div className="kb-board">
            {STATUS_COLUMNS.map((col) => (
              <KanbanColumn
                key={col.id}
                col={col}
                items={buckets[col.id]}
                expanded={col.id === 'done' ? expanded.done : col.id === 'failed' ? expanded.failed : false}
                onToggleExpand={() => {
                  if (col.id === 'done') setExpanded((s) => ({ ...s, done: !s.done }))
                  else if (col.id === 'failed') setExpanded((s) => ({ ...s, failed: !s.failed }))
                }}
                selectedTaskId={selectedTaskId}
                memberById={memberById}
                onOpen={openCard}
                onEdit={openEdit}
                onDelete={openDelete}
              />
            ))}
          </div>
        )}
      </div>
      {dialogs}
    </div>
  )
}
