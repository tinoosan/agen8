export interface SpaceRegistryRow {
  spaceName?: string
  displayName?: string
  spaceRef?: string
  spaceId?: string
  coordinatorChannelId?: string
  coordinatorRole?: string
  status?: string
  roles?: string[]
  memberCount?: number
  members?: SpaceRegistryMember[]
}

export interface SpaceRegistryMember {
  memberId?: string
  displayName?: string
  memberType?: string
  lifecycleState?: string
  channelId?: string
  harnessKind?: string
  provider?: string
  model?: string
  reasoningEffort?: string
}

function clean(v: unknown): string {
  return String(v ?? '').trim()
}

function parseRoles(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(clean).filter(Boolean)
  const raw = clean(value)
  if (!raw) return []
  return raw.split(',').map(clean).filter(Boolean)
}

function parseMembers(value: unknown): SpaceRegistryMember[] {
  if (!Array.isArray(value)) return []
  return value
    .filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === 'object' && !Array.isArray(item))
    .map((item) => ({
      memberId: clean(item.memberId ?? item.id),
      displayName: clean(item.displayName ?? item.name),
      memberType: clean(item.memberType ?? item.type),
      lifecycleState: clean(item.lifecycleState ?? item.status),
      channelId: clean(item.channelId),
      harnessKind: clean(item.harnessKind),
      provider: clean(item.provider),
      model: clean(item.model),
      reasoningEffort: clean(item.reasoningEffort),
    }))
}

function parseObjectRow(value: Record<string, unknown>): SpaceRegistryRow | null {
  const spaceName = clean(value.spaceName ?? value.space_name ?? value.space ?? value.threadName ?? value.thread_title)
  const spaceRef = clean(value.spaceRef ?? value.space_ref)
  const displayName = clean(value.displayName ?? value.display_name ?? value.name ?? value.title)
  const coordinatorRole = clean(value.coordinatorRole ?? value.coordinator_role ?? value.coordinator)
  const spaceId = clean(value.spaceId ?? value.spaceID ?? value.space_id ?? value.defaultSpaceId ?? value.default_space_id)
  if (!spaceId && !spaceName && !displayName) return null
  const members = parseMembers(value.members)
  const rawMemberCount = Number(value.memberCount ?? value.member_count)
  return {
    spaceName,
    displayName: displayName || spaceName || spaceRef || spaceId,
    spaceRef,
    spaceId,
    coordinatorChannelId: clean(value.coordinatorChannelId ?? value.coordinator_channel_id),
    coordinatorRole,
    status: clean(value.status) || 'active',
    roles: parseRoles(value.roles ?? value.roleNames ?? value.role_names ?? value.availableRoles ?? value.available_roles),
    memberCount: Number.isFinite(rawMemberCount) ? rawMemberCount : members.length,
    members,
  }
}

function parsePipeEntryLine(line: string): SpaceRegistryRow | null {
  const trimmed = clean(line)
  if (!trimmed) return null
  const content = trimmed.startsWith('- ') ? trimmed.slice(2).trim() : trimmed
  const parts = content.split('|').map(clean).filter(Boolean)
  if (parts.length < 2) return null
  const displayName = clean(parts[0])
  if (!displayName || displayName.length > 80 || /[<>{}=]/.test(displayName)) return null
  const row: SpaceRegistryRow = { displayName }
  for (const part of parts.slice(1)) {
    const [rawKey, ...rest] = part.split('=')
    const key = clean(rawKey).toLowerCase()
    const value = clean(rest.join('='))
    if (!key || !value) continue
    if (key === 'status') row.status = value
    if (key === 'roles') row.roles = parseRoles(value)
  }
  return row
}

export function parseSpaceRegistryPayload(value: unknown, depth = 0): SpaceRegistryRow[] | null {
  if (depth > 4 || value == null) return null
  if (typeof value === 'string') {
    const trimmed = clean(value)
    if (!trimmed) return null
    if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) {
      const row = parsePipeEntryLine(trimmed)
      return row ? [row] : null
    }
    try {
      return parseSpaceRegistryPayload(JSON.parse(trimmed), depth + 1)
    } catch {
      return null
    }
  }
  if (Array.isArray(value)) {
    if (value.length === 0) return []
    const parsedObjectRows = value
      .filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === 'object' && !Array.isArray(item))
      .map(parseObjectRow)
      .filter((row): row is SpaceRegistryRow => row != null)
    if (parsedObjectRows.length > 0) return parsedObjectRows

    const parsedTextRows = value
      .filter((item): item is string => typeof item === 'string')
      .map(parsePipeEntryLine)
      .filter((row): row is SpaceRegistryRow => row != null)
    return parsedTextRows.length > 0 ? parsedTextRows : null
  }
  if (typeof value !== 'object') return null
  const record = value as Record<string, unknown>
  const directRow = parseObjectRow(record)
  if (directRow) return [directRow]
  if (Array.isArray(record.spaces)) {
    return parseSpaceRegistryPayload(record.spaces, depth + 1) ?? []
  }
  for (const key of ['spaces', 'items', 'data', 'entries', 'body', 'result', 'responseText', 'output', 'repr', 'text']) {
    const parsed = parseSpaceRegistryPayload(record[key], depth + 1)
    if (parsed) return parsed
  }
  return null
}

export function parseSpaceListTranscript(text: string): SpaceRegistryRow[] | null {
  const body = clean(text)
  const structured = parseSpaceRegistryPayload(body)
  if (structured) return structured
  if (!/(?:space\(action=["']?list|space\.list|mcp__agen8__\.?space|agen8:space)/i.test(body)) return null
  if (!/(?:returned|shows|confirms)\s+\d+\s+(?:active\s+)?spaces?/i.test(body)) return null

  const rows: SpaceRegistryRow[] = []
  for (const rawLine of body.split('\n')) {
    const line = clean(rawLine).replace(/^[-*]\s*/, '')
    if (!line || !line.includes('—')) continue
    const [rawSpaceRef, ...restParts] = line.split('—')
    const spaceRef = clean(rawSpaceRef).replace(/^`|`$/g, '')
    if (!spaceRef || spaceRef.length > 80 || /[<>{}=]/.test(spaceRef)) continue
    const rest = restParts.join('—')
    if (!/\bcoordinator\b/i.test(rest)) continue
    const spaceName = clean(rest.match(/\bspace\s+([^,\n]+)/i)?.[1])
    const coordinatorRole = clean(rest.match(/\bcoordinator\s+([^,\n]+)/i)?.[1])
    rows.push({
      spaceName,
      displayName: spaceName || spaceRef,
      spaceRef,
      coordinatorRole,
      status: 'active',
    })
  }

  return rows.length > 0 ? rows : null
}
