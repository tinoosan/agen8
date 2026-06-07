import type { ReactNode } from 'react'
import { Skeleton } from '@/components/ui/skeleton'

/* ── Loading skeleton scaffold for entity detail pages ── */

// Renders the shared wrapper plus the standard three-line header
// (back-link, title, subtitle). Each detail page passes its own body as
// children — the bodies genuinely differ (stacked summary cards on
// Task/Decision, KR rows on Mission), so only the scaffold is shared.

export function DetailSkeleton({ children }: { children: ReactNode }) {
  return (
    <div className="px-6 pt-8 max-w-4xl mx-auto w-full">
      <Skeleton className="h-4 w-20 mb-6" />
      <Skeleton className="h-8 w-96 mb-3" />
      <Skeleton className="h-4 w-40 mb-8" />
      {children}
    </div>
  )
}
