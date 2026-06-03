/**
 * Resolves a taskRef to a display string.
 * Returns null if there is no taskRef at all.
 * Returns `{ text, isDeleted }` so callers can render the TaskDeletedBadge.
 */
export function taskRefDisplay(
  taskRef: string | null | undefined,
  task: { id: string; title?: string } | null | undefined,
): { text: string; isDeleted: boolean } | null {
  if (!taskRef) return null
  if (!task) return { text: '[Task deleted]', isDeleted: true }
  return { text: task.title || taskRef, isDeleted: false }
}
