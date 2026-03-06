import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { TeamGetStatusResult, TeamGetManifestResult } from '../lib/types'

const DETACHED = 'detached-control'

export function useTeamStatus(teamId: string | null) {
  return useQuery<TeamGetStatusResult>({
    queryKey: ['team.getStatus', teamId],
    queryFn: () =>
      rpcCall<TeamGetStatusResult>('team.getStatus', { threadId: DETACHED, teamId }),
    enabled: !!teamId,
    refetchInterval: 1500,
    retry: false,
  })
}

export function useTeamManifest(teamId: string | null) {
  return useQuery<TeamGetManifestResult>({
    queryKey: ['team.getManifest', teamId],
    queryFn: () =>
      rpcCall<TeamGetManifestResult>('team.getManifest', { threadId: DETACHED, teamId }),
    enabled: !!teamId,
    refetchInterval: 10000,
    retry: false,
  })
}
