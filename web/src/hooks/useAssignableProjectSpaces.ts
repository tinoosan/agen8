import { useMemo } from 'react'
import { useProjectSpaces } from './useProjectSpaces'
import { useSpaceList } from './useSpace'
import type { ProjectSpaceSummary, Space } from '../lib/types'

export function useAssignableProjectSpaces(projectId: string | null | undefined, options?: { includeDeleted?: boolean }) {
  const projectSpacesQuery = useProjectSpaces(projectId, options)
  const spaceListQuery = useSpaceList({
    projectId,
    status: 'open',
    limit: 500,
    enabled: !!projectId,
  })

  const spaces = useMemo(
    () => mergeProjectSpaces(projectSpacesQuery.data ?? [], (spaceListQuery.data ?? []).map(spaceToProjectSummary)),
    [projectSpacesQuery.data, spaceListQuery.data],
  )

  return {
    ...projectSpacesQuery,
    data: spaces,
    isLoading: projectSpacesQuery.isLoading || spaceListQuery.isLoading,
    isError: projectSpacesQuery.isError || spaceListQuery.isError,
    error: projectSpacesQuery.error ?? spaceListQuery.error,
  }
}

export function mergeProjectSpaces(...groups: ProjectSpaceSummary[][]): ProjectSpaceSummary[] {
  const seen = new Set<string>()
  const out: ProjectSpaceSummary[] = []
  for (const group of groups) {
    for (const space of group) {
      const id = (space.spaceId ?? '').trim()
      if (!id || seen.has(id)) continue
      seen.add(id)
      out.push(space)
    }
  }
  return out
}

function spaceToProjectSummary(space: Space): ProjectSpaceSummary {
  return {
    projectId: space.projectId ?? '',
    spaceId: space.id,
    title: space.title,
    spaceName: space.title ?? space.id,
    status: space.status ?? 'open',
    sortOrder: 0,
    pinned: false,
    spaceOpen: (space.status ?? 'open') === 'open',
    createdAt: space.createdAt,
    updatedAt: space.updatedAt,
  }
}

