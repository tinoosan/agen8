import type { AgentEvent } from '../../lib/types'
import { materializeNestedJSONStrings } from './spaceInspectorPrettyPayload'

function clean(value: unknown): string {
  const text = String(value ?? '').trim()
  if (!text) return ''
  const lower = text.toLowerCase()
  if (lower === 'null' || lower === 'undefined' || lower === 'nil' || text === '""' || text === '{}') return ''
  return text
}

function parseJSON(raw: string): unknown {
  const trimmed = raw.trim()
  if (!trimmed || (!trimmed.startsWith('{') && !trimmed.startsWith('['))) return raw
  try {
    return materializeNestedJSONStrings(JSON.parse(trimmed))
  } catch {
    return raw
  }
}

function collectErrorText(value: unknown, depth = 0): string {
  if (depth > 5 || value == null) return ''
  if (typeof value === 'string') {
    const parsed = parseJSON(value)
    if (parsed !== value) return collectErrorText(parsed, depth + 1)
    return clean(value)
  }
  if (typeof value === 'number' || typeof value === 'boolean') return clean(value)
  if (Array.isArray(value)) {
    for (const entry of value) {
      const found = collectErrorText(entry, depth + 1)
      if (found) return found
    }
    return ''
  }
  if (typeof value !== 'object') return ''

  const record = value as Record<string, unknown>
  for (const key of ['error', 'rawError', 'message', 'detail', 'reason', 'code']) {
    const direct = collectErrorText(record[key], depth + 1)
    if (direct) return direct
  }
  for (const key of ['body', 'text', 'result', 'output', 'response', 'responseText', 'repr']) {
    const nested = collectErrorText(record[key], depth + 1)
    if (nested) return nested
  }
  return ''
}

export function resolveInspectorErrorText(event: AgentEvent): string {
  const data = event.data ?? {}
  for (const candidate of [
    event.error,
    data.error,
    data.rawError,
    data.message,
    data.result,
    data.output,
    data.outputPreview,
    event.outputPreview,
    event.textPreview,
    data.response,
    event.title,
  ]) {
    const found = collectErrorText(candidate)
    if (found) return found
  }
  return 'Error details were not captured for this event.'
}

export function hasDisplayText(value: unknown): boolean {
  return clean(value) !== ''
}
