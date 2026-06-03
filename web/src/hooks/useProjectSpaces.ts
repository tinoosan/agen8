import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { ProjectSpaceSummary } from '../lib/types'

interface UseProjectSpacesOptions {
  refetchInterval?: number | false
  includeDeleted?: boolean
}

export function getProjectSpaceQueryKeysToInvalidate(projectId?: string | null): Array<[string, string]> {
  const keys: Array<[string, string]> = [['project.space.list', '']]
  if (projectId && projectId.trim() !== '') {
    keys.push(['project.space.list', projectId])
  }
  return keys
}

export function useProjectSpaces(projectId?: string | null, options?: UseProjectSpacesOptions) {
  const includeDeleted = options?.includeDeleted === true
  return useQuery<ProjectSpaceSummary[]>({
    queryKey: ['project.space.list', projectId ?? '', includeDeleted ? 'withDeleted' : 'active'],
    queryFn: async () => {
      if (!projectId) return []
      const res = await rpcCall<{ spaces: ProjectSpaceSummary[] }>(
        'project.space.list',
        { projectId }
      )
      return res.spaces ?? []
    },
    enabled: !!projectId,
    refetchInterval: options?.refetchInterval ?? 5000,
    retry: false,
    select: (spaces) => {
      const normalized = spaces.map((space) => ({
        ...space,
        spaceName: space.spaceName ?? space.title ?? space.spaceId,
      }))
      return includeDeleted ? normalized : normalized.filter((space) => space.spaceOpen)
    },
  })
}
