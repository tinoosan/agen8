import type { ProjectSpaceSummary } from './types'

export function spaceDisplayName(
  _spaceId?: string | null,
  spaceName?: string | null,
): string {
  const name = (spaceName ?? '').trim()
  if (name) {
    const segments = name.split('/')
    const last = (segments[segments.length - 1] ?? '').trim()
    if (last) {
      return last.replace(/[-_]/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
    }
  }
  return 'Space'
}

export function spaceDisplayNameFromLookup(
  spaceId: string,
  lookup: Map<string, ProjectSpaceSummary>,
): string {
  const space = lookup.get(spaceId)
  return spaceDisplayName(spaceId, space?.spaceName)
}
