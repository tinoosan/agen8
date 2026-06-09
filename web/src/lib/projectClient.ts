import { rpcCall, rpcUnwrap } from './rpc'
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
