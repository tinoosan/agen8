import { useMemo } from 'react'
import { useNavigation } from '../lib/routing'
import { useProjects } from './useProjects'

/**
 * Derives the active profileId from the current project context.
 * Uses the project's defaultProfile if available, falls back to the projectId.
 */
export function useProfileId(): string {
  const { projectId } = useNavigation()
  const { data: projects = [] } = useProjects()
  return useMemo(() => {
    const project = projects.find(p => p.id === projectId)
    return project?.id ?? projectId ?? 'default'
  }, [projects, projectId])
}
