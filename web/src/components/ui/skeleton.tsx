import { cn } from "@/lib/utils"

/**
 * Skeleton loader using the project's shimmer animation.
 * Matches the existing `.skeleton` CSS class so the migration
 * from `<div className="skeleton">` is visually identical.
 */
function Skeleton({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("skeleton rounded-[var(--r-sm)]", className)}
      {...props}
    />
  )
}

export { Skeleton }
