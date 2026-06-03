import type { AgentEvent } from '../../lib/types'
import { humanizeKind } from '../activity-feed/activityHelpers'
import { normalizeTaskAction, readableTaskAction, resolveTaskOutput } from './taskPayload'
import { hasDisplayText } from './spaceInspectorError'

const COMPOSITE_TOOL_KINDS = new Set(['task', 'plan', 'mission', 'decision', 'operator', 'metrics', 'schedule'])

function displayToken(value: unknown): string {
  return String(value ?? '')
    .trim()
    .replace(/[-_]+/g, ' ')
    .replace(/\b\w/g, (char) => char.toUpperCase())
}

function stripMarkdown(raw: string): string {
  return raw
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/`([^`]*)`/g, '$1')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/__([^_]+)__/g, '$1')
    .replace(/\*([^*]+)\*/g, '$1')
    .replace(/(^|\s)_([^_]+)_/g, '$1$2')
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/^\s*[-*+]\s+/gm, '')
    .replace(/^\s*\d+\.\s+/gm, '')
    .replace(/\s+/g, ' ')
    .trim()
}

function parseStructuredPayload(value: unknown, depth = 0): unknown {
  if (depth > 5 || value == null) return null
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed || (!trimmed.startsWith('{') && !trimmed.startsWith('['))) return null
    try {
      return parseStructuredPayload(JSON.parse(trimmed), depth + 1)
    } catch {
      return null
    }
  }
  if (Array.isArray(value)) return value
  if (typeof value !== 'object') return null

  const record = value as Record<string, unknown>
  for (const key of ['text', 'result', 'responseText', 'output', 'repr', 'outputPreview']) {
    const nested = parseStructuredPayload(record[key], depth + 1)
    if (nested) return nested
  }
  return record
}

function recordFromPayload(value: unknown): Record<string, unknown> | null {
  const parsed = parseStructuredPayload(value)
  return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
    ? parsed as Record<string, unknown>
    : null
}

function textField(record: Record<string, unknown> | null | undefined, ...keys: string[]): string {
  if (!record) return ''
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
    if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  }
  return ''
}

function isRawStructuredText(value: unknown): boolean {
  return typeof value === 'string' && /^[\s\n\r]*[{[]/.test(value)
}

function displayText(value: unknown): string {
  if (!hasDisplayText(value)) return ''
  return String(value).trim()
}

function titleFromCompositePayload(kind: string, event: AgentEvent): string {
  const data = event.data ?? {}
  const candidates: unknown[] = [
    data.result,
    data.responseText,
    data.output,
    data.outputPreview,
    data.outputFull,
    event.outputPreview,
    event.textPreview,
    data.repr,
    event.title,
    data.title,
  ]

  for (const candidate of candidates) {
    const payload = recordFromPayload(candidate)
    if (!payload) continue

    if (kind === 'plan') {
      const plan = payload.plan && typeof payload.plan === 'object' ? payload.plan as Record<string, unknown> : null
      const phase = payload.phase && typeof payload.phase === 'object' ? payload.phase as Record<string, unknown> : null
      const todo = payload.todo && typeof payload.todo === 'object' ? payload.todo as Record<string, unknown> : null
      const amendment = payload.amendment && typeof payload.amendment === 'object' ? payload.amendment as Record<string, unknown> : null
      const comment = payload.comment && typeof payload.comment === 'object' ? payload.comment as Record<string, unknown> : null
      const title = textField(plan, 'title', 'Title')
        || textField(phase, 'title', 'Title')
        || textField(todo, 'text', 'Text')
        || textField(amendment, 'rationale', 'Rationale')
        || textField(comment, 'text', 'Text')
        || textField(payload, 'guidance', 'title', 'Title')
      if (title) return title
    }

    if (kind === 'mission') {
      const mission = payload.mission && typeof payload.mission === 'object' ? payload.mission as Record<string, unknown> : null
      const kr = payload.keyResult && typeof payload.keyResult === 'object' ? payload.keyResult as Record<string, unknown> : null
      const title = textField(mission, 'title', 'Title')
        || textField(kr, 'title', 'Title')
        || textField(payload, 'title', 'Title')
      if (title) return title
    }

    if (kind === 'decision') {
      const decision = payload.decision && typeof payload.decision === 'object' ? payload.decision as Record<string, unknown> : null
      const title = textField(decision, 'title', 'Title')
        || textField(decision, 'rationale', 'Rationale')
        || textField(payload, 'title', 'Title', 'rationale', 'Rationale')
      if (title) return title
    }

    if (kind === 'operator') {
      const item = payload.operator && typeof payload.operator === 'object' ? payload.operator as Record<string, unknown> : payload
      const title = textField(item, 'title', 'Title', 'description', 'Description', 'recommendation', 'Recommendation')
      if (title) return title
    }

    if (kind === 'metrics') {
      const action = textField(payload, 'action', 'Action') || textField(event.data, 'action', 'op')
      const role = textField(payload, 'Role', 'role')
      const space = textField(payload, 'SpaceName', 'spaceName', 'Space', 'space')
      const scope = role || space
      const label = action === 'agent' ? 'Agent metrics' : action === 'space' ? 'Space metrics' : 'Project metrics'
      return scope ? `${label} · ${scope}` : label
    }

    if (kind === 'schedule') {
      const entry = Array.isArray(payload.entries) && payload.entries[0] && typeof payload.entries[0] === 'object'
        ? payload.entries[0] as Record<string, unknown>
        : null
      const action = textField(payload, 'action', 'Action') || textField(event.data, 'action', 'op')
      const title = textField(entry, 'Name', 'name', 'Goal', 'goal')
        || textField(payload, 'name', 'Name', 'goal', 'Goal', 'message', 'Message')
      if (title) return action ? `Schedule ${displayToken(action)} · ${title}` : title
      if (action) return `Schedule ${displayToken(action)}`
    }

    const named = payload[kind] && typeof payload[kind] === 'object' ? payload[kind] as Record<string, unknown> : null
    const generic = textField(named, 'title', 'Title', 'name', 'Name', 'text', 'Text')
      || textField(payload, 'title', 'Title', 'name', 'Name')
    if (generic) return generic
  }

  return ''
}

export function resolveEventTitle(event: AgentEvent): string {
  const kind = event.kind ?? ''
  const kindLower = kind.toLowerCase()
  if (kindLower === 'task') {
    const output = resolveTaskOutput(event)
    const dataTitle = String(event.data?.title ?? '').trim()
    const taskTitle = String(output?.task?.title ?? event.data?.taskTitle ?? dataTitle).trim()
    if (taskTitle) return taskTitle
    const action = normalizeTaskAction(String(event.data?.action ?? event.data?.op ?? ''))
    if (action) return readableTaskAction(action)
    return 'Task'
  }
  if (COMPOSITE_TOOL_KINDS.has(kindLower)) {
    const structuredTitle = titleFromCompositePayload(kindLower, event)
    if (structuredTitle) return stripMarkdown(structuredTitle)
    const dataTitle = event.data?.title
    const titleText = displayText(dataTitle)
    if (titleText && !isRawStructuredText(titleText)) return titleText
    const action = event.data?.action ?? event.data?.op
    if (action && String(action).trim()) return `${humanizeKind(kind) || displayToken(kind)} ${displayToken(action)}`
    return humanizeKind(kind) || displayToken(kind)
  }
  const dataTitle = displayText(event.data?.title)
  if (dataTitle) return dataTitle
  const isToolSearchTitle = kindLower === 'tool_search' ||
    (kindLower === 'tool' && (event.data?.action ?? '').toLowerCase() === 'search')
  if (isToolSearchTitle && event.data?.query) {
    return `"${event.data.query}"`
  }
  if (kindLower === 'tool' && (event.data?.action ?? '').toLowerCase() !== 'search') {
    return 'List callable tools'
  }
  const eventTitle = displayText(event.title)
  if (eventTitle && eventTitle !== kind) return eventTitle
  const outputPreview = displayText(event.outputPreview)
  if (outputPreview) {
    return stripMarkdown(outputPreview)
  }
  const textPreview = displayText(event.textPreview)
  if (textPreview) {
    return stripMarkdown(textPreview)
  }
  const humanized = humanizeKind(kind)
  if (humanized) return humanized
  return kind
}
