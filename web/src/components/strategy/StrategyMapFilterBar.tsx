import { memo } from 'react'
import { Activity, Ban, CheckCircle2, Diamond, GitBranch, Minus, Plus, Search } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { FilterPreset } from './strategyMapFilters'
import { TRACE_MIN_DEPTH, TRACE_MAX_DEPTH } from './strategyMapFilters'

interface FilterButton {
  key: FilterPreset
  label: string
  icon: typeof Activity
  color: string
  visible: boolean
}

interface Props {
  activeFilter: FilterPreset | null
  onFilterChange: (filter: FilterPreset | null) => void
  hasSelectedNode: boolean
  matchCount: number
  contextDepth: number
  onContextDepthChange: (depth: number) => void
  /** Open the node-search panel. Gives touch users (iPad, where the mobile top
   *  bar is hidden and there's no keyboard for "/") a reachable way in, and
   *  makes search discoverable for everyone. */
  onOpenSearch: () => void
}

export const StrategyMapFilterBar = memo(function StrategyMapFilterBar({
  activeFilter,
  onFilterChange,
  hasSelectedNode,
  matchCount,
  contextDepth,
  onContextDepthChange,
  onOpenSearch,
}: Props) {
  const buttons: FilterButton[] = [
    { key: 'in_motion', label: 'In Motion', icon: Activity, color: 'var(--blue)', visible: true },
    { key: 'blocked', label: 'Blocked', icon: Ban, color: 'var(--amber)', visible: true },
    { key: 'done', label: 'Done', icon: CheckCircle2, color: 'var(--green)', visible: true },
    { key: 'decisions', label: 'Decisions', icon: Diamond, color: 'var(--accent)', visible: true },
    { key: 'trace', label: 'Trace Path', icon: GitBranch, color: 'var(--accent)', visible: hasSelectedNode },
  ]

  const isTraceActive = activeFilter === 'trace'

  return (
    <div className="absolute top-4 left-4 z-10 flex items-center gap-1.5">
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
        if (!btn.visible) return null
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

      {/* Context depth control — appears when trace is active */}
      {isTraceActive && (
        <div
          className="inline-flex items-center gap-1 transition-all duration-200"
          style={{
            padding: '4px 5px',
            borderRadius: 20,
            border: '1px solid var(--border)',
            background: 'var(--bg-panel)',
            boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
          }}
        >
          <button
            type="button"
            onClick={() => onContextDepthChange(Math.max(TRACE_MIN_DEPTH, contextDepth - 1))}
            disabled={contextDepth <= TRACE_MIN_DEPTH}
            className="inline-flex items-center justify-center w-[18px] h-[18px] rounded-full transition-colors hover:bg-[var(--bg-surface)] disabled:opacity-25 focus:outline-none"
            style={{ color: 'var(--text-3)' }}
          >
            <Minus size={10} />
          </button>

          {/* Depth dots — visual indicator of context expansion */}
          <div className="flex items-center gap-[3px] px-1">
            {Array.from({ length: TRACE_MAX_DEPTH }, (_, i) => (
              <div
                key={i}
                className="rounded-full transition-all duration-200"
                style={{
                  width: 6,
                  height: 6,
                  background: i < contextDepth
                    ? 'var(--accent)'
                    : 'var(--border)',
                  opacity: i < contextDepth ? 1 : 0.5,
                }}
              />
            ))}
          </div>

          <button
            type="button"
            onClick={() => onContextDepthChange(Math.min(TRACE_MAX_DEPTH, contextDepth + 1))}
            disabled={contextDepth >= TRACE_MAX_DEPTH}
            className="inline-flex items-center justify-center w-[18px] h-[18px] rounded-full transition-colors hover:bg-[var(--bg-surface)] disabled:opacity-25 focus:outline-none"
            style={{ color: 'var(--text-3)' }}
          >
            <Plus size={10} />
          </button>
        </div>
      )}
    </div>
  )
})

StrategyMapFilterBar.displayName = 'StrategyMapFilterBar'
