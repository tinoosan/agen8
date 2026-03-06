import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { ArtifactNode } from '../lib/types'

interface ArtifactListResult {
  nodes: ArtifactNode[]
}

export function useArtifactFiles(threadId: string | null, teamId: string | null) {
  return useQuery<ArtifactNode[]>({
    queryKey: ['artifact.list.files', threadId, teamId],
    queryFn: async () => {
      const res = await rpcCall<ArtifactListResult>('artifact.list', {
        threadId: threadId ?? undefined,
        teamId: teamId ?? undefined,
      })
      return (res.nodes ?? []).filter((node) => node.kind === 'file' && (node.vpath ?? '').startsWith('/workspace/'))
    },
    enabled: !!threadId && !!teamId,
    refetchInterval: 5000,
    staleTime: 2000,
    retry: false,
  })
}
