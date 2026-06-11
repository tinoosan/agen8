import { memo } from 'react'
import { Activity, Ban, CheckCircle2, Diamond, Search } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { FilterPreset } from './strategyMapFilters'

interface FilterButton {
  key: FilterPreset
  label: string
  icon: typeof Activity
  color: string
}

interface Props {
  activeFilter: FilterPreset | null
  onFilterChange: (filter: FilterPreset | null) => void
  matchCount: number
  /** Open the node-search panel. Gives touch users (iPad, where the mobile top
   *  bar is hidden and there's no keyboard for "/") a reachable way in, and
   *  makes search discoverable for everyone. */
  onOpenSearch: () => void
}

export const StrategyMapFilterBar = memo(function StrategyMapFilterBar({
  activeFilter,
  onFilterChange,
  matchCount,
  onOpenSearch,
}: Props) {
  const buttons: FilterButton[] = [
    { key: 'in_motion', label: 'In Motion', icon: Activity, color: 'var(--blue)' },
    { key: 'blocked', label: 'Blocked', icon: Ban, color: 'var(--amber)' },
    { key: 'done', label: 'Done', icon: CheckCircle2, color: 'var(--green)' },
    { key: 'decisions', label: 'Decisions', icon: Diamond, color: 'var(--accent)' },
  ]

  return (
    <div className="flex min-w-0 items-center gap-1.5 overflow-x-auto">
      {/* Search node — touch-reachable entry point to the node search. Always
          visible (the mobile top-bar search is md:hidden, so iPad/tablet have
          no other way in without a keyboard). Mirrors the inactive-pill style. */}
      <button
        type="button"
        onClick={onOpenSearch}
        aria-label="Search nodes"
        title="Search nodes (/)"
        className={cn(
          'inline-flex shrink-0 items-center justify-center transition-all duration-200',
          'focus:outline-none focus-visible:outline-none hover:scale-[1.03]',
        )}
        style={{
          width: 28,
          height: 28,
          borderRadius: 20,
          border: '1px solid var(--border)',
          background: 'var(--bg-panel)',
          color: 'var(--text-3)',
          boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
        }}
      >
        <Search size={13} />
      </button>

      {buttons.map((btn) => {
        const isActive = activeFilter === btn.key
        const Icon = btn.icon
        return (
          <button
            key={btn.key}
            type="button"
            onClick={() => onFilterChange(isActive ? null : btn.key)}
            className={cn(
              'inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap transition-all duration-200',
              'focus:outline-none focus-visible:outline-none',
              'hover:scale-[1.03]',
            )}
            style={{
              padding: '5px 10px',
              borderRadius: 20,
              fontSize: '0.6875rem',
              fontWeight: 600,
              letterSpacing: '0.01em',
              border: `1px solid ${isActive ? btn.color : 'var(--border)'}`,
              background: isActive
                ? `color-mix(in srgb, ${btn.color} 12%, var(--bg-panel))`
                : 'var(--bg-panel)',
              color: isActive ? btn.color : 'var(--text-3)',
              boxShadow: isActive
                ? `0 0 12px color-mix(in srgb, ${btn.color} 20%, transparent)`
                : '0 1px 3px rgba(0,0,0,0.08)',
            }}
          >
            <Icon size={12} />
            {btn.label}
            {isActive && (
              <span
                className="inline-flex items-center justify-center"
                style={{
                  minWidth: 16,
                  height: 16,
                  padding: '0 4px',
                  borderRadius: 8,
                  fontSize: '0.625rem',
                  fontWeight: 700,
                  background: `color-mix(in srgb, ${btn.color} 20%, transparent)`,
                  color: btn.color,
                }}
              >
                {matchCount}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
})

StrategyMapFilterBar.displayName = 'StrategyMapFilterBar'
