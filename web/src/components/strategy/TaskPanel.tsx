import { useState, type ReactNode } from 'react'
import { X, Clock, Hash, AlertTriangle } from 'lucide-react'
import { CollapsibleSection } from './CollapsibleSection'
import { useResizableSummary } from './useResizableSummary'
import { RelatedSection } from './RelatedSection'
import { Badge } from '@/components/ui/badge'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { taskStatusLabel, taskStatusColor } from '../../lib/statusLabels'
import {
  relativeTime,
  taskIdShort,
  taskDuration,
  parseRetryTask,
  getAcceptanceCriteria,
  getLatestReview,
} from '../../pages/boardHelpers'
import { getTaskActivities } from '../../pages/taskActivity'
import { useRecentDecisions } from '../../hooks/useDecisions'
import { useKeyResult, useProjectKRs, useMissions } from '../../hooks/useMissions'
import { memberDisplayName } from '../../lib/memberDisplay'
import type { TaskActivity } from '../../lib/types'
import type { TaskNodeData } from './TaskNode'
import type { NodePanelProps } from './types'

const SF_TEXT = 'SF Pro Text, SF Pro Icons, Helvetica Neue, Helvetica, Arial, sans-serif'
const SUMMARY_MIN = 80
const SUMMARY_MAX = 480
const SUMMARY_DEFAULT = 200

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
      <div className="w-10 shrink-0 pt-0.5">
        <div style={{ fontSize: '10px', color: 'var(--text-3)' }}>{relativeTime(activity.timestamp)}</div>
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1.5 flex-wrap mb-1">
          <span style={{ fontSize: '10px', fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: activityKindColor(activity.kind) }}>
            {activityKindLabel(activity.kind)}
          </span>
          <span style={{ fontSize: '10px', color: 'var(--text-3)' }}>·</span>
          <span style={{ fontSize: '10px', color: 'var(--text-2)' }}>{activity.actor}</span>
        </div>
        <div style={{ fontSize: '12px', lineHeight: 1.47, color: 'var(--text-2)' }}>{activity.summary}</div>
        {details.length > 0 && (
          <details className="mt-1.5">
            <summary style={{ fontSize: '10px', fontWeight: 600, color: 'var(--accent)', cursor: 'pointer' }}>Details</summary>
            <div
              className="flex flex-col"
              style={{ marginTop: 6, background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 4, padding: '6px 10px', gap: 4 }}
            >
              {details.map(([key, value]) => (
                <div key={key} style={{ fontSize: '11px', color: 'var(--text-2)', wordBreak: 'break-word' }}>
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

export function TaskPanel({ data, projectId, onClose }: NodePanelProps) {
  const d = data as TaskNodeData
  const { task } = d
  const { height: summaryHeight, onResizeStart: handleResizeStart } = useResizableSummary(
    'task-panel-summary-height',
    { min: SUMMARY_MIN, max: SUMMARY_MAX, defaultHeight: SUMMARY_DEFAULT },
  )

  const status = task.status ?? 'pending'
  const label = taskStatusLabel(status)
  const color = taskStatusColor(status)

  const retry = parseRetryTask(task)
  const displayTitle = retry.isRetry ? retry.originalGoal : (task.title || task.description)
  const acceptanceCriteria = getAcceptanceCriteria(task)
  const duration = taskDuration(task)
  const assigneeLabel = memberDisplayName(task.assignedToLabel, task.assignedTo)

  const acDone = acceptanceCriteria.filter((criterion) => criterion.satisfied).length
  const acTotal = acceptanceCriteria.length
  const acColor = acDone === acTotal && acTotal > 0 ? 'var(--green)' : acDone > 0 ? 'var(--amber)' : 'var(--text-3)'

  const latestReview = getLatestReview(task)

  // Data fetching
  const decisionsQuery = useRecentDecisions(projectId)
  const taskDecisions = (decisionsQuery.data ?? []).filter(d => d.taskRef === task.id)

  const krsQuery = useProjectKRs(projectId)
  const directKrQuery = useKeyResult(task.keyResultRef ?? null)
  const kr = task.keyResultRef
    ? directKrQuery.data ?? krsQuery.data?.get(task.keyResultRef)
    : undefined

  const missionsQuery = useMissions(projectId)
  const mission = kr ? (missionsQuery.data ?? []).find(m => m.id === kr.missionId) : undefined

  // Activity
  const activities = getTaskActivities(task)
  const [showOlderActivity, setShowOlderActivity] = useState(false)
  const visibleActivities = showOlderActivity ? activities : activities.slice(-8)
  const hiddenActivityCount = activities.length - visibleActivities.length


  const goalContent: ReactNode = !retry.isRetry && task.description
    ? <p style={{ fontFamily: SF_TEXT, fontSize: '14px', letterSpacing: '-0.224px', lineHeight: 1.47, color: 'var(--text-2)', margin: 0 }}>{task.description}</p>
    : null

  const reviewTone: Record<string, { fg: string; bg: string; label: string }> = {
    approved: { fg: 'var(--green)', bg: 'color-mix(in srgb, var(--green) 12%, transparent)', label: 'Approved' },
    retry: { fg: 'var(--amber)', bg: 'color-mix(in srgb, var(--amber) 12%, transparent)', label: 'Retry Requested' },
    failed: { fg: 'var(--red)', bg: 'color-mix(in srgb, var(--red) 10%, transparent)', label: 'Failed' },
    fail: { fg: 'var(--red)', bg: 'color-mix(in srgb, var(--red) 10%, transparent)', label: 'Failed' },
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header — dark section */}
      <div
        className="flex items-start gap-2 shrink-0"
        style={{ background: 'var(--bg-app)', padding: '16px' }}
      >
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <p
              className="uppercase"
              style={{ fontSize: '10px', fontWeight: 500, letterSpacing: '0.08em', lineHeight: 1.33, color: 'var(--text-3)' }}
            >
              Task
            </p>
            <span className="flex items-center gap-0.5" style={{ color: 'var(--text-3)' }}>
              <Hash size={9} />
              <span style={{ fontSize: '10px', fontFamily: 'monospace', letterSpacing: 0, lineHeight: 1.33 }}>
                {taskIdShort(task.id)}
              </span>
            </span>
          </div>
          <h2
            className="text-foreground line-clamp-2"
            style={{ fontFamily: SF_TEXT, fontSize: '17px', fontWeight: 600, lineHeight: 1.24, letterSpacing: '-0.374px' }}
          >
            {displayTitle}
          </h2>
        </div>
        <button
          onClick={onClose}
          className="shrink-0 text-muted-foreground hover:text-foreground transition-colors mt-0.5"
          style={{ padding: '4px', borderRadius: '50%', background: 'rgba(255,255,255,0.08)' }}
          aria-label="Close panel"
        >
          <X size={14} />
        </button>
      </div>

      {/* Summary zone — user-resizable, scrolls internally */}
      <div
        className="shrink-0 overflow-y-auto flex flex-col"
        style={{ height: summaryHeight, background: 'var(--bg-panel)', padding: '12px 16px', gap: '10px' }}
      >
        {/* Status badge */}
        <div className="flex items-center gap-2 flex-wrap">
          <Badge
            variant="outline"
            className="gap-1.5"
            style={{ borderRadius: '4px', fontSize: '12px', fontWeight: 600, letterSpacing: '-0.12px', lineHeight: 1.33 }}
          >
            <span
              style={{ width: 6, height: 6, borderRadius: '50%', background: color || 'var(--text-3)', flexShrink: 0, display: 'inline-block' }}
            />
            {label}
          </Badge>
        </div>

        {/* Retry feedback banner */}
        {retry.isRetry && retry.feedback && (
          <div
            style={{
              background: 'color-mix(in srgb, var(--amber) 10%, transparent)',
              border: '1px solid color-mix(in srgb, var(--amber) 30%, transparent)',
              borderRadius: '4px',
              padding: '8px 10px',
              fontSize: '11px',
              lineHeight: 1.47,
              color: 'var(--amber)',
              fontFamily: SF_TEXT,
              letterSpacing: '-0.08px',
            }}
          >
            {retry.attemptNum ? <span style={{ fontWeight: 600 }}>Attempt {retry.attemptNum}: </span> : null}
            {retry.feedback}
          </div>
        )}

        {/* Goal / Description */}
        {goalContent}

        {/* Summary */}
        {task.summary && (
          <div className="flex flex-col" style={{ gap: '4px' }}>
            <p
              className="uppercase"
              style={{ fontSize: '10px', fontWeight: 500, letterSpacing: '0.08em', lineHeight: 1.33, color: 'var(--text-3)', margin: 0 }}
            >
              Summary
            </p>
            <p
              style={{ fontFamily: SF_TEXT, fontSize: '13px', letterSpacing: '-0.08px', lineHeight: 1.47, color: 'var(--text-3)', margin: 0 }}
            >
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
              borderRadius: '4px',
              padding: '8px 10px',
            }}
          >
            <p style={{ fontSize: '10px', fontWeight: 500, letterSpacing: '0.08em', lineHeight: 1.33, color: 'var(--red)', marginBottom: '4px', textTransform: 'uppercase', display: 'flex', alignItems: 'center', gap: 4 }}>
              <AlertTriangle size={10} /> Error
            </p>
            <p style={{ fontFamily: 'monospace', fontSize: '11px', letterSpacing: 0, lineHeight: 1.47, color: 'var(--text-2)', wordBreak: 'break-all', margin: 0 }}>
              {task.error}
            </p>
          </div>
        )}

        {/* Block reason */}
        {task.status === 'blocked' && !!task.metadata?.blockReason && (
          <div
            style={{
              background: 'color-mix(in srgb, var(--amber) 10%, transparent)',
              border: '1px solid color-mix(in srgb, var(--amber) 30%, transparent)',
              borderRadius: '4px',
              padding: '8px 10px',
            }}
          >
            <p style={{ fontSize: '10px', fontWeight: 500, letterSpacing: '0.08em', lineHeight: 1.33, color: 'var(--amber)', marginBottom: '4px', textTransform: 'uppercase', display: 'flex', alignItems: 'center', gap: 4 }}>
              <AlertTriangle size={10} /> Blocked
            </p>
            <p style={{ fontSize: '11px', lineHeight: 1.47, color: 'var(--amber)', margin: 0 }}>
              {String(task.metadata.blockReason)}
            </p>
          </div>
        )}
      </div>

      {/* Stats strip — pinned, always visible */}
      <div
        className="shrink-0 flex flex-col"
        style={{ background: 'var(--bg-panel)', padding: '10px 16px', gap: '6px' }}
      >
        {assigneeLabel && (
          <div className="flex justify-between items-baseline">
            <span style={{ fontSize: '10px', fontWeight: 500, letterSpacing: '0.08em', lineHeight: 1.33, color: 'var(--text-3)', textTransform: 'uppercase' }}>Assignee</span>
            <span style={{ fontSize: '11px', fontWeight: 600, letterSpacing: '0.02em', lineHeight: 1.33, color: 'var(--text-2)', textTransform: 'none' }}>
              {assigneeLabel}
            </span>
          </div>
        )}
        <div className="flex justify-between items-baseline">
          <span style={{ fontSize: '10px', fontWeight: 500, letterSpacing: '0.08em', lineHeight: 1.33, color: 'var(--text-3)', textTransform: 'uppercase' }}>Created</span>
          <span className="flex items-center gap-1" style={{ fontSize: '11px', lineHeight: 1.33, color: 'var(--text-2)' }}>
            <Clock size={10} style={{ color: 'var(--text-3)' }} />
            {relativeTime(task.createdAt)}
          </span>
        </div>
        {task.completedAt && (
          <div className="flex justify-between items-baseline">
            <span style={{ fontSize: '10px', fontWeight: 500, letterSpacing: '0.08em', lineHeight: 1.33, color: 'var(--text-3)', textTransform: 'uppercase' }}>
              {duration ? 'Duration' : 'Completed'}
            </span>
            <span className="flex items-center gap-1" style={{ fontSize: '11px', lineHeight: 1.33, color: 'var(--text-2)' }}>
              <Clock size={10} style={{ color: 'var(--text-3)' }} />
              {duration ?? relativeTime(task.completedAt)}
            </span>
          </div>
        )}
        {task.taskKind && (
          <div className="flex justify-between items-baseline">
            <span style={{ fontSize: '10px', fontWeight: 500, letterSpacing: '0.08em', lineHeight: 1.33, color: 'var(--text-3)', textTransform: 'uppercase' }}>Kind</span>
            <span style={{ fontSize: '11px', lineHeight: 1.33, color: 'var(--text-2)' }}>{task.taskKind}</span>
          </div>
        )}
      </div>

      {/* Drag handle */}
      <div
        className="shrink-0 flex items-center justify-center select-none cursor-ns-resize"
        style={{ height: 10, background: 'var(--bg-panel)', borderTop: '1px solid var(--border)', borderBottom: '1px solid var(--border)' }}
        onMouseDown={handleResizeStart}
      >
        <div style={{ width: 20, height: 2, borderRadius: 1, background: 'var(--border-strong)' }} />
      </div>

      {/* Detail sections — takes all remaining space, scrolls independently */}
      <div
        className="flex-1 overflow-y-auto flex flex-col"
        style={{ background: 'var(--bg-panel)', padding: '14px 16px 24px', gap: '18px', minHeight: 0 }}
      >
        {/* 1. Acceptance Criteria */}
        {acTotal > 0 && (
          <CollapsibleSection
            storageKey="task-panel-ac"
            defaultOpen={true}
            label={<>Acceptance Criteria <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>{acDone}/{acTotal}</span></>}
            accent={acColor}
          >
            <ul className="m-0 p-0 list-none flex flex-col" style={{ gap: 0, borderTop: '1px solid var(--border)' }}>
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
                        width: 12,
                        height: 12,
                        marginTop: 2,
                        borderRadius: 3,
                        border: checked ? 'none' : '1px solid var(--border-strong)',
                        background: checked ? 'var(--green)' : 'transparent',
                        flexShrink: 0,
                      }}
                    >
                      {checked && <span style={{ color: 'white', fontSize: '8px', fontWeight: 700, lineHeight: 1 }}>✓</span>}
                    </span>
                    <span
                      style={{
                        fontFamily: SF_TEXT,
                        fontSize: '13px',
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

        {/* 2. Latest Review */}
        {latestReview && (() => {
          const tone = reviewTone[latestReview.decision] ?? { fg: 'var(--text-1)', bg: 'var(--bg-elevated)', label: latestReview.decision }
          return (
            <CollapsibleSection storageKey="task-panel-review" defaultOpen={true} label="Latest Review">
              <div style={{ borderTop: '1px solid var(--border)', paddingTop: 10 }}>
                <div className="flex items-center gap-2 flex-wrap">
                  <span
                    style={{
                      fontSize: '10px',
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
                    <span style={{ fontSize: '11px', color: 'var(--text-3)' }}>
                      by {latestReview.reviewerRole || latestReview.reviewedBy}
                    </span>
                  )}
                  {latestReview.reviewedAt && (
                    <span style={{ fontSize: '11px', color: 'var(--text-3)' }}>{relativeTime(latestReview.reviewedAt)}</span>
                  )}
                </div>
                {latestReview.feedback && (
                  <div
                    className="md-prose"
                    style={{ fontFamily: SF_TEXT, fontSize: '12px', lineHeight: 1.47, color: 'var(--text-2)', marginTop: 8 }}
                  >
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{latestReview.feedback}</ReactMarkdown>
                  </div>
                )}
              </div>
            </CollapsibleSection>
          )
        })()}

        {/* 3. Related — all cross-references in one clean section */}
        <RelatedSection items={[
          ...(kr ? [{ nodeId: kr.id, type: 'Key Result', title: kr.title, badge: `${Math.round(kr.progressPercent ?? 0)}%` }] : []),
          ...(mission ? [{ nodeId: mission.id, type: 'Mission', title: mission.title }] : []),
          ...taskDecisions.map(dec => ({
            nodeId: `decision:${dec.id}`,
            type: 'Decision',
            title: dec.title,
            ...(dec.confidence > 0 ? {
              badge: `${Math.round(dec.confidence * 100)}%`,
              badgeColor: dec.confidence >= 0.8 ? 'var(--green)' : dec.confidence >= 0.6 ? 'var(--amber)' : 'var(--red)',
            } : {}),
          })),
        ]} />

        {/* 6. Artifacts */}
        {task.artifacts && task.artifacts.length > 0 && (
          <CollapsibleSection
            storageKey="task-panel-artifacts"
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
                  <span style={{ fontSize: '11px', color: 'var(--accent)', fontFamily: 'monospace', wordBreak: 'break-all' }}>{a}</span>
                </div>
              ))}
            </div>
          </CollapsibleSection>
        )}

        {/* 8. Activity timeline */}
        {activities.length > 0 && (
          <CollapsibleSection storageKey="task-panel-activity" defaultOpen={true} label="Activity">
            <div style={{ borderTop: '1px solid var(--border)', paddingTop: 10 }}>
              <div className="flex flex-col" style={{ gap: '12px', paddingLeft: 10, borderLeft: '2px solid var(--border)' }}>
                {hiddenActivityCount > 0 && (
                  <button
                    onClick={() => setShowOlderActivity(v => !v)}
                    style={{ fontSize: '10px', fontWeight: 600, color: 'var(--accent)', background: 'transparent', border: 'none', cursor: 'pointer', padding: 0, textAlign: 'left' }}
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
    </div>
  )
}
