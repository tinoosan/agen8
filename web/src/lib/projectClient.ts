import { rpcCall } from './rpc'

// Result of `project.linkToken.create`. The wlt_ `token` is server-minted and
// shown exactly once — it is never returned again.
export interface LinkTokenResult {
  id: string
  prefix: string
  token: string
  projectId: string
  workspaceId?: string
  label?: string
  expiresAt?: string
  createdAt?: string
}

/**
 * Mints a wlt_ link token bound to the caller-owned project. The server gates
 * this on project ownership, so a non-owner (or unauthenticated caller) is
 * rejected loudly rather than receiving a token.
 */
export async function createLinkToken(projectId: string, label?: string): Promise<LinkTokenResult> {
  const trimmedProject = projectId.trim()
  if (!trimmedProject) throw new Error('project id is required to mint a link token')
  const params: { projectId: string; label?: string } = { projectId: trimmedProject }
  const trimmedLabel = label?.trim()
  if (trimmedLabel) params.label = trimmedLabel
  return rpcCall<LinkTokenResult>('project.linkToken.create', params)
}
