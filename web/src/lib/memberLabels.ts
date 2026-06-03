import type { ProjectSpaceSummary, SpaceMember } from './types'

function clean(value: unknown): string {
  return String(value ?? '').trim()
}

export function buildMemberLabelMap(spaces: ProjectSpaceSummary[] | null | undefined): Map<string, string> {
  const labels = new Map<string, string>()
  for (const space of spaces ?? []) {
    for (const raw of space.members ?? []) {
      const record = raw as unknown as Record<string, unknown>
      const id = clean(record.memberId ?? record.id ?? record.member)
      const label = clean(record.label ?? record.displayName ?? record.name)
      if (id && label) labels.set(id, label)
    }
  }
  return labels
}

export function memberDisplayLabel(member: Pick<SpaceMember, 'id' | 'displayName' | 'memberType'> | null | undefined): string {
  return clean(member?.displayName) || clean(member?.memberType) || clean(member?.id) || 'Member'
}

export function resolveMemberLabel(memberId: string | null | undefined, labels: Map<string, string>): string {
  const id = clean(memberId)
  if (!id) return ''
  return labels.get(id) || id
}
