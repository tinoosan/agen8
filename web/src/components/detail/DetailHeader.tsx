import type { ReactNode } from 'react'
import { Link } from 'wouter'
import { ArrowLeft } from 'lucide-react'

/* ── Sticky back-link header for entity detail pages ── */

// Renders the sticky bar, its centered padding container, and the back link
// (ArrowLeft + label). The page-specific title row — title, badges, metadata,
// action buttons — is passed as children, since it differs per entity.

export function DetailHeader({
  backTo,
  backLabel,
  children,
}: {
  backTo: string
  backLabel: string
  children: ReactNode
}) {
  return (
    <div className="sticky top-0 z-10 bg-[var(--bg-app)] border-b border-[var(--border)]/60 w-full">
      <div className="px-6 pt-6 pb-4 max-w-4xl mx-auto w-full">
        <Link
          to={backTo}
          className="inline-flex items-center gap-1.5 text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors no-underline mb-5"
          style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}
        >
          <ArrowLeft size={13} />
          {backLabel}
        </Link>
        {children}
      </div>
    </div>
  )
}
