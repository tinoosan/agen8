import type { AgentEvent } from '../../lib/types'

export interface GraphQueryInspectorMetadata {
  label: string
  value: string
  kind?: 'text' | 'code'
}

export interface GraphQueryInspectorRender {
  label: string
  body: string
  metadata?: GraphQueryInspectorMetadata[]
  confidence?: number
}

function clean(value: unknown): string {
  return String(value ?? '').trim()
}

function displayToken(value: unknown): string {
  return clean(value)
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

function parseNestedGraphPayload(raw: unknown, depth = 0): Record<string, unknown> | null {
  if (raw == null || depth > 4) return null
  if (typeof raw === 'string') {
    const parsed = parseJSONRecord(raw)
    return parsed ? parseNestedGraphPayload(parsed, depth + 1) : null
  }
  if (typeof raw !== 'object' || Array.isArray(raw)) return null
  const record = raw as Record<string, unknown>
  if (
    record.action !== undefined
    || record.node !== undefined
    || record.results !== undefined
    || record.edge !== undefined
    || record.edges !== undefined
    || record.warnings !== undefined
    || record.node_type !== undefined
  ) {
    return record
  }
  for (const key of ['body', 'text', 'result', 'output', 'responseText', 'repr']) {
    const parsed = parseNestedGraphPayload(record[key], depth + 1)
    if (parsed) return parsed
  }
  return null
}

function asRecord(raw: unknown): Record<string, unknown> | null {
  return raw && typeof raw === 'object' && !Array.isArray(raw) ? raw as Record<string, unknown> : null
}

function asRecordList(raw: unknown): Record<string, unknown>[] {
  return Array.isArray(raw)
    ? raw.filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === 'object' && !Array.isArray(item))
    : []
}

function field(record: Record<string, unknown> | null | undefined, ...keys: string[]): string {
  if (!record) return ''
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
    if (typeof value === 'number' && Number.isFinite(value)) return String(value)
    if (typeof value === 'boolean') return value ? 'true' : 'false'
  }
  return ''
}

function appendMetadata(
  metadata: GraphQueryInspectorMetadata[],
  label: string,
  value: string,
  kind?: 'text' | 'code',
): void {
  if (!value) return
  metadata.push({ label, value, kind })
}

function renderNode(payload: Record<string, unknown>, node: Record<string, unknown>): GraphQueryInspectorRender {
  const title = field(node, 'title') || field(payload, 'node_id', 'nodeId') || 'Graph node'
  const type = field(node, 'type') || field(payload, 'node_type', 'nodeType')
  const id = field(node, 'id') || field(payload, 'node_id', 'nodeId')
  const status = field(node, 'status')
  const spaceName = field(node, 'spaceName', 'space_name')
  const fields = asRecord(node.fields)
  const keyFields = [
    field(fields, 'rationale'),
    field(fields, 'alternativesRejected'),
    field(fields, 'summary'),
    field(fields, 'description'),
  ].filter(Boolean)
  const bodyParts = [`**${title}**`]
  if (keyFields.length > 0) {
    bodyParts.push('\n' + keyFields.join('\n\n'))
  }

  const neighbours = asRecordList(node.neighbours)
  if (neighbours.length > 0) {
    bodyParts.push('\n**Neighbours**')
    bodyParts.push(neighbours.slice(0, 8).map((entry) => {
      const entryTitle = field(entry, 'title') || field(entry, 'id')
      const entryType = displayToken(field(entry, 'type'))
      const entryStatus = displayToken(field(entry, 'status'))
      const suffix = [entryType, entryStatus].filter(Boolean).join(' · ')
      return suffix ? `- ${entryTitle} — ${suffix}` : `- ${entryTitle}`
    }).join('\n'))
  }

  const metadata: GraphQueryInspectorMetadata[] = []
  appendMetadata(metadata, 'Action', displayToken(field(payload, 'action')))
  appendMetadata(metadata, 'Type', displayToken(type))
  appendMetadata(metadata, 'Status', displayToken(status))
  appendMetadata(metadata, 'Space', spaceName, 'code')
  appendMetadata(metadata, 'Node ID', id, 'code')
  return {
    label: 'Graph node',
    body: bodyParts.join('\n'),
    metadata: metadata.length > 0 ? metadata : undefined,
    confidence: Number(field(fields, 'confidence')) || undefined,
  }
}

function renderSearch(payload: Record<string, unknown>): GraphQueryInspectorRender {
  const results = asRecordList(payload.results)
  const query = field(payload, 'query')
  const nodeType = field(payload, 'node_type', 'nodeType') || 'all'
  const body = results.length > 0
    ? results.slice(0, 25).map((entry) => {
        const title = field(entry, 'title') || field(entry, 'id') || '(untitled node)'
        const details = [
          displayToken(field(entry, 'type')),
          displayToken(field(entry, 'status')),
          field(entry, 'spaceName', 'space_name'),
        ].filter(Boolean).join(' · ')
        return details ? `- **${title}** — ${details}` : `- **${title}**`
      }).join('\n')
    : 'No graph nodes matched this query.'

  const metadata: GraphQueryInspectorMetadata[] = []
  appendMetadata(metadata, 'Action', 'Search')
  appendMetadata(metadata, 'Scope', nodeType)
  appendMetadata(metadata, 'Query', query)
  appendMetadata(metadata, 'Results', String(results.length))
  return {
    label: 'Graph search',
    body,
    metadata,
  }
}

function renderEdge(payload: Record<string, unknown>): GraphQueryInspectorRender {
  const edge = asRecord(payload.edge) ?? asRecordList(payload.edges)[0]
  const action = displayToken(field(payload, 'action')) || 'Link'
  const source = edge ? `${field(edge, 'sourceType')}/${field(edge, 'sourceID', 'sourceId')}` : ''
  const target = edge ? `${field(edge, 'targetType')}/${field(edge, 'targetID', 'targetId')}` : ''
  const edgeType = edge ? field(edge, 'edgeType', 'edge_type') : ''
  const rationale = edge ? field(edge, 'rationale') : ''
  const metadata: GraphQueryInspectorMetadata[] = []
  appendMetadata(metadata, 'Action', action)
  appendMetadata(metadata, 'Edge', displayToken(edgeType))
  appendMetadata(metadata, 'Source', source, 'code')
  appendMetadata(metadata, 'Target', target, 'code')
  const body = [
    source && target && edgeType ? `**${source}** -> **${target}**` : 'Graph edge updated.',
    rationale,
  ].filter(Boolean).join('\n\n')
  return {
    label: action === 'Unlink' ? 'Graph unlink' : 'Graph link',
    body,
    metadata: metadata.length > 0 ? metadata : undefined,
    confidence: Number(field(edge, 'confidence')) || undefined,
  }
}

export function resolveGraphQueryRender(event: AgentEvent): GraphQueryInspectorRender | null {
  const data = event.data ?? {}
  const candidates = [
    data.result,
    data.output,
    data.outputFull,
    data.outputPreview,
    event.outputPreview,
    event.textPreview,
    data.response,
  ]
  for (const candidate of candidates) {
    const payload = parseNestedGraphPayload(candidate)
    if (!payload) continue
    const action = field(payload, 'action').toLowerCase()
    const node = asRecord(payload.node)
    if (action === 'node' && node) return renderNode(payload, node)
    if (action === 'search') return renderSearch(payload)
    if (action === 'link' || action === 'unlink') return renderEdge(payload)
    if (node) return renderNode(payload, node)
    if (payload.results !== undefined) return renderSearch(payload)
    if (payload.edge !== undefined || payload.edges !== undefined) return renderEdge(payload)
  }
  return null
}
