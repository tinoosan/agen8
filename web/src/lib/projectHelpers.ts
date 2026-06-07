/**
 * Pure project helpers — display-name resolution and other project-level
 * formatting that any view can reuse.
 */
import type { Project } from './types'

/**
 * Resolve a human-friendly display name for a project.
 * Priority: title → root folder name → id. The folder beats the raw id so a
 * titleless project shows something readable instead of an opaque identifier.
 */
export function projectDisplayName(project: Project): string {
  if (project.title?.trim()) return project.title.trim()
  const folder = project.root.split('/').filter(Boolean).at(-1)
  return folder || project.id
}
