import { useMemo } from 'react'
import { useProjectSpaces } from '../../hooks/useProjectSpaces'

function normalize(value: string | null | undefined): string {
  return (value ?? '').trim()
}

export function resolveSpaceLabelValue(
  explicitSpaceLabel: string,
  ownerID: string,
  byID: Map<string, string>,
): string {
  if (explicitSpaceLabel) return explicitSpaceLabel
  if (!ownerID) return ''
  return normalize(byID.get(ownerID))
}

/**
 * Resolves a stable space-facing label for Strategy panel entities, including archived/deleted spaces.
 */
export function useStrategySpaceLabel(projectId?: string | null) {
  const spacesQuery = useProjectSpaces(projectId, {
    includeDeleted: true,
    refetchInterval: 30_000,
  })

  const spaceLabelByID = useMemo(() => {
    const out = new Map<string, string>()
    for (const space of spacesQuery.data ?? []) {
      const ownerID = normalize(space.spaceId)
      const spaceLabel = normalize(space.spaceName)
      if (ownerID && spaceLabel) out.set(ownerID, spaceLabel)
    }
    return out
  }, [spacesQuery.data])

  const resolveSpaceLabel = (options: {
    spaceLabel?: string | null
    spaceId?: string | null
  }): string => {
    const explicitSpaceLabel = normalize(options.spaceLabel)
    const ownerID = normalize(options.spaceId)
    return resolveSpaceLabelValue(explicitSpaceLabel, ownerID, spaceLabelByID)
  }

  return {
    resolveSpaceLabel,
  }
}
