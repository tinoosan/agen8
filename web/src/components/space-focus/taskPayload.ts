import type { AgentEvent } from '../../lib/types'

interface TaskView {
  id?: string
  status?: string
  title?: string
  description?: string
  assignedToMemberId?: string
  keyResultRef?: string
  summary?: string
}

interface TaskOutput {
  task?: TaskView
  tasks?: TaskView[]
  count?: number
}

const ACTION_LABELS: Record<string, string> = {
  create: 'Create Task',
  get: 'Get Task',
  list: 'List Tasks',
  claim: 'Claim Task',
  release: 'Release Task',
  submit: 'Submit Task',
  block: 'Block Task',
  unblock: 'Unblock Task',
  reassign: 'Reassign Task',
  cancel: 'Cancel Task',
  review: 'Review Task',
}

function clean(value: unknown): string {
  return String(value ?? '').trim()
}

export function normalizeTaskAction(rawAction: string): string {
  const cleaned = rawAction.trim().toLowerCase().replace(/[-\s]+/g, '_')
  if (!cleaned) return ''
  switch (cleaned) {
    case 'create_task': return 'create'
    case 'get_task': return 'get'
    case 'list_task':
    case 'list_tasks': return 'list'
    case 'claim_task': return 'claim'
    case 'release_task': return 'release'
    case 'submit_task':
    case 'complete_task':
    case 'complete': return 'submit'
    case 'block_task': return 'block'
    case 'unblock_task': return 'unblock'
    case 'reassign_task': return 'reassign'
    case 'cancel_task':
    case 'fail_task':
    case 'fail': return 'cancel'
    case 'review_task': return 'review'
    default: return cleaned
  }
}

export function readableTaskAction(action: string): string {
  const key = normalizeTaskAction(action)
  if (!key) return 'Task'
  return ACTION_LABELS[key] ?? key.replace(/_/g, ' ').replace(/\b\w/g, (char) => char.toUpperCase())
}

function hasTaskShape(value: unknown): value is TaskOutput {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const output = value as TaskOutput
  return Boolean(output.task || Array.isArray(output.tasks))
}

function parseTaskOutput(value: unknown, depth = 0): TaskOutput | null {
  if (depth > 4 || value == null) return null

  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed || (!trimmed.startsWith('{') && !trimmed.startsWith('['))) return null
    try {
      return parseTaskOutput(JSON.parse(trimmed), depth + 1)
    } catch {
      return null
    }
  }

  if (Array.isArray(value)) return { tasks: value as TaskView[] }
  if (hasTaskShape(value)) return value
  if (typeof value !== 'object') return null

  const record = value as Record<string, unknown>
  if (clean(record.id) && (clean(record.status) || clean(record.title) || clean(record.description))) {
    return { task: record as TaskView }
  }

  for (const key of ['result', 'responseText', 'output', 'repr', 'text']) {
    const decoded = parseTaskOutput(record[key], depth + 1)
    if (decoded) return decoded
  }

  return null
}

export function resolveTaskOutput(event: AgentEvent): TaskOutput | null {
  const data = event.data ?? {}
  return parseTaskOutput(data.result)
    ?? parseTaskOutput(data.responseText)
    ?? parseTaskOutput(data.output)
    ?? parseTaskOutput(data.outputPreview)
}
