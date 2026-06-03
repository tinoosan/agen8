import type { OACategory } from './types'

const DUPLICATE_WINDOW_MS = 5 * 60 * 1000 // 5 minutes

interface OACandidate {
  title: string
  taskRef?: string
  category: OACategory
  createdAt: string
}

/**
 * Detects whether an operator action is a duplicate of an existing one.
 *
 * Detection criteria (per PRD F38):
 *   Same `title` + `taskRef` + `category` within a 5-minute window.
 *
 * The window is exclusive at the boundary (items created exactly 5 minutes
 * ago are NOT considered duplicates).
 */
export function isDuplicateOA(
  title: string,
  taskRef: string | undefined | null,
  category: OACategory,
  existing: OACandidate[],
): boolean {
  const now = Date.now()

  for (const item of existing) {
    const ageMs = now - new Date(item.createdAt).getTime()

    // Outside the 5-minute window (exclusive boundary)
    if (ageMs >= DUPLICATE_WINDOW_MS) continue

    // All three fields must match
    if (
      item.title === title &&
      (item.taskRef ?? undefined) === (taskRef ?? undefined) &&
      item.category === category
    ) {
      return true
    }
  }

  return false
}
