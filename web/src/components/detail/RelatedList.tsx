import type { ReactNode } from 'react'
import { Link } from 'wouter'
import { ChevronRight } from 'lucide-react'
import { CollapsibleSection } from '../strategy/CollapsibleSection'

/* ── Cross-reference list for entity detail pages ── */

// The "Related" section repeated across MissionDetail, TaskDetail, and
// DecisionDetail: a CollapsibleSection wrapping a flat list of labelled rows
// (uppercase 78px label + truncated title + optional suffix + ChevronRight).
// Each page builds its own RelatedItem[] — the link targets and suffixes differ
// per entity — but the row chrome is identical, so only the rendering is shared.
// `suffix` is a ReactNode (usually a "%" string); `suffixColor` overrides the
// default muted color (e.g. decision confidence tints). Renders nothing when
// `items` is empty, so callers can drop their own length guard.

export type RelatedItem = {
  key: string
  label: string
  title: string
  to: string
  suffix?: ReactNode
  suffixColor?: string
}

export function RelatedList({
  items,
  storageKey,
  label = 'Related',
}: {
  items: RelatedItem[]
  storageKey: string
  label?: ReactNode
}) {
  if (items.length === 0) return null
  return (
    <CollapsibleSection storageKey={storageKey} defaultOpen label={label}>
      <div className="flex flex-col" style={{ borderTop: '1px solid var(--border)' }}>
        {items.map((item, i) => (
          <Link
            key={item.key}
            to={item.to}
            className="flex items-center gap-2 py-2.5 no-underline group"
            style={{ borderBottom: i < items.length - 1 ? '1px solid var(--border)' : 'none' }}
          >
            <span style={{ fontSize: '0.625rem', fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: 'var(--text-3)', width: 78 }}>
              {item.label}
            </span>
            <span className="flex-1 min-w-0 truncate text-[var(--text-1)] group-hover:text-[var(--accent)] transition-colors" style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}>
              {item.title}
            </span>
            {item.suffix && (
              <span className="shrink-0 tabular-nums" style={{ fontSize: '0.6875rem', color: item.suffixColor ?? 'var(--text-3)' }}>
                {item.suffix}
              </span>
            )}
            <ChevronRight size={13} className="shrink-0 text-[var(--text-3)]" />
          </Link>
        ))}
      </div>
    </CollapsibleSection>
  )
}
