import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { usePersistedToggle } from './usePersistedToggle'

interface Props {
  storageKey: string
  defaultOpen: boolean
  label: ReactNode
  accent?: string
  icon?: ReactNode
  children: ReactNode
}

/**
 * Collapsible section with persisted open/closed state.
 * Used in detail panels for grouping content (activity, reviews,
 * acceptance criteria, heartbeat, etc.).
 */
export function CollapsibleSection({
  storageKey,
  defaultOpen,
  label,
  accent,
  icon,
  children,
}: Props) {
  const [open, toggle] = usePersistedToggle(storageKey, defaultOpen)
  return (
    <div>
      <button
        type="button"
        className="flex items-center gap-1.5 w-full text-left"
        style={{ background: 'transparent', border: 0, padding: 0, cursor: 'pointer', marginBottom: open ? 4 : 0 }}
        onClick={toggle}
      >
        {open
          ? <ChevronDown size={10} style={{ color: accent ?? 'var(--text-3)', flexShrink: 0 }} />
          : <ChevronRight size={10} style={{ color: accent ?? 'var(--text-3)', flexShrink: 0 }} />}
        <span style={{ fontSize: '10px', fontWeight: 500, letterSpacing: '0.08em', lineHeight: 1.33, color: accent ?? 'var(--text-3)', textTransform: 'uppercase', display: 'flex', alignItems: 'center', gap: 4 }}>
          {icon}
          {label}
        </span>
      </button>
      {open && children}
    </div>
  )
}
