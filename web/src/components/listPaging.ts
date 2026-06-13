/** Clamp + slice helper for client-side paging over an already-fetched list. */
export function pageSlice<T>(items: T[], page: number, pageSize: number): T[] {
  const start = Math.max(0, (page - 1) * pageSize)
  return items.slice(start, start + pageSize)
}
