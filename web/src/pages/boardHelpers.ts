import type { AcceptanceCriterion, Task, TaskAttempt, AttemptReview } from '../lib/types'

export interface TaskBlockerInfo {
  kind: string
  id: string
  reason?: string
  createdAt?: string
}

/** Returns true for system/coordination tasks that should be hidden from the board. */
export function isSystemTask(t: Task): boolean {
  const source = String(t.metadata?.source ?? '')
  const systemSources = [
    'spawn_worker',
    'coordinator.continuation', 'coordinator.stalled',
  ]
  if (systemSources.includes(source)) return true
  if (t.taskKind === 'coordinator') return true
  return false
}

/** Returns the effective status for board column placement. */
export function effectiveStatus(t: Task): string {
  return t.status
}

/* ── Attempt ledger helpers ─────────────────────────── */

/** Parse the structured attempt ledger from task metadata. */
export function getAttempts(task: Task): TaskAttempt[] {
  const raw = task.metadata?.attempts
  if (!Array.isArray(raw)) return []
  return raw as TaskAttempt[]
}

/** Number of completed attempts (for badge display). */
export function getAttemptCount(task: Task): number {
  return getAttempts(task).length
}

/** Latest review from the attempt ledger. */
export function getLatestReview(task: Task): AttemptReview | null {
  const attempts = getAttempts(task)
  for (let i = attempts.length - 1; i >= 0; i--) {
    if (attempts[i].review) return attempts[i].review!
  }
  const meta = task.metadata
  if (!meta) return null
  let decision = typeof meta.reviewDecision === 'string' ? meta.reviewDecision.trim() : ''
  if (!decision) return null
  if (decision === 'approve') decision = 'approved'
  const feedback = typeof meta.reviewFeedback === 'string' ? meta.reviewFeedback.trim() : ''
  const reviewedBy = typeof meta.reviewedBy === 'string' ? meta.reviewedBy.trim() : ''
  const reviewedAt = typeof meta.reviewedAt === 'string' ? meta.reviewedAt.trim() : ''
  const reviewerRole =
    typeof meta.reviewerRole === 'string' ? meta.reviewerRole.trim() : ''
  return {
    decision,
    feedback: feedback || undefined,
    reviewedBy: reviewedBy || undefined,
    reviewedAt: reviewedAt || undefined,
    reviewerRole: reviewerRole || undefined,
  }
}

/** Get acceptance criteria from task state or legacy metadata. */
export function getAcceptanceCriteria(task: Task): AcceptanceCriterion[] {
  if (Array.isArray(task.acceptanceCriteria)) {
    return task.acceptanceCriteria
      .map((criterion, index) => ({
        id: String(criterion.id || `criterion-${index + 1}`),
        text: String(criterion.text || '').trim(),
        satisfied: Boolean(criterion.satisfied),
      }))
      .filter((criterion) => criterion.text)
  }
  const raw = task.metadata?.acceptanceCriteria
  if (!Array.isArray(raw)) return []
  return raw
    .map((value, index) => parseLegacyAcceptanceCriterion(value, index))
    .filter((criterion): criterion is AcceptanceCriterion => criterion !== null)
}

function parseLegacyAcceptanceCriterion(value: unknown, index: number): AcceptanceCriterion | null {
  if (typeof value !== 'string') return null
  const raw = value.trim()
  if (!raw) return null
  const checked = raw.match(/^[-*]\s*\[[xX]\]\s*(.*)/)
  const unchecked = raw.match(/^[-*]\s*\[\s*\]\s*(.*)/)
  return {
    id: `criterion-${index + 1}`,
    text: (checked?.[1] ?? unchecked?.[1] ?? raw).trim(),
    satisfied: Boolean(checked),
  }
}

export function getTaskBlockers(task: Task): TaskBlockerInfo[] {
  const raw = task.metadata?.blockedBy
  if (!Array.isArray(raw)) return []

  const blockers: TaskBlockerInfo[] = []
  for (const entry of raw) {
    if (!entry || typeof entry !== 'object') continue
    const blocker = entry as Record<string, unknown>
    const kind = typeof blocker.kind === 'string' ? blocker.kind.trim() : ''
    const id = typeof blocker.id === 'string' ? blocker.id.trim() : ''
    if (!kind || !id) continue
    blockers.push({
      kind,
      id,
      reason: typeof blocker.reason === 'string' ? blocker.reason : undefined,
      createdAt: typeof blocker.createdAt === 'string' ? blocker.createdAt : undefined,
    })
  }
  return blockers
}

/**
 * Returns a human-readable duration string for a completed task (completedAt - createdAt).
 * Returns null for tasks without completedAt, or when timestamps are invalid.
 */
export function taskDuration(task: Task): string | null {
  if (!task.completedAt || !task.createdAt) return null
  const ms = new Date(task.completedAt).getTime() - new Date(task.createdAt).getTime()
  if (ms <= 0) return null
  const mins = Math.floor(ms / 60_000)
  if (mins < 60) return `${mins}m`
  const hrs = Math.floor(mins / 60)
  const remainMins = mins % 60
  if (hrs < 24) return remainMins > 0 ? `${hrs}h ${remainMins}m` : `${hrs}h`
  return `${Math.floor(hrs / 24)}d`
}

/* ── Display helpers shared with TaskCard ───────────── */

export function relativeTime(iso?: string): string {
  if (!iso) return 'unknown'
  const diff = Date.now() - new Date(iso).getTime()
  const mins = Math.floor(diff / 60_000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.floor(hrs / 24)}d ago`
}

export function taskIdShort(id: string): string {
  return id.length > 8 ? id.slice(-6) : id
}

export function urgencyBadgeVariant(
  urgency: string | null | undefined,
): 'secondary' | 'info' | 'warning' | 'danger' {
  switch (urgency) {
    case 'critical': return 'danger'
    case 'high':     return 'warning'
    case 'medium':   return 'info'
    default:         return 'secondary'
  }
}

export function urgencyAccentColor(urgency: string | null | undefined): string | null {
  switch (urgency) {
    case 'critical': return 'var(--red)'
    case 'high':     return 'var(--amber)'
    case 'medium':   return 'var(--blue)'
    case 'low':      return 'var(--text-3)'
    default:         return null
  }
}

/** Check if a task is a retry and extract original goal + feedback. */
export function parseRetryTask(
  task: Task,
):
  | { isRetry: true; originalGoal: string; feedback: string; attemptNum?: number }
  | { isRetry: false } {
  const goal = task.description ?? ''

  // Format 2 (new): "TASK RETRY (Attempt N of M)\n\n== Previous Attempts ==\n...\n\n== Original Goal ==\n{goal}"
  const newMatch = goal.match(/^TASK RETRY \(Attempt (\d+)/)
  if (newMatch) {
    const attemptNum = parseInt(newMatch[1], 10)
    const origIdx = goal.indexOf('== Original Goal ==')
    const originalGoal =
      origIdx >= 0 ? goal.slice(origIdx + '== Original Goal =='.length).trim() : goal
    const reviewLines = [...goal.matchAll(/Review:\s*\w+\s*-\s*(.*)/g)]
    const feedback =
      reviewLines.length > 0 ? reviewLines[reviewLines.length - 1][1].trim() : ''
    return { isRetry: true, originalGoal, feedback, attemptNum }
  }

  // Format 1 (legacy): metadata.source === 'retry'
  if (String(task.metadata?.source ?? '') === 'retry') {
    const match = goal.match(/^RETRY with feedback:\n([\s\S]*?)\n\nOriginal goal:\s*([\s\S]*)$/)
    if (match) return { isRetry: true, feedback: match[1].trim(), originalGoal: match[2].trim() }
    const match2 = goal.match(/^Retry:\s*([\s\S]*?)\n\nOriginal goal:\s*([\s\S]*)$/)
    if (match2) return { isRetry: true, feedback: match2[1].trim(), originalGoal: match2[2].trim() }
    return { isRetry: true, originalGoal: goal, feedback: '' }
  }

  return { isRetry: false }
}
