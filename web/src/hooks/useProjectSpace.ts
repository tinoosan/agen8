import { useQueryClient } from '@tanstack/react-query'
import { onNotification } from '../lib/rpc'
import { useEffect } from 'react'
import { useProjectSpaces } from './useProjectSpaces'

export function useProjectSpace(projectId: string | null, spaceId: string | null) {
  const queryClient = useQueryClient()
  const spacesQuery = useProjectSpaces(projectId, { refetchInterval: 3000, includeDeleted: true })

  useEffect(() => {
    const methods = [
      'project.reconcile.started',
      'project.reconcile.drift',
      'project.reconcile.converged',
      'project.reconcile.failed',
      'project.reconcile.diagnostic',
      'project.reconcile.recovered',
    ] as const
    const unsubs = methods.map((method) =>
      onNotification(method, (notification) => {
        const payload = notification.params as { projectId?: string } | undefined
        if (projectId && payload?.projectId && payload.projectId !== projectId) return
        queryClient.invalidateQueries({ queryKey: ['project.space.list', projectId ?? ''] })
      }),
    )
    return () => { unsubs.forEach((unsubscribe) => unsubscribe()) }
  }, [projectId, queryClient])

  return {
    ...spacesQuery,
    data: (spacesQuery.data ?? []).find((space) => space.spaceId === spaceId) ?? null,
  }
}
