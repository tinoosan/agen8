/**
 * Space-related pure helpers. Extracted from the sidebar so they can
 * be reused anywhere (URL validation, title normalization, display).
 */
import type { Project } from './types'

/** True if the ID matches the canonical `space-<uuid>` format. */
export function isCanonicalSpaceId(id: string | null | undefined): id is string {
  const value = (id ?? '').trim()
  return value.startsWith('space-')
}

/**
 * Normalize a user-typed space title into a stable slug identifier.
 * Lowercase, alphanumeric + hyphens only, no leading/trailing hyphens.
 */
export function normalizeSpaceTitle(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

/**
 * Resolve a human-friendly display name for a project.
 * Priority: title → id → last path segment of root.
 */
export function projectDisplayName(project: Project): string {
  if (project.title?.trim()) return project.title.trim()
  if (project.id) return project.id
  const segments = project.root.split('/')
  return segments[segments.length - 1] || project.root
}
