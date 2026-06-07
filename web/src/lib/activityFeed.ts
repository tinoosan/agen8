import type { Task, DecisionView, MissionView } from './types'
import { taskClaimedMemberLabel, taskAssignedMemberLabel, taskCreatedMemberLabel } from './taskMembers'
import { decisionActorDisplay } from './decisionDisplay'
import { sanitizeDecisionTitle, sanitizeDisplayTitle } from './displaySanitizers'
import { taskDetailLink, decisionDetailLink, missionDetailLink } from './routing'

/**
 * The activity feed is a CLIENT-SIDE PROJECTION, not a persisted event log.
 *
 * Domain events in this system are ephemeral (the bus is non-persistent and no
 * activity/event table exists), so there is no backend history to read. Instead
 * we synthesize a chronological feed from data already fetched via the existing
 * read APIs: each task carries created/started/completed timestamps, each
 * decision a createdAt, each mission a createdAt. This goes live for free —
 * the existing SSE invalidation refetches those queries, and recomputing the
 * projection picks up the change.
 *
 * Honest-MVP rule: we only emit milestones that have a REAL stored timestamp.
 * Transitions without one (in_review "sent to review", blocked/unblocked,
 * retry) are deliberately omitted rather than inventing a time — the system's
 * standing rule is no invented data.
 */

export type ActivityKind = 'task' | 'decision' | 'mission'

export type ActivityType =
  | 'task.created'
  | 'task.started'
  | 'task.completed'
  | 'task.failed'
  | 'task.canceled'
  | 'decision.logged'
  | 'mission.created'

export interface ActivityEvent {
  /** Stable across refetches: `${type}:${entityId}`. A task yields up to three
   *  events with distinct type prefixes, so ids never collide. Used as the React
   *  key, so a row only re-mounts (and replays its entrance) on genuine arrival. */
  id: string
  kind: ActivityKind
  type: ActivityType
  /** ISO timestamp of the milestone. */
  at: string
  /** Parsed epoch ms, for sorting/bucketing without re-parsing. */
  atMs: number
  /** Display label of who acted, when known (missions have no actor on the view). */
  actor?: string
  /** Entity title/summary the event is about. */
  subject: string
  /** Route to the entity's detail surface. */
  link: string
}

/** Coalesce the first non-empty trimmed string, else undefined. */
function firstNonEmpty(...vals: Array<string | null | undefined>): string | undefined {
  for (const v of vals) {
    const t = v?.trim()
    if (t) return t
  }
  return undefined
}

function parseMs(iso: string | undefined): number | null {
  if (!iso) return null
  const ms = Date.parse(iso)
  return Number.isNaN(ms) ? null : ms
}

/** Map a terminal task status to its activity type. Non-terminal → null. */
function terminalType(status: string): ActivityType | null {
  switch (status) {
    case 'succeeded':
      return 'task.completed'
    case 'failed':
      return 'task.failed'
    case 'canceled':
      return 'task.canceled'
    default:
      return null
  }
}

function taskEvents(task: Task, projectId: string): ActivityEvent[] {
  const out: ActivityEvent[] = []
  const subject = firstNonEmpty(task.title, task.description) ?? 'Untitled task'
  const link = taskDetailLink(projectId, task.id)

  const createdMs = parseMs(task.createdAt)
  if (createdMs !== null) {
    out.push({
      id: `task.created:${task.id}`,
      kind: 'task',
      type: 'task.created',
      at: task.createdAt as string,
      atMs: createdMs,
      actor: taskCreatedMemberLabel(task),
      subject,
      link,
    })
  }

  const startedMs = parseMs(task.startedAt)
  if (startedMs !== null) {
    out.push({
      id: `task.started:${task.id}`,
      kind: 'task',
      type: 'task.started',
      at: task.startedAt as string,
      atMs: startedMs,
      actor: firstNonEmpty(taskClaimedMemberLabel(task), taskAssignedMemberLabel(task)),
      subject,
      link,
    })
  }

  const tType = terminalType(task.status ?? '')
  const completedMs = parseMs(task.completedAt)
  if (tType && completedMs !== null) {
    out.push({
      id: `${tType}:${task.id}`,
      kind: 'task',
      type: tType,
      at: task.completedAt as string,
      atMs: completedMs,
      actor: firstNonEmpty(taskClaimedMemberLabel(task), taskAssignedMemberLabel(task)),
      subject,
      link,
    })
  }

  return out
}

function decisionEvent(decision: DecisionView, projectId: string): ActivityEvent | null {
  const atMs = parseMs(decision.createdAt)
  if (atMs === null) return null
  return {
    id: `decision.logged:${decision.id}`,
    kind: 'decision',
    type: 'decision.logged',
    at: decision.createdAt,
    atMs,
    actor: decisionActorDisplay(decision).label,
    subject: sanitizeDecisionTitle(decision.title),
    link: decisionDetailLink(projectId, decision.id),
  }
}

function missionEvent(mission: MissionView, projectId: string): ActivityEvent | null {
  const atMs = parseMs(mission.createdAt)
  if (atMs === null) return null
  return {
    id: `mission.created:${mission.id}`,
    kind: 'mission',
    type: 'mission.created',
    at: mission.createdAt,
    atMs,
    // MissionView carries no creator, so leave actor unset — the UI phrases
    // these as "Mission created — <title>" rather than inventing an actor.
    subject: sanitizeDisplayTitle(mission.title) ?? 'Untitled mission',
    link: missionDetailLink(projectId, mission.id),
  }
}

export interface BuildActivityInput {
  projectId: string
  tasks?: Task[]
  decisions?: DecisionView[]
  missions?: MissionView[]
}

/**
 * Project the current entity snapshots into a single chronological stream,
 * newest first. Pure and deterministic given its inputs.
 */
export function buildActivityEvents({
  projectId,
  tasks,
  decisions,
  missions,
}: BuildActivityInput): ActivityEvent[] {
  const events: ActivityEvent[] = []

  for (const task of tasks ?? []) events.push(...taskEvents(task, projectId))
  for (const decision of decisions ?? []) {
    const e = decisionEvent(decision, projectId)
    if (e) events.push(e)
  }
  for (const mission of missions ?? []) {
    const e = missionEvent(mission, projectId)
    if (e) events.push(e)
  }

  // Newest first; tie-break on id so equal timestamps render in a stable order.
  events.sort((a, b) => b.atMs - a.atMs || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
  return events
}

/* ── Temporal grouping (mirrors DecisionFeed's buckets so the two feeds read
 *    the same way) ─────────────────────────────────────────────────────── */

const MINUTE = 60_000
const HOUR = 3_600_000
const DAY = 86_400_000

export type ActivityBucket = 'Just now' | 'Last hour' | 'Today' | 'Yesterday' | 'Older'

export const ACTIVITY_BUCKET_ORDER: ActivityBucket[] = [
  'Just now',
  'Last hour',
  'Today',
  'Yesterday',
  'Older',
]

export function getActivityBucket(atMs: number, now: number = Date.now()): ActivityBucket {
  const diff = now - atMs
  if (diff < 5 * MINUTE) return 'Just now'
  if (diff < HOUR) return 'Last hour'
  if (diff < DAY) return 'Today'
  if (diff < 2 * DAY) return 'Yesterday'
  return 'Older'
}

export interface ActivityGroup {
  bucket: ActivityBucket
  items: ActivityEvent[]
}

/** Group an already-sorted event list into the ordered time buckets. */
export function groupActivityByBucket(
  events: ActivityEvent[],
  now: number = Date.now(),
): ActivityGroup[] {
  const groups = new Map<ActivityBucket, ActivityEvent[]>()
  for (const e of events) {
    const bucket = getActivityBucket(e.atMs, now)
    const list = groups.get(bucket)
    if (list) list.push(e)
    else groups.set(bucket, [e])
  }
  return ACTIVITY_BUCKET_ORDER.filter((b) => groups.has(b)).map((bucket) => ({
    bucket,
    items: groups.get(bucket)!,
  }))
}
