import { cn } from '@/lib/utils'
import type { Project } from '../../lib/types'
import { projectDisplayName } from '../../lib/projectHelpers'
import { PROJECT_ICONS, DEFAULT_PROJECT_COLOR } from '../../lib/projectCustomization'

/**
 * The colored glyph that stands in for a project wherever it's scanned or
 * navigated (projects list rows, sidebar entries). It renders the project's
 * customization icon tinted with its color, on a low-alpha wash of that color.
 *
 * Fallbacks keep it always-renderable: a project with no icon shows its
 * monogram (first letter of the display name), and a project with no color uses
 * a neutral token so an un-customized project reads as neutral rather than
 * picking an arbitrary hue.
 */
export function ProjectAvatar({
  project,
  size = 20,
  className,
}: {
  project: Project
  size?: number
  className?: string
}) {
  const color = project.customization?.color?.trim() || DEFAULT_PROJECT_COLOR
  // Index the curated map directly (rather than via a helper call) so the looked
  // up glyph is a stable component reference, not one "created during render".
  const iconKey = project.customization?.icon?.trim().toLowerCase()
  const Icon = iconKey ? PROJECT_ICONS[iconKey] : undefined
  const monogram = projectDisplayName(project).trim().charAt(0).toUpperCase() || '?'

  return (
    <span
      data-testid="project-avatar"
      aria-hidden
      className={cn('inline-flex shrink-0 items-center justify-center rounded-[var(--r-sm)]', className)}
      style={{
        width: size,
        height: size,
        color,
        backgroundColor: `color-mix(in srgb, ${color} 16%, transparent)`,
      }}
    >
      {Icon ? (
        <Icon size={Math.round(size * 0.58)} strokeWidth={2} />
      ) : (
        <span style={{ fontSize: Math.round(size * 0.5), fontWeight: 600, lineHeight: 1 }}>{monogram}</span>
      )}
    </span>
  )
}
