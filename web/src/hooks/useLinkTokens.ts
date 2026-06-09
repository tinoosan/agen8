import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { qk } from '../lib/queryKeys'
import { listLinkTokens, revokeLinkToken, type LinkTokenSummary } from '../lib/projectClient'

// Lists the link tokens bound to a project so the link dialog can show whether a
// folder is already linked and let the owner manage existing tokens. Summaries
// carry no secret; the server gates the call on project ownership.
export function useLinkTokens(projectId: string | null) {
  return useQuery<LinkTokenSummary[]>({
    queryKey: qk.linkTokens(projectId),
    queryFn: async () => listLinkTokens(projectId ?? ''),
    enabled: !!projectId,
  })
}

// Revokes one of a project's link tokens. The server re-derives the project's
// own token set and rejects an id outside it, so a caller can never revoke
// another project's token. Invalidates every cached token list on success so
// the revoked token flips to its new status immediately.
export function useRevokeLinkToken() {
  const queryClient = useQueryClient()
  return useMutation<void, Error, { projectId: string; tokenId: string }>({
    mutationFn: ({ projectId, tokenId }) => revokeLinkToken(projectId, tokenId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: qk.linkTokensAll })
    },
  })
}
