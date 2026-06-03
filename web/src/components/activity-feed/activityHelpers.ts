import type { AgentEvent } from '../../lib/types'

/* ── Kind label helpers ──────────────────────────────── */

export const kindColors: Record<string, { bg: string; fg: string }> = {
  error: { bg: 'var(--red-dim)', fg: 'var(--red)' },
  fail: { bg: 'var(--red-dim)', fg: 'var(--red)' },
  done: { bg: 'var(--green-dim)', fg: 'var(--green)' },
  complete: { bg: 'var(--green-dim)', fg: 'var(--green)' },
  start: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  tool: { bg: 'var(--amber-dim)', fg: 'var(--amber)' },
  read: { bg: 'var(--bg-elevated)', fg: 'var(--text-3)' },
  write: { bg: 'var(--amber-dim)', fg: 'var(--amber)' },
  exec: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  spawn: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  message: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  model: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  task: { bg: 'var(--green-dim)', fg: 'var(--green)' },
  space: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  review: { bg: 'var(--amber-dim)', fg: 'var(--amber)' },
  heartbeat: { bg: 'var(--red-dim)', fg: 'var(--red)' },
  progress: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  edit: { bg: 'var(--amber-dim)', fg: 'var(--amber)' },
  soul: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  browser: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  shell: { bg: 'var(--bg-elevated)', fg: 'var(--text-3)' },
  bash: { bg: 'var(--bg-elevated)', fg: 'var(--text-3)' },
  http: { bg: 'var(--bg-elevated)', fg: 'var(--text-3)' },
  mission: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  plan: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  graph: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  operator: { bg: 'var(--amber-dim)', fg: 'var(--amber)' },
  'oa.': { bg: 'var(--amber-dim)', fg: 'var(--amber)' },
}

/** Map raw internal kind strings to short, human-friendly past-tense action
 * phrases ("what happened"). Returns null to hide the pill. */
export function humanizeKind(kind: string): string | null {
  const lower = kind.toLowerCase()
  const map: Record<string, string> = {
    'read_file': 'Read',
    'write_file': 'Wrote',
    'list_files': 'Listed',
    'delete_file': 'Deleted',
    'metrics': 'Metrics',
    'web_search': 'Web Search',
    'tool_search': 'Tool Search',
    'search_files': 'Searched',
    'edit_file': 'Edited',
    'file_change': 'Changed',
    'code_compile': 'Compiled',
    'tool_execution': 'Tool Called',
    'agent.tool.call': 'Tool Called',
    'tool': 'Tool',
    'user_message': 'User Message',
    'agent_message': 'Agent Reply',
    'agent_speak': 'Agent Reply',
    'model_response': 'Model Response',
    'task.done': 'Task Done',
    'task.start': 'Task Started',
    'task.create': 'Task Created',
    'task.claim': 'Task Claimed',
    'task': 'Task',
    'task.queued': 'Task Queued',
    'subagent.spawn': 'Subagent Spawned',
    'subagent.done': 'Subagent Done',
    'error': 'Error',
    'llm.error': 'LLM Error',
    'runtime.error': 'Runtime Error',
    'llm.retry': 'LLM Retry',
    'space_message': 'Space Message',
    'list_spaces': 'Listed Spaces',
    'space': 'Space',
    'shell_exec': 'Bash',
    'bash': 'Bash',
    'http': 'HTTP Request',
    'soul_update': 'Soul Updated',
    'note': 'Noted',
    'browser': 'Browsed',
    'heartbeat': 'Heartbeat',
    'operator': 'Operator',
    'operator_escalate': 'Escalated',
    'operator_request': 'Action Requested',
    'operator_action': 'Operator Action',
    'operator_action.created': 'Action Created',
    'operator_action.resolved': 'Action Resolved',
    'oa.created': 'Action Created',
    'oa.acknowledged': 'Action Acknowledged',
    'oa.started': 'Action Started',
    'oa.completed': 'Action Completed',
    'oa.verified': 'Action Verified',
    'oa.blocked': 'Action Blocked',
    'oa.unblocked': 'Action Unblocked',
    'oa.canceled': 'Action Canceled',
    'oa.progress_noted': 'Progress Noted',
    'oa.comment': 'Comment Added',
    'decision': 'Decision',
    'decision_log': 'Decision Logged',
    'mission': 'Mission',
    'plan': 'Plan',
    'graph_query': 'Graph Query',
  }
  if (map[lower]) return map[lower]
  if (lower.includes('_') || lower.includes('.')) return null
  if (kind.length <= 8) return kind
  return null
}

export function getKindStyle(kind: string): { bg: string; fg: string } {
  const lower = kind.toLowerCase()
  for (const [key, style] of Object.entries(kindColors)) {
    if (lower.includes(key)) return style
  }
  return { bg: 'var(--bg-elevated)', fg: 'var(--text-3)' }
}

export function getStatusClass(event: AgentEvent): string {
  if (event.status === 'error' || event.error) return 'error'
  if (event.status === 'pending' || !event.completedAt) return 'pending'
  return 'ok'
}

/* ── Formatting helpers ──────────────────────────────── */

export function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 0) return 'now'
  const s = Math.floor(diff / 1000)
  if (s < 5) return 'now'
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h`
  return `${Math.floor(h / 24)}d`
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${(ms / 1000).toFixed(1)}s`
  const m = Math.floor(s / 60)
  if (m < 60) {
    const rem = s % 60
    return rem > 0 ? `${m}m ${rem}s` : `${m}m`
  }
  const h = Math.floor(m / 60)
  const remM = m % 60
  return remM > 0 ? `${h}h ${remM}m` : `${h}h`
}

/** Extract the basename from a file path */
export function basename(path: string): string {
  const parts = path.split('/')
  return parts[parts.length - 1] || path
}

/* ── Diff helpers ────────────────────────────────────── */

export const DIFF_SKIP = ['CHECKLIST.md', 'HEAD.md', 'SUMMARY.md', 'MEMORY.md']

export function isDiffSkipped(path?: string): boolean {
  return !!path && DIFF_SKIP.some(p => path.includes(p))
}

export function guessLang(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  const map: Record<string, string> = {
    ts: 'typescript', tsx: 'tsx', js: 'javascript', jsx: 'jsx',
    go: 'go', py: 'python', rs: 'rust', json: 'json',
    md: 'markdown', yaml: 'yaml', yml: 'yaml', sh: 'bash',
    css: 'css', html: 'html', sql: 'sql', toml: 'toml',
  }
  return map[ext] ?? 'text'
}

export type DiffLine = { type: 'add' | 'del' | 'hunk' | 'meta' | 'ctx'; text: string }

export function parseDiff(unified: string): { lines: DiffLine[]; added: number; deleted: number } {
  let added = 0; let deleted = 0
  const lines: DiffLine[] = []
  for (const raw of unified.split('\n')) {
    if (raw.startsWith('+++') || raw.startsWith('---') || raw.startsWith('diff ')) {
      lines.push({ type: 'meta', text: raw })
    } else if (raw.startsWith('@@')) {
      lines.push({ type: 'hunk', text: raw })
    } else if (raw.startsWith('+')) {
      added++; lines.push({ type: 'add', text: raw })
    } else if (raw.startsWith('-')) {
      deleted++; lines.push({ type: 'del', text: raw })
    } else {
      lines.push({ type: 'ctx', text: raw })
    }
  }
  return { lines, added, deleted }
}

/* ── UUID detection ──────────────────────────────────── */

/** Returns true if s is a raw UUID that should never be shown to the user. */
export function looksLikeSpaceUUID(s: string): boolean {
  if (!s) return false
  const lower = s.toLowerCase()
  const id = lower.startsWith('space-') ? lower.slice(6) : lower
  if (id.length !== 36) return false
  // Must match 8-4-4-4-12 hex pattern
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(id)
}

/* ── Constants ───────────────────────────────────────── */

export const FILE_WRITE_KINDS = new Set(['write_file', 'edit_file'])
