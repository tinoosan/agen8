import { cn } from '@/lib/utils'

/** Inline indicator for deleted/missing task references (PRD F38). */
export function TaskDeletedBadge({ className }: { className?: string }) {
  return (
    <span className={cn('text-[var(--text-3)] italic text-xs', className)}>
      [Task deleted]
    </span>
  )
}
