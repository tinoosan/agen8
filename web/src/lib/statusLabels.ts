/**
 * Shared status label mapping for consistent terminology across all views.
 *
 * Internal statuses (from backend) map to a single set of human-readable labels
 * so that Board, Dashboard, and all other views speak the same language.
 */

/* ── Task-level status labels ──────────────────── */

const TASK_STATUS_MAP: Record<string, string> = {
  pending: 'Queued',
  active: 'Working',
  blocked: 'Blocked',
  paused: 'Paused',
  in_review: 'In Review',
  succeeded: 'Done',
  failed: 'Failed',
  canceled: 'Canceled',
}

export function taskStatusLabel(status: string): string {
  return TASK_STATUS_MAP[status] ?? status
}

/* ── Task-level status colors ──────────────────── */

const TASK_STATUS_COLORS: Record<string, string> = {
  Queued: 'var(--text-3)',
  Working: 'var(--blue)',
  Blocked: 'var(--amber)',
  Paused: 'var(--text-2)',
  'In Review': 'var(--accent)',
  Done: 'var(--green)',
  Failed: 'var(--red)',
  Canceled: 'var(--text-3)',
}

export function taskStatusColor(status: string): string {
  const label = TASK_STATUS_MAP[status] ?? status
  return TASK_STATUS_COLORS[label] ?? 'var(--text-3)'
}

/* ── Member-level status labels ──────────── */

const MEMBER_STATUS_MAP: Record<string, string> = {
  running: 'Running',
  active: 'Running',
  idle: 'Idle',
  waiting: 'Idle',
  pending: 'Starting',
  starting: 'Starting',
  failed: 'Failed',
  error: 'Failed',
  done: 'Done',
  stopped: 'Done',
  completed: 'Done',
  working: 'Working',
  thinking: 'Thinking',
  streaming: 'Thinking',
  blocked: 'Blocked',
}

export function memberStatusLabel(status: string): string {
  return MEMBER_STATUS_MAP[status.toLowerCase()] ?? status
}

/* ── Member-level status colors ──────────────────── */

const MEMBER_STATUS_COLORS: Record<string, string> = {
  Running: 'var(--green)',
  Idle: 'var(--text-3)',
  Starting: 'var(--amber)',
  Failed: 'var(--red)',
  Done: 'var(--blue)',
  Working: 'var(--green)',
  Thinking: 'var(--accent)',
  Blocked: 'var(--red)',
}

export function memberStatusColor(status: string): string {
  const label = memberStatusLabel(status)
  return MEMBER_STATUS_COLORS[label] ?? 'var(--text-3)'
}
