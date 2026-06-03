import type { KeyResultView, ProjectSpaceSummary } from './types'
import { safeReferenceLabel, sanitizeSpaceTitle } from './displaySanitizers'

export function spaceSummaryLabel(space: ProjectSpaceSummary | null | undefined): string {
  return sanitizeSpaceTitle(space?.spaceName) ?? 'Space'
}

export function keyResultSpaceOwnerLabel(spaceID: string | null | undefined, spaces: ProjectSpaceSummary[]): string | null {
  const id = (spaceID ?? '').trim()
  if (!id) return null
  const space = spaces.find(item => item.spaceId === id)
  if (space) return spaceSummaryLabel(space)
  return safeReferenceLabel(id)
}

export function keyResultSpaceOwnerLabelFromKR(kr: Pick<KeyResultView, 'spaceId' | 'ownerSpaceName'>, spaces: ProjectSpaceSummary[]): string | null {
  const spaceLabel = keyResultSpaceOwnerLabel(kr.spaceId, spaces)
  if (spaceLabel && spaceLabel !== safeReferenceLabel(kr.spaceId)) return spaceLabel

  const space = sanitizeSpaceTitle(kr.ownerSpaceName)
  return space ?? spaceLabel
}

export function isAssignableSpace(space: ProjectSpaceSummary): boolean {
  const status = (space.status ?? '').trim().toLocaleLowerCase()
  if (status === 'archived' || status === 'deleted' || status === 'inactive') return false
  if ((space.lifecyclePhase ?? '').trim().toLocaleLowerCase() === 'deleting') return false
  return Boolean((space.spaceId ?? '').trim())
}

export function assignableSpaces(spaces: ProjectSpaceSummary[]): ProjectSpaceSummary[] {
  const seen = new Set<string>()
  const out: ProjectSpaceSummary[] = []
  for (const space of spaces) {
    if (!isAssignableSpace(space)) continue
    const key = [
      (space.projectRoot ?? space.projectId ?? '').trim().toLocaleLowerCase(),
      (space.spaceName ?? '').trim().toLocaleLowerCase(),
    ].join('\u0000')
    if (seen.has(key)) continue
    seen.add(key)
    out.push(space)
  }
  return out
}
