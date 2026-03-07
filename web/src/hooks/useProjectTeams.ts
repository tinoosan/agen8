import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { onNotification, rpcCall } from '../lib/rpc'
import type { ProjectReconcileNotification, ProjectTeamSummary } from '../lib/types'

export function useProjectTeams() {
  const queryClient = useQueryClient()
  const query = useQuery<ProjectTeamSummary[]>({
    queryKey: ['project.listTeams'],
    queryFn: async () => {
      const res = await rpcCall<{ teams: ProjectTeamSummary[] }>(
        'project.listTeams',
        {}
      )
      return res.teams ?? []
    },
    refetchInterval: 2000,
    retry: false,
  })

  useEffect(() => {
    const methods = [
      'project.reconcile.started',
      'project.reconcile.drift',
      'project.reconcile.converged',
      'project.reconcile.failed',
    ] as const

    const unsubs = methods.map((method) =>
      onNotification(method, (notification) => {
        const params = notification.params as ProjectReconcileNotification | undefined
        if (params?.projectRoot === '' || params?.projectRoot === undefined) {
          queryClient.invalidateQueries({ queryKey: ['project.listTeams'] })
          return
        }
        queryClient.invalidateQueries({ queryKey: ['project.listTeams'] })
      }),
    )

    return () => {
      for (const unsub of unsubs) unsub()
    }
  }, [queryClient])

  return query
}

export function useProjectContext() {
  return useQuery({
    queryKey: ['project.getContext'],
    queryFn: () =>
      rpcCall<{ context: { config: { projectId: string }; state: { activeTeamId: string } } }>(
        'project.getContext',
        {}
      ),
    refetchInterval: 5000,
    retry: false,
  })
}
