import { Button } from '@/components/ui/button'

/**
 * ListPager — the house pagination control: "Page X of Y" with
 * Previous/Next. One component so every paged surface (decisions, tasks,
 * missions) reads and behaves identically.
 *
 * Renders nothing when everything fits on one page — a pager over one page
 * is noise.
 */
export default function ListPager({
  page,
  totalPages,
  onPageChange,
}: {
  page: number
  totalPages: number
  onPageChange: (page: number) => void
}) {
  if (totalPages <= 1) return null
  return (
    <div className="flex items-center justify-between gap-3 text-[0.75rem] text-[var(--text-3)]">
      <span className="tabular-nums">Page {page} of {totalPages}</span>
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="sm" onClick={() => onPageChange(Math.max(1, page - 1))} disabled={page <= 1}>
          Previous
        </Button>
        <Button variant="ghost" size="sm" onClick={() => onPageChange(Math.min(totalPages, page + 1))} disabled={page >= totalPages}>
          Next
        </Button>
      </div>
    </div>
  )
}

/** Clamp + slice helper for client-side paging over an already-fetched list. */
export function pageSlice<T>(items: T[], page: number, pageSize: number): T[] {
  const start = Math.max(0, (page - 1) * pageSize)
  return items.slice(start, start + pageSize)
}
