import { useState } from 'react'
import { useRoute, useLocation } from 'wouter'
import {
  AlertTriangle,
  Clock,
  Hash,
  Network,
  Pencil,
  Ban,
} from 'lucide-react'
import { useTask } from '../hooks/useProjectTasks'
import { useRecentDecisions } from '../hooks/useDecisions'
import { useKeyResult, useProjectKRs, useMissions } from '../hooks/useMissions'
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
import { tasksPanelLink, missionDetailLink, decisionsLink, strategyMapLink, mapNodeId } from '../lib/routing'
import { CollapsibleSection } from '../components/strategy/CollapsibleSection'
import { StatItem } from '../components/detail/StatItem'
import { DetailNotFound, DetailError } from '../components/detail/DetailStates'
import { DetailSkeleton } from '../components/detail/DetailSkeleton'
import { DetailHeader } from '../components/detail/DetailHeader'
import { RelatedList, type RelatedItem } from '../components/detail/RelatedList'
import EditTaskDialog from '../components/task/EditTaskDialog'
import { CancelTaskDialog } from '../components/task/CancelTaskDialog'
import { AcceptanceCriteriaList } from '../components/task/AcceptanceCriteriaList'
import { LatestReviewSection } from '../components/task/LatestReviewSection'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

const TERMINAL_STATUSES = ['succeeded', 'failed', 'canceled']

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

  const [editOpen, setEditOpen] = useState(false)
  const [cancelOpen, setCancelOpen] = useState(false)

  const { data: task, isLoading, isError, error } = useTask(taskId)

  // Related-entity lookups — only meaningful once the task is loaded.
  const decisionsQuery = useRecentDecisions(projectId)
  const krsQuery = useProjectKRs(projectId)
  const directKrQuery = useKeyResult(task?.keyResultRef ?? null)
  const missionsQuery = useMissions(projectId)

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

  const taskDecisions = (decisionsQuery.data ?? []).filter((d) => d.taskRef === task.id)
  const kr = task.keyResultRef
    ? directKrQuery.data ?? krsQuery.data?.get(task.keyResultRef)
    : undefined
  // Resolve the related mission through the KR when there is one; otherwise fall
  // back to the task's own missionRef. Tasks created with a direct mission_ref
  // (no KR) are valid — without this fallback their Related section is empty.
  const mission = kr
    ? (missionsQuery.data ?? []).find((m) => m.id === kr.missionId)
    : task.missionRef
      ? (missionsQuery.data ?? []).find((m) => m.id === task.missionRef)
      : undefined
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

  return (
    <div className="flex flex-col h-full overflow-y-auto">
      {/* Sticky header */}
      <DetailHeader backTo={tasksPanelLink(projectId)} backLabel="Tasks">
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
            {task.taskKind && <StatItem label="Kind" value={task.taskKind} />}
          </div>
        </div>

        {/* Acceptance Criteria */}
        <AcceptanceCriteriaList task={task} />

        {/* Latest Review */}
        <LatestReviewSection task={task} />

        {/* Related */}
        <RelatedList items={related} storageKey="task-detail-related" />

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
      </div>

      <EditTaskDialog task={task} open={editOpen} onOpenChange={setEditOpen} />
      <CancelTaskDialog task={task} open={cancelOpen} onOpenChange={setCancelOpen} />
    </div>
  )
}
