import { Suspense, useEffect, useRef, useState } from 'react'
import { useRoute, useLocation } from 'wouter'
import { toast } from 'sonner'
import {
  AlertTriangle,
  Clock,
  Network,
  Pencil,
  Ban,
  Check,
  Plus,
  X,
} from 'lucide-react'
import { useTask, useUpdateTask, useAssignTask } from '../hooks/useProjectTasks'
import { useProjectMembers } from '../hooks/useProjectMembers'
import { memberDisplayName } from '../lib/memberDisplay'
import ResumeSession from '../components/ResumeSession'
import { isResumableSession } from '../lib/sessionResume'
import { useRecentDecisions } from '../hooks/useDecisions'
import { useKeyResult, useMission, useProjectKRs } from '../hooks/useMissions'
import { taskStatusLabel, taskStatusColor } from '../lib/statusLabels'
import { formatRelative } from '@/lib/format'
import { confidenceColor } from '@/lib/decisionDisplay'
import {
  taskAssignedMemberLabel,
  taskClaimedMemberLabel,
  taskCreatedMemberLabel,
} from '../lib/taskMembers'
import {
  taskIdShort,
  taskDuration,
  parseRetryTask,
} from './boardHelpers'
import { tasksPanelLink, missionDetailLink, decisionsLink, strategyMapLink, mapNodeId, useNavigation } from '../lib/routing'
import { StatItem } from '../components/detail/StatItem'
import { DetailNotFound, DetailError } from '../components/detail/DetailStates'
import { DetailSkeleton } from '../components/detail/DetailSkeleton'
import { DetailHeader } from '../components/detail/DetailHeader'
import { RelatedList, type RelatedItem } from '../components/detail/RelatedList'
import { CancelTaskDialog } from '../components/task/CancelTaskDialog'
import { AcceptanceCriteriaList } from '../components/task/AcceptanceCriteriaList'
import { LatestReviewSection } from '../components/task/LatestReviewSection'
import { TaskArtifactsSection } from '../components/task/TaskArtifactsSection'
import CopyIdChip from '../components/CopyIdChip'
import { ResizeHandle } from '../components/detail/ResizeHandle'
import { useIsBelow } from '../hooks/use-mobile'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { lazyWithRetry } from '../lib/lazyWithRetry'
import type { ArtifactViewerPanelProps } from '../components/task/ArtifactViewerPanel'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'

const TERMINAL_STATUSES = ['succeeded', 'failed', 'canceled']

const ArtifactViewerPanel = lazyWithRetry<ArtifactViewerPanelProps>(
  () => import('../components/task/ArtifactViewerPanel').then((module) => ({ default: module.ArtifactViewerPanel })),
  'components/task/ArtifactViewerPanel',
)

function ArtifactViewerLoading({ layout }: { layout: 'sheet' | 'inline' }) {
  const layoutClass = layout === 'sheet'
    ? 'fixed inset-y-0 right-0 z-50 w-screen border-l border-[var(--border)] bg-[var(--bg-surface)] sm:w-[min(720px,90vw)]'
    : 'h-full w-full border-l border-[var(--border)] bg-[var(--bg-surface)]'
  return (
    <div className={`flex items-center justify-center ${layoutClass}`} role="status" aria-label="Loading artifact viewer">
      <span className="spinner spinner-md" />
    </div>
  )
}

/* ── Inline-edit acceptance-criteria rows ── */

interface CriterionRow {
  id: string
  text: string
  satisfied: boolean
}

function newCriterionId(): string {
  const rand = typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : Math.random().toString(36).slice(2)
  return `criterion-${rand}`
}

/* ── Loading skeleton ── */

function TaskDetailSkeleton() {
  return (
    <DetailSkeleton>
      <div className="flex flex-col gap-4">
        <Skeleton className="h-20 w-full rounded-[var(--r-md)]" />
        <Skeleton className="h-32 w-full rounded-[var(--r-md)]" />
      </div>
    </DetailSkeleton>
  )
}

/* ── Main component ── */

export default function TaskDetail() {
  const [, params] = useRoute('/project/:projectId/tasks/:taskId')
  const [, navigate] = useLocation()
  const projectId = params?.projectId ? decodeURIComponent(params.projectId) : null
  const taskId = params?.taskId ? decodeURIComponent(params.taskId) : null

  // Inline edit state. Edits happen in place on the page (no modal): the title
  // turns into an input in the header and the description/kind/criteria become
  // fields in the body, with Save/Cancel living in the action bar. Decision
  // dec-51c505cd — inline scales better than a modal for this large page.
  const [editing, setEditing] = useState(false)
  const [editTitle, setEditTitle] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [editKind, setEditKind] = useState('')
  const [editCriteria, setEditCriteria] = useState<CriterionRow[]>([])
  const [editAssignee, setEditAssignee] = useState('')
  const [cancelOpen, setCancelOpen] = useState(false)
  // Artifact viewer host: a split panel beside the page on wide screens so
  // the task stays in view while reviewing; an overlay sheet when there is
  // no room for both (dec-e8b58636).
  const [openArtifactVPath, setOpenArtifactVPath] = useState<string | null>(null)
  const viewerAsSheet = useIsBelow(1152)
  const [renderedArtifactVPath, setRenderedArtifactVPath] = useState<string | null>(null)
  const [inlineViewerVisible, setInlineViewerVisible] = useState(false)
  // Persisted, drag-adjustable width of the inline viewer panel.
  const [panelWidth, setPanelWidth] = useState<number>(() => {
    const stored = typeof window !== 'undefined' ? Number(window.localStorage.getItem('task-artifact-panel-width')) : NaN
    return Number.isFinite(stored) && stored > 0 ? stored : 560
  })
  // The flex row holding the task content + the viewer panel. Measured so the
  // panel's max width can leave a minimum task column (the sidebar is outside
  // this row, so window.innerWidth would over-estimate the available space).
  const splitRowRef = useRef<HTMLDivElement | null>(null)
  const MIN_TASK_WIDTH = 375 // mobile floor: the task content never goes narrower
  const MIN_PANEL_WIDTH = 360
  const RESIZE_HANDLE_WIDTH = 4 // the w-1 handle sits between content and panel
  const clampPanelWidth = (width: number) => {
    const rowWidth = splitRowRef.current?.getBoundingClientRect().width ?? window.innerWidth
    const max = Math.max(MIN_PANEL_WIDTH, rowWidth - MIN_TASK_WIDTH - RESIZE_HANDLE_WIDTH)
    return Math.min(Math.max(width, MIN_PANEL_WIDTH), max)
  }
  const resizePanel = (deltaX: number) => {
    setPanelWidth((current) => {
      // Handle is on the panel's LEFT edge: dragging left (negative delta)
      // widens the panel. Cap so the task column keeps at least MIN_TASK_WIDTH.
      const next = clampPanelWidth(current - deltaX)
      window.localStorage.setItem('task-artifact-panel-width', String(Math.round(next)))
      return next
    })
  }
  // Re-clamp when the available row width changes (mount, viewport resize) so a
  // width saved on a wide screen can't crush the task column on a narrow one.
  useEffect(() => {
    if (viewerAsSheet || !openArtifactVPath) return
    const reclamp = () => setPanelWidth((w) => clampPanelWidth(w))
    reclamp()
    window.addEventListener('resize', reclamp)
    return () => window.removeEventListener('resize', reclamp)
  }, [viewerAsSheet, openArtifactVPath])

  useEffect(() => {
    if (viewerAsSheet) {
      const timeout = window.setTimeout(() => {
        setRenderedArtifactVPath(null)
        setInlineViewerVisible(false)
      }, 180)
      return () => window.clearTimeout(timeout)
    }

    if (openArtifactVPath) {
      const frame = requestAnimationFrame(() => setInlineViewerVisible(true))
      return () => cancelAnimationFrame(frame)
    }

    if (!renderedArtifactVPath) {
      return
    }

    const timeout = window.setTimeout(() => setRenderedArtifactVPath(null), 180)
    return () => window.clearTimeout(timeout)
  }, [openArtifactVPath, renderedArtifactVPath, viewerAsSheet])

  const openArtifactViewer = (vpath: string) => {
    setRenderedArtifactVPath(vpath)
    setInlineViewerVisible(false)
    setOpenArtifactVPath(vpath)
  }

  const closeArtifactViewer = () => {
    setInlineViewerVisible(false)
    setOpenArtifactVPath(null)
  }

  const updateTask = useUpdateTask()
  const assignTask = useAssignTask()
  const { focusedProjectRoot } = useNavigation()

  const { data: task, isLoading, isError, error } = useTask(taskId)
  const membersQuery = useProjectMembers(projectId)

  // Related-entity lookups — only meaningful once the task is loaded.
  const decisionsQuery = useRecentDecisions(projectId)
  const krsQuery = useProjectKRs(projectId)
  const directKrQuery = useKeyResult(task?.keyResultRef ?? null)
  // Resolve the KR (scope-independent direct fetch, falling back to the project
  // KR map) and, through it or the task's own missionRef, the related mission.
  // The mission is fetched by id via useMission rather than scanned out of the
  // project-scoped mission list: a mission with an empty scopeId never appears
  // in that list, which used to leave a KR-less task's Related section blank.
  const kr = task?.keyResultRef
    ? directKrQuery.data ?? krsQuery.data?.get(task.keyResultRef)
    : undefined
  const relatedMissionId = kr?.missionId ?? task?.missionRef ?? null
  const missionQuery = useMission(relatedMissionId)

  if (!projectId || !taskId) {
    return <DetailNotFound entity="task" />
  }

  if (isLoading) return <TaskDetailSkeleton />

  if (isError) {
    return <DetailError entity="task" message={error instanceof Error ? error.message : 'Unknown error'} />
  }

  if (!task) {
    return <DetailNotFound entity="task" />
  }

  const status = task.status ?? 'pending'
  const statusLabel = taskStatusLabel(status)
  const statusColor = taskStatusColor(status)
  const isTerminal = TERMINAL_STATUSES.includes(status)

  const retry = parseRetryTask(task)
  const displayTitle = retry.isRetry ? retry.originalGoal : (task.title || task.description)
  const duration = taskDuration(task)
  const assigneeLabel = taskAssignedMemberLabel(task)
  const claimedByLabel = taskClaimedMemberLabel(task)
  const createdByLabel = taskCreatedMemberLabel(task)

  // The member to jump back to: whoever's actually on it (claimed), else the
  // assignee. Resolve the full record so we can offer a resume affordance when
  // its harness session is reopenable.
  const workingMemberId = task.claimedByMemberId ?? task.assignedTo
  const workingMember = workingMemberId
    ? (membersQuery.data ?? []).find((m) => m.id === workingMemberId)
    : undefined
  const canResumeWorking = isResumableSession(workingMember?.harnessKind, workingMember?.nativeSessionRef)

  // Bind the post-guard narrowed task so the edit closures below don't see the
  // widened `Task | undefined` (TS won't carry the `if (!task)` narrowing in).
  const taskId_ = task.id

  // Seed the inline edit fields from the current task and switch to edit mode.
  function beginEdit() {
    setEditTitle(task!.title ?? '')
    setEditDescription(task!.description ?? '')
    setEditKind(task!.taskKind ?? '')
    setEditAssignee(task!.assignedTo ?? '')
    setEditCriteria(
      (task!.acceptanceCriteria ?? []).map((c) => ({ id: c.id, text: c.text, satisfied: c.satisfied })),
    )
    setEditing(true)
  }

  function addCriterion() {
    setEditCriteria((prev) => [...prev, { id: newCriterionId(), text: '', satisfied: false }])
  }

  function updateCriterionText(id: string, text: string) {
    setEditCriteria((prev) => prev.map((c) => (c.id === id ? { ...c, text } : c)))
  }

  function toggleCriterion(id: string, satisfied: boolean) {
    setEditCriteria((prev) => prev.map((c) => (c.id === id ? { ...c, satisfied } : c)))
  }

  function removeCriterion(id: string) {
    setEditCriteria((prev) => prev.filter((c) => c.id !== id))
  }

  async function handleSaveEdit() {
    const trimmedDescription = editDescription.trim()
    if (!trimmedDescription) {
      toast.error('Task description is required')
      return
    }
    const acceptanceCriteria = editCriteria
      .map((c) => ({ id: c.id, text: c.text.trim(), satisfied: c.satisfied }))
      .filter((c) => c.text)
    // Reassignment is a distinct verb from update: the backend requeues a
    // non-terminal task (clears the claim, resets to pending) so the new
    // assignee can claim it. Only fire it when the picker actually changed.
    const assigneeChanged = !isTerminal && !!editAssignee && editAssignee !== (task!.assignedTo ?? '')
    try {
      await updateTask.mutateAsync({
        taskId: taskId_,
        title: editTitle.trim(),
        description: trimmedDescription,
        taskKind: editKind.trim(),
        acceptanceCriteria,
      })
      if (assigneeChanged) {
        await assignTask.mutateAsync({ taskId: taskId_, assignedTo: editAssignee })
        toast.success('Task updated and reassigned — it returns to the queue for the new assignee')
      } else {
        toast.success('Task updated')
      }
      setEditing(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update task')
    }
  }

  const taskDecisions = (decisionsQuery.data ?? []).filter((d) => d.taskRef === task.id)
  const mission = missionQuery.data ?? undefined
  const related: RelatedItem[] = []
  if (mission) {
    related.push({ key: 'mission', label: 'Mission', title: mission.title, to: missionDetailLink(projectId, mission.id) })
  }
  if (kr) {
    related.push({
      key: 'kr',
      label: 'Key Result',
      title: kr.title,
      to: mission ? missionDetailLink(projectId, mission.id) : tasksPanelLink(projectId),
      ...(kr.progressPercent > 0 ? { suffix: `${Math.round(kr.progressPercent)}%` } : {}),
    })
  }
  for (const dec of taskDecisions) {
    related.push({
      key: dec.id,
      label: 'Decision',
      title: dec.title,
      to: decisionsLink(projectId),
      ...(dec.confidence > 0
        ? { suffix: `${Math.round(dec.confidence * 100)}%`, suffixColor: confidenceColor(dec.confidence) }
        : {}),
    })
  }

  // Assignee picker options. Active roster only — a removed member should not be
  // a reassignment target.
  const activeMembers = (membersQuery.data ?? []).filter((m) => m.lifecycleState === 'active')
  const membersLoading = membersQuery.isLoading
  const hasMembers = activeMembers.length > 0

  return (
    <div ref={splitRowRef} className="flex h-full min-h-0 overflow-hidden">
    <div className="flex flex-col h-full overflow-y-auto overflow-x-hidden flex-1 min-w-0">
      {/* Sticky header */}
      <DetailHeader backTo={tasksPanelLink(projectId)} backLabel="Tasks">
          <div className="flex flex-wrap items-start gap-3">
            <div className="flex-1 min-w-[220px]">
              <div className="flex items-center gap-2 mb-2">
                <span className="uppercase" style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', color: 'var(--text-3)' }}>
                  Task
                </span>
                <CopyIdChip id={task.id} shortId={taskIdShort(task.id)} />
              </div>
              {editing ? (
                <Input
                  aria-label="Task title"
                  value={editTitle}
                  onChange={(e) => setEditTitle(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                      e.preventDefault()
                      handleSaveEdit()
                    } else if (e.key === 'Escape') {
                      setEditing(false)
                    }
                  }}
                  placeholder="e.g. Wire up the export endpoint"
                  autoFocus
                  className="w-full min-w-0 h-auto py-1 font-bold tracking-[-0.56px] leading-[1.14]"
                  style={{ fontSize: 'clamp(1.25rem, 5vw, 1.75rem)' }}
                />
              ) : (
                <h1
                  className="m-0 text-[var(--text-1)]"
                  style={{ fontSize: 'clamp(1.25rem, 5vw, 1.75rem)', fontWeight: 700, letterSpacing: '-0.56px', lineHeight: 1.14 }}
                >
                  {displayTitle}
                </h1>
              )}
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

            {/* Actions — Map/Edit/Cancel-task in read mode; Save/Cancel-edit in
                edit mode. flex-wrap keeps the row from overflowing the header at
                mobile/iPad widths (wrapping is intrinsic, no breakpoints). */}
            <div className="flex flex-wrap items-center justify-end gap-2 shrink-0">
              {editing ? (
                <>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleSaveEdit}
                    disabled={updateTask.isPending || assignTask.isPending || !editDescription.trim()}
                    className="dashboard-action-button dashboard-action-button-accent"
                    style={{ letterSpacing: '-0.12px' }}
                  >
                    <Check size={12} className="mr-1" />
                    {updateTask.isPending || assignTask.isPending ? 'Saving…' : 'Save changes'}
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setEditing(false)}
                    className="dashboard-action-button dashboard-action-button-neutral"
                    style={{ letterSpacing: '-0.12px' }}
                  >
                    <X size={12} className="mr-1" />
                    Cancel
                  </Button>
                </>
              ) : (
                <>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => navigate(strategyMapLink(projectId, mapNodeId('task', task.id)))}
                    className="dashboard-action-button dashboard-action-button-neutral"
                    style={{ letterSpacing: '-0.12px' }}
                    title="View in Context Map"
                    aria-label="View in Context Map"
                  >
                    <Network size={12} className="mr-1" />
                    Map
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={beginEdit}
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
                </>
              )}
            </div>
          </div>
      </DetailHeader>

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

        {/* Description + kind — inline editors in edit mode, read-only otherwise */}
        {editing ? (
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="edit-task-desc" style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-3)' }}>
                Goal
              </Label>
              <Textarea
                id="edit-task-desc"
                placeholder="Describe what needs to be done and why…"
                value={editDescription}
                onChange={(e) => setEditDescription(e.target.value)}
                rows={3}
                className="min-h-[118px] resize-y"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="edit-task-kind" style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-3)' }}>
                Kind <span className="text-muted-foreground font-normal lowercase">(optional)</span>
              </Label>
              <Input
                id="edit-task-kind"
                placeholder="e.g. feature, bugfix, research"
                value={editKind}
                onChange={(e) => setEditKind(e.target.value)}
              />
            </div>
            {!isTerminal && (
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="edit-task-assignee" style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-3)' }}>
                  Assignee
                </Label>
                <Select value={editAssignee} onValueChange={setEditAssignee} disabled={!hasMembers}>
                  <SelectTrigger id="edit-task-assignee">
                    <SelectValue
                      placeholder={
                        membersLoading
                          ? 'Loading members…'
                          : hasMembers
                            ? 'Select a project member'
                            : 'No members available'
                      }
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {activeMembers.map((m) => (
                      <SelectItem key={m.id} value={m.id}>
                        {memberDisplayName(m.displayName, m.id) ?? m.id}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {editAssignee && editAssignee !== (task.assignedTo ?? '') && (
                  <span className="text-[var(--text-3)]" style={{ fontSize: '0.6875rem', lineHeight: 1.4 }}>
                    Reassigning returns this task to the queue for the new member to claim.
                  </span>
                )}
              </div>
            )}
          </div>
        ) : (
          !retry.isRetry && task.description && (
            <div className="flex flex-col gap-1.5">
              <span style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-3)' }}>
                Goal
              </span>
              <p className="m-0 text-[var(--text-2)]" style={{ fontSize: '0.875rem', letterSpacing: '-0.14px', lineHeight: 1.55 }}>
                {task.description}
              </p>
            </div>
          )
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
            {/* Assignee is edited inline (picker) for non-terminal tasks; don't also
                show it read-only here while editing. */}
            {assigneeLabel && !(editing && !isTerminal) && <StatItem label="Assignee" value={assigneeLabel} />}
            {claimedByLabel && claimedByLabel !== assigneeLabel && (
              <StatItem label="Claimed" value={claimedByLabel} />
            )}
            {createdByLabel && createdByLabel !== assigneeLabel && createdByLabel !== claimedByLabel && (
              <StatItem label="Created By" value={createdByLabel} />
            )}
            <StatItem
              label="Created"
              value={formatRelative(task.createdAt, { fallback: 'unknown' })}
              icon={<Clock size={11} style={{ color: 'var(--text-3)' }} />}
            />
            {task.completedAt && (
              <StatItem
                label={duration ? 'Duration' : 'Completed'}
                value={duration ?? formatRelative(task.completedAt, { fallback: 'unknown' })}
                icon={<Clock size={11} style={{ color: 'var(--text-3)' }} />}
              />
            )}
            {/* Kind is edited inline while editing; don't duplicate it here. */}
            {task.taskKind && !editing && <StatItem label="Kind" value={task.taskKind} />}
          </div>
          {canResumeWorking && workingMember && (
            <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-[var(--border)]/60 pt-3">
              <span className="text-[0.75rem] text-[var(--text-3)]">
                Jump back into {memberDisplayName(workingMember.displayName, workingMember.id) ?? workingMember.id}&apos;s session
              </span>
              <ResumeSession
                harnessKind={workingMember.harnessKind}
                nativeSessionRef={workingMember.nativeSessionRef}
                projectRoot={focusedProjectRoot}
              />
            </div>
          )}
        </div>

        {/* Acceptance Criteria — editable rows in edit mode, read-only otherwise */}
        {editing ? (
          <div className="flex flex-col gap-2">
            <Label style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-3)' }}>
              Acceptance criteria <span className="text-muted-foreground font-normal lowercase">(optional)</span>
            </Label>
            {editCriteria.length > 0 && (
              <div className="flex flex-col gap-2">
                {editCriteria.map((row, index) => (
                  <div key={row.id} className="flex items-center gap-2">
                    <Checkbox
                      checked={row.satisfied}
                      onCheckedChange={(value) => toggleCriterion(row.id, value === true)}
                      aria-label="Mark criterion satisfied"
                      className="shrink-0"
                    />
                    <Input
                      value={row.text}
                      onChange={(e) => updateCriterionText(row.id, e.target.value)}
                      placeholder={`Criterion ${index + 1}`}
                      className="flex-1 min-w-0"
                    />
                    <button
                      type="button"
                      onClick={() => removeCriterion(row.id)}
                      aria-label="Remove criterion"
                      className="shrink-0 flex items-center justify-center h-7 w-7 rounded-full border-none cursor-pointer bg-transparent text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] transition-colors"
                    >
                      <X size={14} />
                    </button>
                  </div>
                ))}
              </div>
            )}
            <button
              type="button"
              onClick={addCriterion}
              className="inline-flex items-center gap-1.5 self-start border-none cursor-pointer bg-transparent text-[var(--text-2)] hover:text-[var(--text-1)] transition-colors"
              style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}
            >
              <Plus size={13} />
              Add acceptance criterion
            </button>
          </div>
        ) : (
          <AcceptanceCriteriaList task={task} />
        )}

        {/* Latest Review */}
        <LatestReviewSection task={task} />

        {/* Related */}
        <RelatedList items={related} storageKey="task-detail-related" />

        {/* Artifacts */}
        <TaskArtifactsSection task={task} projectId={projectId} onOpenArtifact={openArtifactViewer} />
      </div>

      <CancelTaskDialog task={task} open={cancelOpen} onOpenChange={setCancelOpen} />
    </div>

    {openArtifactVPath && viewerAsSheet && (
      <Suspense fallback={<ArtifactViewerLoading layout="sheet" />}>
        <ArtifactViewerPanel
          key={openArtifactVPath}
          projectId={projectId}
          vpath={openArtifactVPath}
          onClose={closeArtifactViewer}
          layout="sheet"
        />
      </Suspense>
    )}
    {renderedArtifactVPath && !viewerAsSheet && (
      <>
        <div
          className="shrink-0 h-full min-h-0 overflow-hidden transition-[width,opacity,transform] duration-[180ms] ease-out"
          style={{
            width: inlineViewerVisible ? RESIZE_HANDLE_WIDTH : 0,
            opacity: inlineViewerVisible ? 1 : 0,
            transform: inlineViewerVisible ? 'translateX(0)' : 'translateX(6px)',
          }}
          aria-hidden={!inlineViewerVisible}
        >
          <ResizeHandle onResize={resizePanel} aria-label="Resize artifact viewer" />
        </div>
        <div
          className="shrink-0 h-full min-h-0 flex flex-col overflow-hidden transition-[width,opacity,transform] duration-[180ms] ease-out will-change-[width,opacity,transform]"
          style={{
            width: inlineViewerVisible ? panelWidth : 0,
            opacity: inlineViewerVisible ? 1 : 0,
            transform: inlineViewerVisible ? 'translateX(0)' : 'translateX(10px)',
          }}
          aria-hidden={!inlineViewerVisible}
        >
          <Suspense fallback={<ArtifactViewerLoading layout="inline" />}>
            <ArtifactViewerPanel
              key={renderedArtifactVPath}
              projectId={projectId}
              vpath={renderedArtifactVPath}
              onClose={closeArtifactViewer}
              layout="inline"
            />
          </Suspense>
        </div>
      </>
    )}
    </div>
  )
}
