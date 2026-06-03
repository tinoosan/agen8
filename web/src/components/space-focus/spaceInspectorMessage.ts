import type { AgentEvent } from '../../lib/types'

export interface SpaceInspectorMetadata {
  label: string
  value: string
  kind?: 'text' | 'code'
}

export interface SpaceMessageRender {
  label: string
  body: string
  metadata?: SpaceInspectorMetadata[]
}

function displayToken(value: unknown): string {
  return String(value ?? '')
    .trim()
    .replace(/[-_]+/g, ' ')
    .replace(/\b\w/g, (char) => char.toUpperCase())
}

function parseJSONRecord(raw: unknown): Record<string, unknown> | null {
  if (typeof raw !== 'string' || raw.trim() === '') return null
  try {
    const parsed = JSON.parse(raw)
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? parsed as Record<string, unknown>
      : null
  } catch {
    return null
  }
}

function stringField(record: Record<string, unknown> | null | undefined, ...keys: string[]): string {
  if (!record) return ''
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
    if (typeof value === 'number' && Number.isFinite(value)) return String(value)
    if (typeof value === 'boolean') return value ? 'true' : 'false'
  }
  return ''
}

function parseWrappedToolPayload(raw: unknown): Record<string, unknown> | null {
  const outer = parseJSONRecord(raw)
  if (!outer) return null
  const text = stringField(outer, 'text')
  const inner = parseJSONRecord(text)
  return inner ?? outer
}

function composeMessageMarkdown(message: string, subject: string): string {
  const parts = [message.trim()]
  if (subject.trim()) {
    parts.push('\n**Subject**\n')
    parts.push(subject.trim())
  }
  return parts.join('\n')
}

export function resolveSpaceMessageRender(event: AgentEvent): SpaceMessageRender | null {
  const data = event.data ?? {}
  const candidates = [data.result, data.output, event.outputPreview, event.textPreview, data.response]
  for (const candidate of candidates) {
    const payload = parseWrappedToolPayload(candidate)
    if (!payload) continue
    const bodyRecord = payload.body && typeof payload.body === 'object' && !Array.isArray(payload.body)
      ? payload.body as Record<string, unknown>
      : null
    const message = stringField(bodyRecord, 'body', 'message', 'text')
      || stringField(payload, 'message', 'text')
    if (!message) continue
    const subject = stringField(bodyRecord, 'subject')
    const metadata: SpaceInspectorMetadata[] = []
    const destination = stringField(bodyRecord, 'destinationSpaceRef', 'destinationSpaceId')
    const channel = stringField(bodyRecord, 'destinationChannel')
    const kind = stringField(bodyRecord, 'kind')
    const broadcast = stringField(bodyRecord, 'broadcast')
    const correlationID = stringField(bodyRecord, 'correlationId')
    if (destination) metadata.push({ label: 'Destination', value: destination, kind: 'code' })
    if (channel) metadata.push({ label: 'Channel', value: channel, kind: 'code' })
    if (kind) metadata.push({ label: 'Kind', value: displayToken(kind) })
    if (broadcast) metadata.push({ label: 'Broadcast', value: broadcast === 'true' ? 'Yes' : 'No' })
    if (correlationID) metadata.push({ label: 'Correlation', value: correlationID, kind: 'code' })
    return {
      label: 'Message',
      body: composeMessageMarkdown(message, subject),
      metadata: metadata.length > 0 ? metadata : undefined,
    }
  }
  return null
}
