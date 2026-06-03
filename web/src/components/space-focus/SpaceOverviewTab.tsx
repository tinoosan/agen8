/**
 * SpaceOverviewTab — the "what's happening in this space right now"
 * landing page. Six data cards in a 3-column grid, each pulling from
 * existing service hooks and filtering to the current space when a
 * domain exposes project-level reads.
 */
import { useMemo, useState } from 'react'
import {
  Target,
  AlertCircle,
  ScrollText,
  ListChecks,
  Activity,
  BarChart2,
} from 'lucide-react'
import { toast } from 'sonner'

import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

import { useMissions, useKeyResults, useUpdateKRProgress } from '../../hooks/useMissions'
import { usePendingEscalations } from '../../hooks/useEscalations'
import { useRecentDecisions } from '../../hooks/useDecisions'
import { useSpaceDetail } from '../../hooks/useSpaceDetail'
import { useSpaceMemberList } from '../../hooks/useSpace'
import { useLocation } from 'wouter'
import { formatKRProgress } from '../../lib/missionUtils'
import { filterRemovedMemberEvents } from '../../lib/removedMemberLogs'
import { resolveEventTitle } from './spaceInspectorTitle'
import { resolveMemberIdentity } from '../../lib/memberIdentity'
import type {
  AgentEvent,
  DecisionView,
  EscalationView,
  KeyResultView,
  KeyResultStatus,
  MissionView,
  SpaceMember,
} from '../../lib/types'

// Events the overview feed hides. Two categories:
//   - Lifecycle markers (agent.turn.started) — same exclusion the
//     Inspector applies; pure plumbing with no rendered content.
//   - User messages — these are operator inputs, not agent activity.
//     The user can already see what they typed in the chat tab, and
//     they crowd out the actual work in this "what's the agent
//     doing" feed.
const HIDDEN_INSPECTOR_KINDS = new Set([
  'agent.turn.started',
  'user_message',
])

interface SpaceOverviewTabProps {
  spaceId: string
  projectId: string | null
  /**
   * Lightweight task counts the parent already has from space status.
   * The Work-in-Progress card derives its data from these counts plus
   * the parent's running roster, kept here only as a fallback so we
   * can render the empty state in a stable layout.
   */
  status?: { pending: number; active: number; done: number; totalCostUSD: number; totalTokens: number }
  showRemovedMemberLogs?: boolean
}

/* ------------------------------------------------------------------ */
/* Top-level layout                                                    */
/* ------------------------------------------------------------------ */

export default function SpaceOverviewTab({ spaceId, projectId, showRemovedMemberLogs = false }: SpaceOverviewTabProps) {
  return (
    <div className="space-overview-pane h-full overflow-auto">
      <div className="space-overview-content flex min-h-full w-full flex-col gap-3 p-6">
        {/* Members row spans the full width above the cards — primary
            entry into a member-scoped chat. */}
        <MembersStripCard projectId={projectId} spaceId={spaceId} />
        {/* 3 columns on lg, 2 on md, 1 on sm. Recent Activity spans
            2 cols on lg+ so the second row fills cleanly instead of
            leaving an empty third cell — and the activity feed gets
            the horizontal room its longer rows need. */}
        <div className="space-overview-grid grid flex-1 grid-cols-1 gap-3 md:grid-cols-2 lg:min-h-[520px] lg:grid-cols-3 lg:grid-rows-[auto_minmax(360px,1fr)]">
          <CurrentObjectiveCard spaceId={spaceId} projectId={projectId} />
          <AttentionNeededCard spaceId={spaceId} projectId={projectId} />
          <KeyDecisionsCard spaceId={spaceId} projectId={projectId} />
          <WorkInProgressCard spaceId={spaceId} projectId={projectId} />
          <div className="space-overview-wide-card min-h-0 lg:col-span-2 lg:h-full">
            <RecentActivityCard spaceId={spaceId} showRemovedMemberLogs={showRemovedMemberLogs} />
          </div>
        </div>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* Shared card chrome                                                  */
/* ------------------------------------------------------------------ */

function SectionCard({
  icon,
  title,
  iconColor,
  rightSlot,
  className,
  contentClassName,
  children,
}: {
  icon: React.ReactNode
  title: string
  iconColor?: string
  rightSlot?: React.ReactNode
  className?: string
  contentClassName?: string
  children: React.ReactNode
}) {
  return (
    <Card className={cn('space-overview-section-card border-0 shadow-[0_1px_3px_0_rgb(0_0_0/0.08)] flex min-h-[180px] flex-col', className)}>
      <CardHeader className="space-overview-card-header p-4 pb-2 flex flex-row items-center gap-2 space-y-0">
        <span
          className="flex items-center justify-center"
          style={{ color: iconColor ?? 'var(--accent)' }}
        >
          {icon}
        </span>
        <span className="text-[13px] font-semibold text-[var(--text-1)]">{title}</span>
        <span className="flex-1" />
        {rightSlot}
      </CardHeader>
      <CardContent className={cn('space-overview-card-content p-4 pt-2 flex-1 min-h-0', contentClassName)}>{children}</CardContent>
    </Card>
  )
}

function EmptyState({ message }: { message: string }) {
  return (
    <div className="text-[12px] text-[var(--text-3)] py-2">{message}</div>
  )
}

function CardSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="space-y-2">
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} className="h-[28px] rounded" />
      ))}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* Members strip                                                       */
/* ------------------------------------------------------------------ */

/**
 * Maps a SpaceMember.lifecycleState to a small visible status indicator.
 * "active" = green, "idle" = muted, "removed"/etc don't render here
 * (we filter removed members out before reaching this).
 */
function memberStatusColor(state: string | undefined): string {
  switch ((state ?? '').toLowerCase()) {
    case 'active': return 'var(--green)'
    case 'idle': return 'var(--text-3)'
    case 'pending': return 'var(--amber)'
    case 'error':
    case 'failed': return 'var(--red)'
    default: return 'var(--text-3)'
  }
}

/**
 * Members row — compact avatar circles for each member. Click an
 * avatar → navigates to the inspector tab for that member's space.
 * Status dot overlays the avatar corner. Role shown
 * as a tiny uppercase label under the name.
 */
function MembersStripCard({
  projectId,
  spaceId,
}: {
  projectId: string | null
  spaceId: string | null
}) {
  const [, navigate] = useLocation()
  const membersQuery = useSpaceMemberList({
    spaceId: spaceId ?? undefined,
    enabled: !!spaceId,
  })
  // Filter to active members so the strip doesn't carry historical
  // ghosts. Sort by most-recently-seen so the user's last
  // conversation partner shows up first.
  const members = useMemo(
    () =>
      (membersQuery.data ?? [])
        .filter(m => (m.lifecycleState ?? '').toLowerCase() !== 'removed')
        .slice()
        .sort((a, b) => (b.lastSeenAt ?? '').localeCompare(a.lastSeenAt ?? '')),
    [membersQuery.data],
  )

  if (membersQuery.isLoading) {
    return (
      <Card className="space-overview-members-card border-0 shadow-[0_1px_3px_0_rgb(0_0_0/0.08)]">
        <CardHeader className="space-overview-card-header p-4 pb-2 flex flex-row items-center gap-2 space-y-0">
          <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--text-3)]">
            Members
          </span>
        </CardHeader>
        <CardContent className="space-overview-members-content p-3 pt-0">
          <div className="flex gap-4 pb-1">
            {Array.from({ length: 3 }, (_, i) => (
              <div key={i} className="flex flex-col items-center gap-[5px] p-1" style={{ minWidth: 56 }}>
                <Skeleton className="w-9 h-9 rounded-full" />
                <Skeleton className="h-[13px] w-10 rounded" />
                <Skeleton className="h-[10px] w-8 rounded" />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <>
      <Card className="space-overview-members-card border-0 shadow-[0_1px_3px_0_rgb(0_0_0/0.08)]">
        <CardHeader className="space-overview-card-header p-4 pb-2 flex flex-row items-center gap-2 space-y-0">
          <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--text-3)]">
            Members
          </span>
          <span className="flex-1" />
          {members.length > 0 && (
            <span className="text-[11px] text-[var(--text-3)] tabular-nums">
              {members.length}
            </span>
          )}
        </CardHeader>
        <CardContent className="space-overview-members-content p-3 pt-0">
          <div className="flex gap-4 overflow-x-auto pb-1">
            {members.map(member => (
              <MemberAvatar
                key={member.id}
                member={member}
                onSelect={() => {
                  if (!projectId || !spaceId) return
                  navigate(
                    `/project/${encodeURIComponent(projectId)}/space/${encodeURIComponent(spaceId)}?tab=inspector`,
                  )
                }}
              />
            ))}
          </div>
        </CardContent>
      </Card>
    </>
  )
}

function MemberAvatar({
  member,
  onSelect,
}: {
  member: SpaceMember
  onSelect: () => void
}) {
  const identity = resolveMemberIdentity(member.id)
  const Icon = identity.icon
  const memberType = (member.memberType ?? '').toLowerCase()
  const typeLabel =
    memberType === 'coordinator'
      ? 'Coord'
      : memberType === 'worker'
      ? 'Worker'
      : member.memberType.replace(/_/g, ' ')
  const statusColor = memberStatusColor(member.lifecycleState)

  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        'flex flex-col items-center gap-[5px] p-1 rounded-[10px]',
        'bg-transparent border-0 cursor-pointer transition-colors',
        'hover:bg-[var(--bg-hover)]',
      )}
      style={{ minWidth: 56 }}
      title={`Inspect ${member.displayName}`}
    >
      {/* Avatar with status dot overlay */}
      <div className="relative">
        <div
          className="flex items-center justify-center w-9 h-9 rounded-full"
          style={{
            backgroundColor: `color-mix(in srgb, ${identity.colorVar} 18%, transparent)`,
            color: identity.colorVar,
          }}
          aria-hidden
        >
          <Icon size={16} strokeWidth={2} />
        </div>
        <span
          aria-hidden
          className="absolute bottom-0 right-0 h-[7px] w-[7px] rounded-full"
          style={{
            backgroundColor: statusColor,
            border: '2px solid var(--bg-panel, var(--bg-surface))',
          }}
          title={member.lifecycleState}
        />
      </div>
      <span className="text-[11px] font-medium text-[var(--text-2)] max-w-[64px] truncate text-center capitalize">
        {member.displayName}
      </span>
      <span className="text-[9px] font-semibold uppercase tracking-[0.04em] text-[var(--text-3)] -mt-0.5">
        {typeLabel}
      </span>
    </button>
  )
}

/* ------------------------------------------------------------------ */
/* 1. Current Objective                                                */
/* ------------------------------------------------------------------ */

function krStatusVariant(status: KeyResultStatus): 'success' | 'warning' | 'info' | 'danger' | 'secondary' {
  switch (status) {
    case 'on_track': return 'success'
    case 'at_risk': return 'warning'
    case 'completed': return 'success'
    case 'dropped': return 'danger'
    default: return 'secondary'
  }
}

function CurrentObjectiveCard({ spaceId, projectId }: { spaceId: string; projectId: string | null }) {
  // Scope: most recently updated active mission for this project that
  // has any KR assigned to this space.
  const missionsQuery = useMissions(projectId, 'active')
  const missions = missionsQuery.data ?? []
  const primaryMission = missions[0] ?? null

  return (
    <SectionCard
      icon={<Target size={14} />}
      title="Current Objective"
      iconColor="var(--accent)"
    >
      {missionsQuery.isLoading ? (
        <CardSkeleton rows={2} />
      ) : !primaryMission ? (
        <EmptyState message="No active mission. Set one in the Strategy view to give this space a north star." />
      ) : (
        <CurrentObjectiveBody mission={primaryMission} spaceId={spaceId} />
      )}
    </SectionCard>
  )
}

function CurrentObjectiveBody({ mission, spaceId }: { mission: MissionView; spaceId: string }) {
  const krQuery = useKeyResults(mission.id)
  const krs = krQuery.data ?? []
  // Show the space's KR if present; otherwise the first KR — gives the
  // user a sense of progress either way. KeyResultView.spaceId is the
  // current owning space; ownerSpaceName may also indicate ownership.
  const spaceKR = useMemo(() => {
    const ownedBySpace = krs.find(kr =>
      (kr.spaceId && kr.spaceId === spaceId) ||
      (kr.ownerSpaceName && kr.ownerSpaceName === spaceId),
    )
    if (ownedBySpace) return ownedBySpace
    return krs[0] ?? null
  }, [krs, spaceId])

  return (
    <div className="space-y-3">
      <div className="text-[14px] font-medium text-[var(--text-1)] leading-snug">
        {mission.title}
      </div>
      {spaceKR ? <KeyResultRow kr={spaceKR} missionId={mission.id} /> : (
        <EmptyState message="No key results assigned yet." />
      )}
    </div>
  )
}

function KeyResultRow({ kr, missionId }: { kr: KeyResultView; missionId: string }) {
  const updateProgress = useUpdateKRProgress()
  const [isEditing, setIsEditing] = useState(false)
  const [progressValue, setProgressValue] = useState(() => String(kr.currentValue ?? 0))
  const [progressNote, setProgressNote] = useState('')

  // formatKRProgress returns either "X / Y unit" / "Done" etc, or
  // null for binary KRs. We render the percent badge from
  // progressPercent and the formatted string as a sub-label so binary
  // KRs still surface meaningfully.
  const valueLabel = formatKRProgress({
    measurementType: kr.measurementType,
    currentValue: kr.currentValue,
    targetValue: kr.targetValue,
    unit: kr.unit,
  })
  const percent = Math.max(0, Math.min(100, Math.round(kr.progressPercent || 0)))
  const variant = krStatusVariant(kr.status)

  async function handleProgressSubmit() {
    const value = Number(progressValue)
    const note = progressNote.trim()
    if (!Number.isFinite(value)) {
      toast.error('Progress must be a number')
      return
    }
    if (!note) {
      toast.error('Add a short progress note')
      return
    }

    try {
      await updateProgress.mutateAsync({
        keyResultId: kr.id,
        missionId,
        value,
        note,
      })
      toast.success('Progress updated')
      setIsEditing(false)
      setProgressNote('')
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to update progress')
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <span className="flex-1 truncate text-[12px] text-[var(--text-2)]">{kr.title}</span>
        <Badge variant={variant} className="text-[10px] px-1.5 py-0 tabular-nums">
          {percent}%
        </Badge>
      </div>
      <div className="h-1 bg-[var(--bg-elevated)] rounded-full overflow-hidden">
        <div
          className="h-full rounded-full"
          style={{
            width: `${percent}%`,
            backgroundColor:
              variant === 'success' ? 'var(--green)' :
              variant === 'warning' ? 'var(--amber)' :
              variant === 'danger' ? 'var(--red)' :
              'var(--accent)',
          }}
        />
      </div>
      {valueLabel && (
        <div className="text-[11px] text-[var(--text-3)] tabular-nums">{valueLabel}</div>
      )}
      {isEditing ? (
        <div className="space-y-2 rounded-md border border-[var(--border)] bg-[var(--bg-elevated)] p-2">
          <div className="grid grid-cols-[88px_1fr] gap-2">
            <Input
              aria-label="Progress value"
              className="h-8 text-[12px]"
              inputMode="decimal"
              value={progressValue}
              onChange={(event) => setProgressValue(event.target.value)}
            />
            <Input
              aria-label="Progress note"
              className="h-8 text-[12px]"
              placeholder="What changed?"
              value={progressNote}
              onChange={(event) => setProgressNote(event.target.value)}
            />
          </div>
          <div className="flex justify-end gap-1.5">
            <Button
              type="button"
              variant="ghost"
              size="xs"
              onClick={() => {
                setIsEditing(false)
                setProgressValue(String(kr.currentValue ?? 0))
                setProgressNote('')
              }}
              disabled={updateProgress.isPending}
            >
              Cancel
            </Button>
            <Button
              type="button"
              size="xs"
              onClick={handleProgressSubmit}
              disabled={updateProgress.isPending}
            >
              Save
            </Button>
          </div>
        </div>
      ) : (
        <Button
          type="button"
          variant="ghost"
          size="xs"
          className="h-7 px-2 text-[11px]"
          onClick={() => {
            setProgressValue(String(kr.currentValue ?? 0))
            setIsEditing(true)
          }}
        >
          <BarChart2 size={12} />
          Update progress
        </Button>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* 2. Attention Needed                                                 */
/* ------------------------------------------------------------------ */

function AttentionNeededCard({ spaceId, projectId }: { spaceId: string; projectId: string | null }) {
  // Scope: pending escalations for this project, narrowed client-side
  // to this space.
  const escQuery = usePendingEscalations(projectId)
  const escalations = useMemo(
    () => (escQuery.data ?? []).filter(e =>
      !e.spaceId || !spaceId || e.spaceId === spaceId,
    ),
    [escQuery.data, spaceId],
  )

  return (
    <SectionCard
      icon={<AlertCircle size={14} />}
      title="Attention Needed"
      iconColor="var(--amber)"
      rightSlot={
        escalations.length > 0 && (
          <Badge variant="warning" className="text-[10px] px-1.5 py-0 tabular-nums">
            {escalations.length}
          </Badge>
        )
      }
    >
      {escQuery.isLoading ? (
        <CardSkeleton rows={2} />
      ) : escalations.length === 0 ? (
        <EmptyState message="Nothing waiting on you." />
      ) : (
        <div className="space-y-2">
          {escalations.slice(0, 3).map(esc => (
            <EscalationRow key={esc.id} esc={esc} />
          ))}
          {escalations.length > 3 && (
            <div className="text-[11px] text-[var(--text-3)] pt-1">
              +{escalations.length - 3} more
            </div>
          )}
        </div>
      )}
    </SectionCard>
  )
}

function EscalationRow({ esc }: { esc: EscalationView }) {
  return (
    <div className="space-y-0.5">
      <div className="flex items-center gap-2">
        <span className="flex-1 truncate text-[12px] font-medium text-[var(--text-1)]">
          {esc.title}
        </span>
        {esc.urgency && (
          <Badge
            variant={esc.urgency === 'critical' ? 'danger' : esc.urgency === 'high' ? 'warning' : 'secondary'}
            className="text-[10px] px-1.5 py-0 capitalize"
          >
            {esc.urgency}
          </Badge>
        )}
      </div>
      {esc.description && (
        <div className="text-[11px] text-[var(--text-3)] truncate">{esc.description}</div>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* 3. Key Decisions                                                    */
/* ------------------------------------------------------------------ */

function KeyDecisionsCard({ spaceId, projectId }: { spaceId: string; projectId: string | null }) {
  // Scope: project + spaceId filter (the decision filter accepts spaceId
  // today). Will switch to spaceId once decision.list supports it.
  const decisionsQuery = useRecentDecisions(projectId, { spaceId })
  const decisions = decisionsQuery.data ?? []

  return (
    <SectionCard
      icon={<ScrollText size={14} />}
      title="Key Decisions"
      iconColor="var(--violet, #a78bfa)"
    >
      {decisionsQuery.isLoading ? (
        <CardSkeleton rows={3} />
      ) : decisions.length === 0 ? (
        <EmptyState message="No decisions logged yet. Decisions made by the agents will appear here." />
      ) : (
        <div className="space-y-2">
          {decisions.slice(0, 4).map(d => (
            <DecisionRow key={d.id} decision={d} />
          ))}
        </div>
      )}
    </SectionCard>
  )
}

function DecisionRow({ decision }: { decision: DecisionView }) {
  const date = decision.createdAt ? new Date(decision.createdAt) : null
  const dateLabel = date ? date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : ''
  return (
    <div className="space-y-0.5">
      <div className="text-[12px] font-medium text-[var(--text-1)] truncate">
        {decision.title}
      </div>
      <div className="flex items-center gap-2 text-[11px] text-[var(--text-3)]">
        {/*
         * Prefer the resolved member display name, fall back to the
         * raw memberId only if the registry lookup didn't return a
         * name, and finally to the role/source label. The bare
         * sourceIdentity field is no longer surfaced — that was
         * leaking the implementation-detail member uuid into list
         * views.
         */}
        <span className="truncate">{decision.memberName?.trim() || decision.memberId?.trim() || decision.source}</span>
        {dateLabel && (
          <>
            <span>·</span>
            <span className="tabular-nums">{dateLabel}</span>
          </>
        )}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* 4. Work in Progress                                                 */
/* ------------------------------------------------------------------ */

interface TaskListEntry {
  task: { id: string; description: string; status: string; assignedTo?: string }
}

function statusVariant(status: string): 'info' | 'success' | 'warning' | 'danger' | 'secondary' {
  switch (status) {
    case 'active': return 'info'
    case 'pending': return 'warning'
    case 'blocked': return 'danger'
    case 'in_review': return 'info'
    case 'succeeded': return 'success'
    default: return 'secondary'
  }
}

function WorkInProgressCard({ spaceId }: { spaceId: string; projectId: string | null }) {
  // Scope: tasks via task.list filtered by spaceId. The hook layer
  // doesn't expose a clean per-space task list yet; calling the RPC
  // directly with a small fetch wrapper. We scope by spaceId server-
  // side (already supported on TaskFilter post-cutover) and limit to
  // non-terminal statuses to focus the card on "work in flight".
  const tasksQuery = useSpaceTaskList(spaceId, ['pending', 'active', 'blocked', 'in_review'])
  const tasks = tasksQuery.data ?? []

  return (
    <SectionCard
      icon={<ListChecks size={14} />}
      title="Work in Progress"
      iconColor="var(--blue)"
      rightSlot={
        tasks.length > 0 && (
          <Badge variant="info" className="text-[10px] px-1.5 py-0 tabular-nums">
            {tasks.length}
          </Badge>
        )
      }
    >
      {tasksQuery.isLoading ? (
        <CardSkeleton rows={3} />
      ) : tasks.length === 0 ? (
        <EmptyState message="No active tasks. Send a message in the Chat tab to kick something off." />
      ) : (
        <div className="space-y-2">
          {tasks.slice(0, 4).map(t => (
            <TaskRow key={t.task.id} entry={t} />
          ))}
          {tasks.length > 4 && (
            <div className="text-[11px] text-[var(--text-3)] pt-1">
              +{tasks.length - 4} more in the Board tab
            </div>
          )}
        </div>
      )}
      <span className="hidden">{spaceId}</span>
    </SectionCard>
  )
}

function TaskRow({ entry }: { entry: TaskListEntry }) {
  const variant = statusVariant(entry.task.status)
  return (
    <div className="space-y-0.5">
      <div className="flex items-center gap-2">
        <span className="flex-1 truncate text-[12px] text-[var(--text-1)]">
          {entry.task.description || 'Untitled task'}
        </span>
        <Badge variant={variant} className="text-[10px] px-1.5 py-0 capitalize">
          {entry.task.status.replace('_', ' ')}
        </Badge>
      </div>
      {entry.task.assignedTo && (
        <div className="text-[11px] text-[var(--text-3)] truncate">
          Assigned to {entry.task.assignedTo}
        </div>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* 5. Recent Activity                                                  */
/* ------------------------------------------------------------------ */

function RecentActivityCard({ spaceId, showRemovedMemberLogs }: { spaceId: string | null; showRemovedMemberLogs: boolean }) {
  // Mini-Inspector: same data source the Inspector tab uses, just
  // capped to the most recent N rows and filtered through the same
  // hidden-kinds exclusion so this card mirrors what the user sees
  // there. useSpaceDetail polls space.detail for this space and
  // projects entries into AgentEvent[]. Tool calls, agent text,
  // decisions, errors all flow through here.
  const { inspectorEvents, query } = useSpaceDetail(spaceId)
  const membersQuery = useSpaceMemberList({ spaceId, enabled: !!spaceId, includeRemoved: true })
  const visibleEvents = useMemo(
    () => filterRemovedMemberEvents(
      inspectorEvents.filter(e => !HIDDEN_INSPECTOR_KINDS.has((e.kind ?? '').toLowerCase())),
      membersQuery.data,
      showRemovedMemberLogs,
    ),
    [inspectorEvents, membersQuery.data, showRemovedMemberLogs],
  )
  // inspectorEvents is chronological; reverse for "most recent first"
  // which is the conventional reading order for an activity feed.
  const recent = useMemo(() => [...visibleEvents].reverse(), [visibleEvents])

  return (
    <SectionCard
      icon={<Activity size={14} />}
      title="Recent Activity"
      iconColor="var(--green)"
      className="h-full"
      contentClassName="overflow-hidden"
    >
      {query.isLoading && recent.length === 0 ? (
        <CardSkeleton rows={4} />
      ) : recent.length === 0 ? (
        <EmptyState message="No agent activity yet. Tool calls and agent actions will appear here." />
      ) : (
        <div className="h-full space-y-2 overflow-y-auto pr-1">
          {recent.slice(0, 8).map(event => (
            <EventRow key={event.id} event={event} />
          ))}
        </div>
      )}
    </SectionCard>
  )
}

/**
 * Format an absolute timestamp as a compact relative time:
 *   "just now" / "5m" / "1h" / "3h" / "2d"
 *
 * Bias toward the shortest legible form so a feed of these reads
 * scanably without taking horizontal space. Falls back to a short
 * date for anything older than a week.
 */
function formatRelativeTime(iso: string): string {
  const ts = Date.parse(iso)
  if (!Number.isFinite(ts)) return ''
  const seconds = Math.floor((Date.now() - ts) / 1000)
  if (seconds < 30) return 'just now'
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${days}d ago`
  return new Date(ts).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

/**
 * Trim a summary fragment to keep activity rows compact. Single line,
 * collapsed whitespace, ellipsized past the budget.
 */
function shortObject(value: string | undefined | null, max = 60): string {
  if (!value) return ''
  const single = value.replace(/\s+/g, ' ').trim()
  if (single.length <= max) return single
  return single.slice(0, max - 1).trimEnd() + '…'
}

/**
 * Attempt to parse a tool's outputPreview into a meaningful object.
 *
 * Backend tool results land here as JSON-encoded strings, sometimes
 * with a `body` field that is itself a JSON-encoded string (the args
 * the agent sent). Dumping these raw into the activity feed produces
 * unreadable rows like `John pulled metrics {"ok":true,"op":...}`.
 *
 * This walks one or two levels deep, extracting the fields the feed
 * actually wants to show — action/op verb, args (query/path/count),
 * and a success indicator. Returns an object with the most useful
 * fields, all optional.
 */
function parseToolPayload(raw: string | undefined | null): {
  ok?: boolean
  op?: string
  action?: string
  path?: string
  query?: string
  count?: number
  /** Free-form summary the caller can render when nothing more
   *  specific is available. Already short and stripped of JSON. */
  summary?: string
} {
  if (!raw || typeof raw !== 'string') return {}
  const trimmed = raw.trim()
  // Looks like JSON (starts with { or [) but might be truncated
  // mid-stream — outputPreview gets capped at a byte budget, so
  // structurally valid JSON often arrives partial. If parsing fails
  // on a JSON-shaped input we explicitly return an empty payload
  // rather than leaking the raw braces into the activity feed.
  const looksJSONish = trimmed.startsWith('{') || trimmed.startsWith('[')
  if (!looksJSONish) {
    // Plain text — safe to surface as a summary candidate.
    return { summary: shortObject(trimmed, 60) }
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    // JSON-shaped but unparseable. Treat as opaque structured data
    // and render no object — the verb alone tells the user a tool
    // ran without dumping raw machine output into a human feed.
    return {}
  }
  if (!parsed || typeof parsed !== 'object') return {}
  const obj = parsed as Record<string, unknown>

  // Some payloads wrap the actual args in a `body` field that is
  // itself a JSON-encoded string. Unwrap once if it parses.
  let inner: Record<string, unknown> = obj
  if (typeof obj.body === 'string') {
    try {
      const innerParsed = JSON.parse(obj.body)
      if (innerParsed && typeof innerParsed === 'object') {
        inner = { ...obj, ...(innerParsed as Record<string, unknown>) }
      }
    } catch {
      // body wasn't JSON; leave inner as-is.
    }
  }

  const pickString = (...keys: string[]): string | undefined => {
    for (const k of keys) {
      const v = inner[k]
      if (typeof v === 'string' && v.trim()) return v.trim()
    }
    return undefined
  }
  const pickNumber = (...keys: string[]): number | undefined => {
    for (const k of keys) {
      const v = inner[k]
      if (typeof v === 'number' && Number.isFinite(v)) return v
      if (typeof v === 'string' && /^\d+$/.test(v)) return Number(v)
    }
    return undefined
  }
  const pickBool = (...keys: string[]): boolean | undefined => {
    for (const k of keys) {
      const v = inner[k]
      if (typeof v === 'boolean') return v
      if (v === 'true') return true
      if (v === 'false') return false
    }
    return undefined
  }

  return {
    ok: pickBool('ok', 'success'),
    op: pickString('op', 'operation'),
    action: pickString('action', 'verb', 'method'),
    path: pickString('path', 'target', 'file'),
    query: pickString('query', 'pattern', 'q', 'search'),
    count: pickNumber('count', 'total', 'numItems', 'num_items', 'len'),
  }
}

/**
 * Build a compact object string from a parsed tool payload. Picks
 * the most informative bits in order: explicit query/path > count
 * with op/action context > raw summary. Designed to read as the
 * predicate of a sentence ("for X", "5 items", etc).
 */
function summarizePayload(payload: ReturnType<typeof parseToolPayload>): string {
  if (payload.query) return `for "${shortObject(payload.query, 40)}"`
  if (payload.path) return shortObject(payload.path, 50)
  if (typeof payload.count === 'number') {
    if (payload.action) return `${payload.count} ${payload.action}`
    if (payload.op) return `${payload.count} ${payload.op}`
    return `${payload.count} items`
  }
  if (payload.action && payload.op && payload.action !== payload.op) {
    return `(${payload.op} · ${payload.action})`
  }
  if (payload.action) return `(${payload.action})`
  if (payload.op) return `(${payload.op})`
  // ok flag alone — only show on failures, since success is the
  // assumed default and rendering "(ok)" everywhere is noise.
  if (payload.ok === false) return '(failed)'
  return shortObject(payload.summary ?? '', 50)
}

/**
 * Summarize an AgentEvent into "actor + verb + object" for the
 * Recent Activity feed. The Inspector tab can afford a structured
 * card per event; the overview card has one-line rows, so we boil
 * each event down to its narrative shape.
 *
 * Falls through to resolveEventTitle (the Inspector's title helper)
 * when no kind-specific shape applies — guarantees we always have
 * something readable rather than the bare kind string.
 */
function summarizeEvent(event: AgentEvent): { verb: string; object: string } {
  const kind = (event.kind ?? '').toLowerCase()
  const data = event.data ?? {}
  const action = (data.action ?? data.op ?? '').toLowerCase()
  // Tool outputs land here as JSON blobs. Parse them once up front
  // so every case below can pull structured fields (op/action/
  // query/path/count) instead of dumping raw JSON. Falls back to
  // summary when outputPreview is plain text.
  const payload = parseToolPayload(event.outputPreview ?? event.textPreview ?? '')
  // preview = the cleaned-up object phrase derived from the parsed
  // payload (e.g. `for "foo"`, `5 items`, `(failed)`). Used wherever
  // we don't have a more specific arg from event.data itself.
  const preview = summarizePayload(payload)
  const path = (event.path || data.path || data.target || payload.path || '').trim()

  // File operations: tool/read, tool/write, tool/edit, tool/grep,
  // glob, etc all expose a path. Render as "verb path-or-pattern".
  if (kind === 'tool/read' || kind === 'read_file' || (kind === 'tool_call' && action === 'read') || action === 'read') {
    return { verb: 'read', object: shortObject(path) || preview }
  }
  if (kind === 'tool/write' || kind === 'write_file' || action === 'write' || action === 'create') {
    return { verb: 'wrote', object: shortObject(path) || preview }
  }
  if (kind === 'tool/edit' || kind === 'edit_file' || action === 'edit' || action === 'patch') {
    return { verb: 'edited', object: shortObject(path) || preview }
  }
  if (kind === 'tool/grep' || kind === 'search_files' || action === 'grep' || action === 'search') {
    const query = shortObject(data.query ?? data.pattern ?? '')
    if (query) return { verb: 'searched for', object: `"${query}"` }
    return { verb: 'searched', object: shortObject(path) || preview }
  }
  if (kind === 'tool/glob' || kind === 'list_files' || action === 'glob' || action === 'list_dir') {
    return { verb: 'listed', object: shortObject(path || data.pattern) || preview }
  }
  if (kind === 'tool/bash' || kind === 'shell_exec' || kind === 'bash' || action === 'bash' || action === 'shell') {
    return { verb: 'ran', object: shortObject(data.command ?? data.cmd ?? '') || 'shell command' }
  }

  // Graph query: agen8's domain-specific knowledge graph tool. The
  // query string lives on the inner payload (data.query or
  // payload.query); render the action verb if present so we can
  // distinguish "describe", "list", "search", etc.
  if (kind === 'graph_query' || kind.includes('graph_query') || kind.includes('graph-query')) {
    const query = data.query ?? payload.query
    const act = (payload.action ?? '').toLowerCase()
    if (query) return { verb: 'queried the graph', object: `for "${shortObject(query, 50)}"` }
    if (act) return { verb: `ran graph ${act}`, object: '' }
    return { verb: 'queried the graph', object: '' }
  }

  // Metrics / project metrics — payload usually has op="metrics"
  // and an action verb like "summary" / "list". Surface the action.
  if (kind.includes('metric')) {
    if (payload.action) return { verb: `pulled ${payload.action} metrics`, object: '' }
    if (typeof payload.count === 'number') return { verb: 'pulled metrics', object: `${payload.count} entries` }
    return { verb: 'pulled metrics', object: payload.ok === false ? '(failed)' : '' }
  }

  // Plan / plan list / plan tool — action is typically list/get/
  // create/complete in either the data or the parsed payload.
  if (kind === 'plan' || kind.startsWith('plan ') || kind.startsWith('plan/') || kind.startsWith('plan_')) {
    const act = (action || payload.action || '').toLowerCase()
    if (act === 'list' || kind.includes('list')) return { verb: 'listed plans', object: typeof payload.count === 'number' ? `${payload.count} entries` : '' }
    if (act === 'get' || act === 'describe') return { verb: 'reviewed the plan', object: '' }
    if (act === 'create') return { verb: 'created a plan', object: '' }
    if (act === 'complete') return { verb: 'completed a plan', object: '' }
    return { verb: 'reviewed the plan', object: act ? `(${act})` : '' }
  }

  // Space catalog (agen8/space) — introspection tool. Action is
  // typically "list" or "get". Render the action so users can tell
  // the difference between "looked up the catalog" vs "fetched a
  // single space".
  if (kind === 'space' || kind.endsWith('/space') || kind.includes('list_spaces')) {
    const act = (action || payload.action || '').toLowerCase()
    if (act === 'list') return { verb: 'listed spaces', object: typeof payload.count === 'number' ? `${payload.count} spaces` : '' }
    if (act === 'get' || act === 'describe') return { verb: 'looked up a space', object: '' }
    return { verb: 'looked up', object: 'space catalog' }
  }

  // Mission tool calls (agen8/mission, mission.list, etc).
  if (kind === 'mission' || kind.startsWith('mission/') || kind.startsWith('mission_')) {
    const act = (action || payload.action || '').toLowerCase()
    if (act === 'list' || kind.includes('list')) return { verb: 'listed missions', object: typeof payload.count === 'number' ? `${payload.count} missions` : '' }
    if (act === 'create' || kind.includes('create')) return { verb: 'created a mission', object: '' }
    return { verb: 'inspected mission', object: '' }
  }

  // Generic *list* — anything ending in list/listed surfaces as a
  // listing verb with the entity name pulled from the kind prefix.
  if (kind.includes('list')) {
    const subject = kind
      .replace(/[._/]list.*$/, '')
      .replace(/[._/-]/g, ' ')
      .trim()
    return { verb: subject ? `listed ${subject}` : 'listed', object: preview }
  }

  // Task lifecycle events: created/completed/blocked etc.
  if (kind === 'task' || kind.startsWith('task.')) {
    const taskTitle = shortObject(
      String(data.title ?? data.taskTitle ?? data.task_title ?? '').trim() || resolveEventTitle(event),
    )
    const taskAction = (data.action ?? data.op ?? '').toLowerCase() ||
      (kind.startsWith('task.') ? kind.slice(5) : '')
    if (taskAction === 'create' || taskAction === 'created') return { verb: 'created task', object: taskTitle }
    if (taskAction === 'complete' || taskAction === 'completed' || taskAction === 'submit' || taskAction === 'done')
      return { verb: 'completed', object: taskTitle }
    if (taskAction === 'block' || taskAction === 'blocked') return { verb: 'blocked on', object: taskTitle }
    if (taskAction === 'claim' || taskAction === 'claimed' || taskAction === 'start' || taskAction === 'started')
      return { verb: 'started', object: taskTitle }
    return { verb: 'updated task', object: taskTitle }
  }

  // Messages: agent_message, user_message — surface "said" + first
  // line of the body so the feed reads like a conversation digest.
  // Budget intentionally generous (240 chars ≈ ~3 visual lines on a
  // typical card width). The row's CSS uses line-clamp-2 to do the
  // visual truncation so longer messages get to fill both lines on
  // wide screens instead of being chopped at 60 chars in the data.
  if (kind === 'agent_message' || kind === 'message.received' || kind === 'agent_message_received' || kind === 'agent_speak' || kind === 'model_response') {
    const body = shortObject(event.outputPreview ?? event.textPreview ?? data.body ?? '', 240)
    return body ? { verb: 'said', object: `"${body}"` } : { verb: 'sent message', object: '' }
  }
  if (kind === 'user_message') {
    const body = shortObject(event.textPreview ?? data.body ?? '', 240)
    return body ? { verb: 'asked', object: `"${body}"` } : { verb: 'sent message', object: '' }
  }

  // Decisions / tool/decision: surface the title.
  if (kind.includes('decision')) {
    const title = shortObject(String(data.title ?? '').trim() || resolveEventTitle(event))
    return { verb: 'logged decision', object: title }
  }

  // Errors get a verb that flags severity without leaning on the
  // dot color alone (helps users with reduced color sensitivity).
  if (kind.includes('error') || kind.includes('failed')) {
    const reason = shortObject(event.error ?? data.error ?? data.message ?? resolveEventTitle(event))
    return { verb: 'hit an error', object: reason }
  }

  // Generic tool-shaped kinds (anything namespaced via "/" or
  // starting with "tool"). Use the resolved title as the tool name
  // and "ran" as the verb so the row reads as a sentence rather
  // than just a tool ID. outputPreview becomes the object so the
  // row tells the user what came back.
  if (kind.startsWith('tool') || kind.includes('/')) {
    const toolLabel = shortObject(resolveEventTitle(event), 40).toLowerCase()
    return {
      verb: toolLabel ? `ran ${toolLabel}` : 'ran a tool',
      object: preview,
    }
  }

  // Final fallback: prepend "ran" if the title doesn't already read
  // like a verb. We err on the side of always producing a verb-shaped
  // row so the feed is grammatically consistent.
  const humanized = shortObject(resolveEventTitle(event), 40).toLowerCase()
  return {
    verb: humanized ? `ran ${humanized}` : 'did something',
    object: preview,
  }
}

function EventRow({ event }: { event: AgentEvent }) {
  const role =
    String(event.data?.role ?? event.from ?? '').trim() ||
    'agent'
  // Member identity (icon + color) seeded off the actor name keeps
  // the Recent Activity feed visually consistent with the chat
  // bubbles (which also seed off role).
  const identity = resolveMemberIdentity(role)
  const Icon = identity.icon
  const { verb, object } = summarizeEvent(event)
  const time = formatRelativeTime(event.startedAt ?? '')

  return (
    <div className="flex items-start gap-2.5">
      {/* Avatar — small tinted circle in the actor's identity color.
          Same pattern as the chat agent bubble for visual continuity. */}
      <div
        className="shrink-0 mt-[1px] flex items-center justify-center w-[20px] h-[20px] rounded-full"
        style={{
          backgroundColor: `color-mix(in srgb, ${identity.colorVar} 18%, transparent)`,
          color: identity.colorVar,
        }}
        aria-hidden
      >
        <Icon size={11} strokeWidth={2} />
      </div>
      <div className="flex-1 min-w-0">
        {/* Two-line clamp instead of single-line truncate. Recent
            Activity now spans 2 cols on lg+ so the row has plenty of
            horizontal real estate; clamping at 1 line cut messages
            mid-thought even when the column was 800+px wide. Two
            lines absorbs ~95% of natural agent-message lengths
            without the feed running away. */}
        <div className="text-[12px] text-[var(--text-1)] leading-snug line-clamp-2">
          <span className="font-medium capitalize" style={{ color: identity.colorVar }}>
            {role}
          </span>
          <span className="text-[var(--text-2)]"> {verb}</span>
          {object && (
            <span className="text-[var(--text-3)]"> {object}</span>
          )}
        </div>
        {time && (
          <div className="text-[11px] text-[var(--text-3)] tabular-nums">
            {time}
          </div>
        )}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* Local hook: per-space task list                                     */
/* ------------------------------------------------------------------ */

import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../../lib/rpc'
import type { Task } from '../../lib/types'

/**
 * Lightweight task.list wrapper scoped to a single spaceId.
 * Returns task entries shaped for the WorkInProgress card. Status
 * filter is applied server-side. We don't join activity data here —
 * the card just needs goal/status/role for a compact summary.
 *
 * Lives in this file because the existing useProjectTasks hook targets
 * the project-tasks view and returns a richer payload.
 */
function useSpaceTaskList(spaceId: string | null, statuses: string[]) {
  return useQuery<TaskListEntry[]>({
    queryKey: ['task.list.space-overview', spaceId ?? '', statuses.join(',')],
    queryFn: async () => {
      const res = await rpcCall<{ tasks: Task[] }>('task.list', {
        spaceId,
        status: statuses,
        limit: 25,
      })
      return (res.tasks ?? []).map(task => ({
        task: {
          id: task.id,
          description: task.title || task.description,
          status: task.status,
          assignedTo: task.assignedTo,
        },
      }))
    },
    enabled: !!spaceId,
    refetchInterval: 5000,
    retry: false,
  })
}
