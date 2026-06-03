export function resolveUpdatedByActor(
  updatedBy: string | undefined,
  resolveSpaceLabel: (options: { spaceLabel?: string | null; spaceId?: string | null }) => string,
): string {
  if (!updatedBy) return ''
  const match = /^member:([^/]+)(\/.*)?$/.exec(updatedBy.trim())
  if (!match) return updatedBy
  const spaceId = match[1]
  const suffix = match[2] ?? ''
  const spaceLabel = resolveSpaceLabel({ spaceId })
  if (!spaceLabel) return updatedBy
  return `member:${spaceLabel}${suffix}`
}
