import { rpcCall, rpcUnwrap, rpcUnwrapList } from './rpc'
import type { Project, ProjectCustomization } from './types'

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

// Safe-to-display view of a minted link token. Carries the prefix and lifecycle
// timestamps but never the raw token or hash. `status` collapses the server's
// (active, revokedAt, expiresAt) evaluation into one word the UI renders directly.
export interface LinkTokenSummary {
  id: string
  prefix: string
  projectId: string
  workspaceId?: string
  label?: string
  status: 'active' | 'revoked' | 'expired'
  expiresAt?: string
  revokedAt?: string
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

/**
 * Lists the link tokens bound to a caller-owned project. Summaries carry no
 * secret, so the server gates this on ownership only: a non-owner cannot
 * enumerate a project's tokens. Returns [] when the project has none.
 */
export async function listLinkTokens(projectId: string): Promise<LinkTokenSummary[]> {
  const id = projectId.trim()
  if (!id) throw new Error('project id is required to list link tokens')
  return rpcUnwrapList<LinkTokenSummary>('project.linkToken.list', { projectId: id }, 'tokens')
}

/**
 * Revokes one of a caller-owned project's link tokens. The server re-derives the
 * project's own token set and rejects a tokenId that is not in it, so a caller
 * who owns project A can never revoke project B's token by guessing its id.
 */
export async function revokeLinkToken(projectId: string, tokenId: string): Promise<void> {
  const id = projectId.trim()
  if (!id) throw new Error('project id is required to revoke a link token')
  const token = tokenId.trim()
  if (!token) throw new Error('token id is required to revoke a link token')
  await rpcCall<Record<string, never>>('project.linkToken.revoke', { projectId: id, tokenId: token })
}

/**
 * Edits a caller-owned project's user-facing fields (title, customization).
 * Fields are sent only when provided, mirroring the server's "nil = leave
 * alone" contract: omit `title` to keep the current name, pass `''` to clear a
 * custom name and fall back to the folder name. The server gates this on
 * ownership, so a non-owner is rejected loudly rather than silently editing.
 */
export async function updateProject(
  projectId: string,
  changes: { title?: string; customization?: ProjectCustomization },
): Promise<Project> {
  const id = projectId.trim()
  if (!id) throw new Error('project id is required to update a project')
  const params: { projectId: string; title?: string; customization?: ProjectCustomization } = {
    projectId: id,
  }
  if (changes.title !== undefined) params.title = changes.title
  if (changes.customization !== undefined) params.customization = changes.customization
  return rpcUnwrap<Project>('project.update', params, 'project')
}
