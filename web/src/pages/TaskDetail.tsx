import { useState, type ReactNode } from 'react'
import { useRoute, Link } from 'wouter'
import { toast } from 'sonner'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  ArrowLeft,
  AlertCircle,
  AlertTriangle,
  Clock,
  Hash,
  Pencil,
  Ban,
  ChevronRight,
} from 'lucide-react'
import { useTask, useCancelTask } from '../hooks/useProjectTasks'
import { useRecentDecisions } from '../hooks/useDecisions'
import { useKeyResult, useProjectKRs, useMissions } from '../hooks/useMissions'
import { taskStatusLabel, taskStatusColor } from '../lib/statusLabels'
import {
  taskAssignedMemberLabel,
  taskClaimedMemberLabel,
  taskCreatedMemberLabel,
} from '../lib/taskMembers'
import {
  relativeTime,
  taskIdShort,
  taskDuration,
  parseRetryTask,
  getAcceptanceCriteria,
  getLatestReview,
} from './boardHelpers'
import { getTaskActivities } from './taskActivity'
import { tasksPanelLink, missionDetailLink, decisionsLink } from '../lib/routing'
import { CollapsibleSection } from '../components/strategy/CollapsibleSection'
import EditTaskDialog from '../components/task/EditTaskDialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import type { Task, TaskActivity } from '../lib/types'

const TERMINAL_STATUSES = ['succeeded', 'failed', 'canceled']

/* ── Activity entry (mirrors TaskPanel's local component) ── */

function activityKindLabel(kind: string): string {
  switch (kind) {
    case 'tool_call': return 'Tool'
    case 'decision_logged': return 'Decision'
    case 'sub_task_created': return 'Sub-task'
    default: return 'State'
  }
}

function activityKindColor(kind: string): string {
  switch (kind) {
    case 'decision_logged': return 'var(--green)'
    case 'tool_call': return 'var(--accent)'
    case 'sub_task_created': return 'var(--accent)'
    default: return 'var(--text-3)'
  }
}

function ActivityEntry({ activity }: { activity: TaskActivity }) {
  const details = activity.details
    ? Object.entries(activity.details).filter(([, v]) => v !== '' && v != null)
    : []
  return (
    <div className="flex gap-3">
      <div className="w-12 shrink-0 pt-0.5">
        <div style={{ fontSize: '0.625rem', color: 'var(--text-3)' }}>{relativeTime(activity.timestamp)}</div>
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1.5 flex-wrap mb-1">
          <span style={{ fontSize: '0.625rem', fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: activityKindColor(activity.kind) }}>
            {activityKindLabel(activity.kind)}
          </span>
          <span style={{ fontSize: '0.625rem', color: 'var(--text-3)' }}>·</span>
          <span style={{ fontSize: '0.625rem', color: 'var(--text-2)' }}>{activity.actor}</span>
        </div>
        <div style={{ fontSize: '0.8125rem', lineHeight: 1.47, color: 'var(--text-2)' }}>{activity.summary}</div>
        {details.length > 0 && (
          <details className="mt-1.5">
            <summary style={{ fontSize: '0.625rem', fontWeight: 600, color: 'var(--accent)', cursor: 'pointer' }}>Details</summary>
            <div
              className="flex flex-col"
              style={{ marginTop: 6, background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 4, padding: '6px 10px', gap: 4 }}
            >
              {details.map(([key, value]) => (
                <div key={key} style={{ fontSize: '0.6875rem', color: 'var(--text-2)', wordBreak: 'break-word' }}>
                  <span style={{ fontWeight: 600, color: 'var(--text-1)' }}>{key}:</span> {String(value)}
                </div>
              ))}
            </div>
          </details>
        )}
      </div>
    </div>
  )
}

/* ── Stat row in the metadata grid ── */

function StatItem({ label, value, icon }: { label: string; value: ReactNode; icon?: ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <span style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-3)' }}>
        {label}
      </span>
      <span className="flex items-center gap-1.5 text-[var(--text-1)]" style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}>
        {icon}
        {value}
      </span>
    </div>
  )
}

/* ── Loading skeleton ── */

function TaskDetailSkeleton() {
  return (
    <div className="px-6 pt-8 max-w-4xl mx-auto w-full">
      <Skeleton className="h-4 w-20 mb-6" />
      <Skeleton className="h-8 w-96 mb-3" />
      <Skeleton className="h-4 w-40 mb-8" />
      <div className="flex flex-col gap-4">
        <Skeleton className="h-20 w-full rounded-[var(--r-md)]" />
        <Skeleton className="h-32 w-full rounded-[var(--r-md)]" />
      </div>
    </div>
  )
}

/* ── Cancel-task confirmation dialog ── */

function CancelTaskDialog({
  task,
  open,
  onOpenChange,
}: {
  task: Task
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [reason, setReason] = useState('')
  const cancelTask = useCancelTask()

  async function handleCancel() {
    const trimmed = reason.trim()
    if (!trimmed) {
      toast.error('A reason is required to cancel a task')
      return
    }
    try {
      await cancelTask.mutateAsync({ taskId: task.id, reason: trimmed })
      toast.success('Task canceled')
      setReason('')
      onOpenChange(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to cancel task')
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Cancel task</AlertDialogTitle>
          <AlertDialogDescription>
            This moves the task to a canceled state. The reason is recorded on the task and shared
            with whoever was assigned. This can't be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="flex flex-col gap-2 py-1">
          <Label htmlFor="cancel-task-reason" className="dashboard-mission-dialog-label">Reason</Label>
          <Textarea
            id="cancel-task-reason"
            placeholder="Why is this task being canceled?"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            rows={3}
            autoFocus
            className="dashboard-mission-dialog-field min-h-[88px] resize-none"
          />
        </div>
        <AlertDialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            className="dashboard-action-button dashboard-action-button-neutral border-0"
          >
            Keep task
          </Button>
          <Button
            onClick={handleCancel}
            disabled={cancelTask.isPending || !reason.trim()}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {cancelTask.isPending ? 'Canceling…' : 'Cancel task'}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

/* ── Main component ── */

export default function TaskDetail() {
  const [, params] = useRoute('/project/:projectId/tasks/:taskId')
  const projectId = params?.projectId ? decodeURIComponent(params.projectId) : null
  const taskId = params?.taskId ? decodeURIComponent(params.taskId) : null

  const [editOpen, setEditOpen] = useState(false)
  const [cancelOpen, setCancelOpen] = useState(false)
  const [showOlderActivity, setShowOlderActivity] = useState(false)

  const { data: task, isLoading, isError, error } = useTask(taskId)

  // Related-entity lookups — only meaningful once the task is loaded.
  const decisionsQuery = useRecentDecisions(projectId)
  const krsQuery = useProjectKRs(projectId)
  const directKrQuery = useKeyResult(task?.keyResultRef ?? null)
  const missionsQuery = useMissions(projectId)

  if (!projectId || !taskId) {
    return <div className="max-w-4xl mx-auto px-6 pt-8 text-[var(--text-3)] text-sm">Task not found.</div>
  }

  if (isLoading) return <TaskDetailSkeleton />

  if (isError) {
    return (
      <div className="max-w-4xl mx-auto px-6 pt-8">
        <div className="flex items-center gap-2 text-[var(--red)] text-sm">
          <AlertCircle size={15} />
          <span>Failed to load task: {error instanceof Error ? error.message : 'Unknown error'}</span>
        </div>
      </div>
    )
  }

  if (!task) {
    return <div className="max-w-4xl mx-auto px-6 pt-8 text-[var(--text-3)] text-sm">Task not found.</div>
  }

  const status = task.status ?? 'pending'
  const statusLabel = taskStatusLabel(status)
  const statusColor = taskStatusColor(status)
  const isTerminal = TERMINAL_STATUSES.includes(status)

  const retry = parseRetryTask(task)
  const displayTitle = retry.isRetry ? retry.originalGoal : (task.title || task.description)
  const acceptanceCriteria = getAcceptanceCriteria(task)
  const acDone = acceptanceCriteria.filter((c) => c.satisfied).length
  const acTotal = acceptanceCriteria.length
  const acColor = acDone === acTotal && acTotal > 0 ? 'var(--green)' : acDone > 0 ? 'var(--amber)' : 'var(--text-3)'
  const duration = taskDuration(task)
  const assigneeLabel = taskAssignedMemberLabel(task)
  const claimedByLabel = taskClaimedMemberLabel(task)
  const createdByLabel = taskCreatedMemberLabel(task)
  const latestReview = getLatestReview(task)
  const activities = getTaskActivities(task)
  const visibleActivities = showOlderActivity ? activities : activities.slice(-8)
  const hiddenActivityCount = activities.length - visibleActivities.length

  const taskDecisions = (decisionsQuery.data ?? []).filter((d) => d.taskRef === task.id)
  const kr = task.keyResultRef
    ? directKrQuery.data ?? krsQuery.data?.get(task.keyResultRef)
    : undefined
  const mission = kr ? (missionsQuery.data ?? []).find((m) => m.id === kr.missionId) : undefined
  const hasRelated = !!mission || !!kr || taskDecisions.length > 0

  const reviewTone: Record<string, { fg: string; bg: string; label: string }> = {
    approved: { fg: 'var(--green)', bg: 'color-mix(in srgb, var(--green) 12%, transparent)', label: 'Approved' },
    retry: { fg: 'var(--amber)', bg: 'color-mix(in srgb, var(--amber) 12%, transparent)', label: 'Retry Requested' },
    failed: { fg: 'var(--red)', bg: 'color-mix(in srgb, var(--red) 10%, transparent)', label: 'Failed' },
    fail: { fg: 'var(--red)', bg: 'color-mix(in srgb, var(--red) 10%, transparent)', label: 'Failed' },
  }

  return (
    <div className="flex flex-col h-full overflow-y-auto">
      {/* Sticky header */}
      <div className="sticky top-0 z-10 bg-[var(--bg-app)] border-b border-[var(--border)]/60 w-full">
        <div className="px-6 pt-6 pb-4 max-w-4xl mx-auto w-full">
          <Link
            to={tasksPanelLink(projectId)}
            className="inline-flex items-center gap-1.5 text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors no-underline mb-5"
            style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}
          >
            <ArrowLeft size={13} />
            Tasks
          </Link>

          <div className="flex items-start gap-3">
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 mb-2">
                <span className="uppercase" style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', color: 'var(--text-3)' }}>
                  Task
                </span>
                <span className="flex items-center gap-0.5 text-[var(--text-3)]">
                  <Hash size={9} />
                  <span style={{ fontSize: '0.625rem', fontFamily: 'monospace' }}>{taskIdShort(task.id)}</span>
                </span>
              </div>
              <h1
                className="m-0 text-[var(--text-1)]"
                style={{ fontSize: '1.75rem', fontWeight: 700, letterSpacing: '-0.56px', lineHeight: 1.14 }}
              >
                {displayTitle}
              </h1>
              <div className="flex items-center gap-2 mt-3">
                <Badge
                  variant="outline"
                  className="gap-1.5"
                  style={{ borderRadius: '4px', fontSize: '0.75rem', fontWeight: 600, letterSpacing: '-0.12px' }}
                >
                  <span style={{ width: 6, height: 6, borderRadius: '50%', background: statusColor || 'var(--text-3)', display: 'inline-block' }} />
                  {statusLabel}
                </Badge>
                {task.taskKind && (
                  <span className="text-[var(--text-3)]" style={{ fontSize: '0.75rem', letterSpacing: '-0.08px' }}>
                    {task.taskKind}
                  </span>
                )}
              </div>
            </div>

            {/* Actions */}
            <div className="flex items-center gap-2 shrink-0">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setEditOpen(true)}
                className="dashboard-action-button dashboard-action-button-neutral"
                style={{ letterSpacing: '-0.12px' }}
              >
                <Pencil size={12} className="mr-1" />
                Edit
              </Button>
              {!isTerminal && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setCancelOpen(true)}
                  className="dashboard-action-button"
                  style={{ letterSpacing: '-0.12px', color: 'var(--red)' }}
                >
                  <Ban size={12} className="mr-1" />
                  Cancel
                </Button>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="px-6 py-5 max-w-4xl mx-auto w-full flex flex-col gap-5">
        {/* Retry banner */}
        {retry.isRetry && retry.feedback && (
          <div
            style={{
              background: 'color-mix(in srgb, var(--amber) 10%, transparent)',
              border: '1px solid color-mix(in srgb, var(--amber) 30%, transparent)',
              borderRadius: 'var(--r-md)',
              padding: '10px 12px',
              fontSize: '0.8125rem',
              lineHeight: 1.47,
              color: 'var(--amber)',
            }}
          >
            {retry.attemptNum ? <span style={{ fontWeight: 600 }}>Attempt {retry.attemptNum}: </span> : null}
            {retry.feedback}
          </div>
        )}

        {/* Description */}
        {!retry.isRetry && task.description && (
          <div className="flex flex-col gap-1.5">
            <span style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-3)' }}>
              Goal
            </span>
            <p className="m-0 text-[var(--text-2)]" style={{ fontSize: '0.875rem', letterSpacing: '-0.14px', lineHeight: 1.55 }}>
              {task.description}
            </p>
          </div>
        )}

        {/* Summary */}
        {task.summary && (
          <div className="flex flex-col gap-1.5">
            <span style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-3)' }}>
              Summary
            </span>
            <p className="m-0 text-[var(--text-3)]" style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px', lineHeight: 1.55 }}>
              {task.summary}
            </p>
          </div>
        )}

        {/* Error */}
        {task.error && (
          <div
            style={{
              background: 'color-mix(in srgb, var(--red) 8%, transparent)',
              border: '1px solid color-mix(in srgb, var(--red) 25%, transparent)',
              borderRadius: 'var(--r-md)',
              padding: '10px 12px',
            }}
          >
            <p className="flex items-center gap-1 m-0 mb-1.5 uppercase" style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', color: 'var(--red)' }}>
              <AlertTriangle size={10} /> Error
            </p>
            <p className="m-0" style={{ fontFamily: 'monospace', fontSize: '0.75rem', lineHeight: 1.5, color: 'var(--text-2)', wordBreak: 'break-word' }}>
              {task.error}
            </p>
          </div>
        )}

        {/* Block reason */}
        {status === 'blocked' && !!task.metadata?.blockReason && (
          <div
            style={{
              background: 'color-mix(in srgb, var(--amber) 10%, transparent)',
              border: '1px solid color-mix(in srgb, var(--amber) 30%, transparent)',
              borderRadius: 'var(--r-md)',
              padding: '10px 12px',
            }}
          >
            <p className="flex items-center gap-1 m-0 mb-1.5 uppercase" style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', color: 'var(--amber)' }}>
              <AlertTriangle size={10} /> Blocked
            </p>
            <p className="m-0" style={{ fontSize: '0.8125rem', lineHeight: 1.5, color: 'var(--amber)' }}>
              {String(task.metadata.blockReason)}
            </p>
          </div>
        )}

        {/* Stats grid */}
        <div className="rounded-[var(--r-md)] bg-[var(--bg-surface)] border border-[var(--border)]/60 px-4 py-3.5">
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
            {assigneeLabel && <StatItem label="Assignee" value={assigneeLabel} />}
            {claimedByLabel && claimedByLabel !== assigneeLabel && (
              <StatItem label="Claimed" value={claimedByLabel} />
            )}
            {createdByLabel && createdByLabel !== assigneeLabel && createdByLabel !== claimedByLabel && (
              <StatItem label="Created By" value={createdByLabel} />
            )}
            <StatItem
              label="Created"
              value={relativeTime(task.createdAt)}
              icon={<Clock size={11} style={{ color: 'var(--text-3)' }} />}
            />
            {task.completedAt && (
              <StatItem
                label={duration ? 'Duration' : 'Completed'}
                value={duration ?? relativeTime(task.completedAt)}
                icon={<Clock size={11} style={{ color: 'var(--text-3)' }} />}
              />
            )}
            {task.taskKind && <StatItem label="Kind" value={task.taskKind} />}
          </div>
        </div>

        {/* Acceptance Criteria */}
        {acTotal > 0 && (
          <CollapsibleSection
            storageKey="task-detail-ac"
            defaultOpen
            label={<>Acceptance Criteria <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>{acDone}/{acTotal}</span></>}
            accent={acColor}
          >
            <ul className="m-0 p-0 list-none flex flex-col" style={{ borderTop: '1px solid var(--border)' }}>
              {acceptanceCriteria.map((criterion, i) => {
                const checked = criterion.satisfied
                return (
                  <li
                    key={criterion.id || i}
                    className="flex items-start gap-2"
                    style={{
                      paddingTop: '9px',
                      paddingBottom: '9px',
                      borderBottom: i < acceptanceCriteria.length - 1 ? '1px solid var(--border)' : 'none',
                    }}
                  >
                    <span
                      className="flex-shrink-0 flex items-center justify-center"
                      style={{
                        width: 13,
                        height: 13,
                        marginTop: 2,
                        borderRadius: 3,
                        border: checked ? 'none' : '1px solid var(--border-strong)',
                        background: checked ? 'var(--green)' : 'transparent',
                      }}
                    >
                      {checked && <span style={{ color: 'white', fontSize: '0.5rem', fontWeight: 700, lineHeight: 1 }}>✓</span>}
                    </span>
                    <span
                      style={{
                        fontSize: '0.8125rem',
                        lineHeight: 1.47,
                        letterSpacing: '-0.08px',
                        color: checked ? 'var(--text-3)' : 'var(--text-1)',
                        textDecoration: checked ? 'line-through' : 'none',
                      }}
                    >
                      {criterion.text}
                    </span>
                  </li>
                )
              })}
            </ul>
          </CollapsibleSection>
        )}

        {/* Latest Review */}
        {latestReview && (() => {
          const tone = reviewTone[latestReview.decision] ?? { fg: 'var(--text-1)', bg: 'var(--bg-elevated)', label: latestReview.decision }
          return (
            <CollapsibleSection storageKey="task-detail-review" defaultOpen label="Latest Review">
              <div style={{ borderTop: '1px solid var(--border)', paddingTop: 10 }}>
                <div className="flex items-center gap-2 flex-wrap">
                  <span
                    style={{
                      fontSize: '0.625rem',
                      fontWeight: 700,
                      letterSpacing: '0.06em',
                      textTransform: 'uppercase',
                      padding: '2px 8px',
                      borderRadius: 980,
                      color: tone.fg,
                      background: tone.bg,
                    }}
                  >
                    {tone.label}
                  </span>
                  {(latestReview.reviewerRole || latestReview.reviewedBy) && (
                    <span style={{ fontSize: '0.6875rem', color: 'var(--text-3)' }}>
                      by {latestReview.reviewerRole || latestReview.reviewedBy}
                    </span>
                  )}
                  {latestReview.reviewedAt && (
                    <span style={{ fontSize: '0.6875rem', color: 'var(--text-3)' }}>{relativeTime(latestReview.reviewedAt)}</span>
                  )}
                </div>
                {latestReview.feedback && (
                  <div className="md-prose" style={{ fontSize: '0.8125rem', lineHeight: 1.47, color: 'var(--text-2)', marginTop: 8 }}>
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{latestReview.feedback}</ReactMarkdown>
                  </div>
                )}
              </div>
            </CollapsibleSection>
          )
        })()}

        {/* Related */}
        {hasRelated && (
          <CollapsibleSection storageKey="task-detail-related" defaultOpen label="Related">
            <div className="flex flex-col" style={{ borderTop: '1px solid var(--border)' }}>
              {mission && (
                <Link
                  to={missionDetailLink(projectId, mission.id)}
                  className="flex items-center gap-2 py-2.5 no-underline group"
                  style={{ borderBottom: (kr || taskDecisions.length > 0) ? '1px solid var(--border)' : 'none' }}
                >
                  <span style={{ fontSize: '0.625rem', fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: 'var(--text-3)', width: 78 }}>Mission</span>
                  <span className="flex-1 min-w-0 truncate text-[var(--text-1)] group-hover:text-[var(--accent)] transition-colors" style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}>
                    {mission.title}
                  </span>
                  <ChevronRight size={13} className="shrink-0 text-[var(--text-3)]" />
                </Link>
              )}
              {kr && (
                <Link
                  to={mission ? missionDetailLink(projectId, mission.id) : tasksPanelLink(projectId)}
                  className="flex items-center gap-2 py-2.5 no-underline group"
                  style={{ borderBottom: taskDecisions.length > 0 ? '1px solid var(--border)' : 'none' }}
                >
                  <span style={{ fontSize: '0.625rem', fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: 'var(--text-3)', width: 78 }}>Key Result</span>
                  <span className="flex-1 min-w-0 truncate text-[var(--text-1)] group-hover:text-[var(--accent)] transition-colors" style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}>
                    {kr.title}
                  </span>
                  {kr.progressPercent > 0 && (
                    <span className="shrink-0 tabular-nums text-[var(--text-3)]" style={{ fontSize: '0.6875rem' }}>
                      {Math.round(kr.progressPercent)}%
                    </span>
                  )}
                  <ChevronRight size={13} className="shrink-0 text-[var(--text-3)]" />
                </Link>
              )}
              {taskDecisions.map((dec, i) => (
                <Link
                  key={dec.id}
                  to={decisionsLink(projectId)}
                  className="flex items-center gap-2 py-2.5 no-underline group"
                  style={{ borderBottom: i < taskDecisions.length - 1 ? '1px solid var(--border)' : 'none' }}
                >
                  <span style={{ fontSize: '0.625rem', fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: 'var(--text-3)', width: 78 }}>Decision</span>
                  <span className="flex-1 min-w-0 truncate text-[var(--text-1)] group-hover:text-[var(--accent)] transition-colors" style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}>
                    {dec.title}
                  </span>
                  {dec.confidence > 0 && (
                    <span
                      className="shrink-0 tabular-nums"
                      style={{ fontSize: '0.6875rem', color: dec.confidence >= 0.8 ? 'var(--green)' : dec.confidence >= 0.6 ? 'var(--amber)' : 'var(--red)' }}
                    >
                      {Math.round(dec.confidence * 100)}%
                    </span>
                  )}
                  <ChevronRight size={13} className="shrink-0 text-[var(--text-3)]" />
                </Link>
              ))}
            </div>
          </CollapsibleSection>
        )}

        {/* Artifacts */}
        {task.artifacts && task.artifacts.length > 0 && (
          <CollapsibleSection
            storageKey="task-detail-artifacts"
            defaultOpen={false}
            label={<>Artifacts <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>({task.artifacts.length})</span></>}
          >
            <div style={{ borderTop: '1px solid var(--border)' }}>
              {task.artifacts.map((a, i) => (
                <div
                  key={i}
                  style={{
                    paddingTop: 8,
                    paddingBottom: 8,
                    borderBottom: i < task.artifacts!.length - 1 ? '1px solid var(--border)' : 'none',
                  }}
                >
                  <span style={{ fontSize: '0.75rem', color: 'var(--accent)', fontFamily: 'monospace', wordBreak: 'break-all' }}>{a}</span>
                </div>
              ))}
            </div>
          </CollapsibleSection>
        )}

        {/* Activity */}
        {activities.length > 0 && (
          <CollapsibleSection storageKey="task-detail-activity" defaultOpen label="Activity">
            <div style={{ borderTop: '1px solid var(--border)', paddingTop: 10 }}>
              <div className="flex flex-col" style={{ gap: '12px', paddingLeft: 10, borderLeft: '2px solid var(--border)' }}>
                {hiddenActivityCount > 0 && (
                  <button
                    onClick={() => setShowOlderActivity((v) => !v)}
                    style={{ fontSize: '0.625rem', fontWeight: 600, color: 'var(--accent)', background: 'transparent', border: 'none', cursor: 'pointer', padding: 0, textAlign: 'left' }}
                  >
                    {showOlderActivity ? 'Hide earlier activity' : `Show ${hiddenActivityCount} earlier event${hiddenActivityCount > 1 ? 's' : ''}`}
                  </button>
                )}
                {visibleActivities.map((a, i) => <ActivityEntry key={a.eventId ?? i} activity={a} />)}
              </div>
            </div>
          </CollapsibleSection>
        )}
      </div>

      <EditTaskDialog task={task} open={editOpen} onOpenChange={setEditOpen} />
      <CancelTaskDialog task={task} open={cancelOpen} onOpenChange={setCancelOpen} />
    </div>
  )
}
