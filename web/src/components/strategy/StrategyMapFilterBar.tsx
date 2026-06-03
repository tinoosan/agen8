import { memo } from 'react'
import { AlertCircle, XCircle, GitBranch, Minus, Plus } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { FilterPreset } from './strategyMapFilters'

interface FilterButton {
  key: FilterPreset
  label: string
  icon: typeof AlertCircle
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
}

const MAX_CONTEXT_DEPTH = 3

export const StrategyMapFilterBar = memo(function StrategyMapFilterBar({
  activeFilter,
  onFilterChange,
  hasSelectedNode,
  matchCount,
  contextDepth,
  onContextDepthChange,
}: Props) {
  const buttons: FilterButton[] = [
    { key: 'attention', label: 'Attention', icon: AlertCircle, color: 'var(--amber)', visible: true },
    { key: 'failed', label: 'Failed', icon: XCircle, color: 'var(--red)', visible: true },
    { key: 'trace', label: 'Trace Path', icon: GitBranch, color: 'var(--accent)', visible: hasSelectedNode },
  ]

  const isTraceActive = activeFilter === 'trace'

  return (
    <div className="absolute top-4 left-4 z-10 flex items-center gap-1.5">
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
              'inline-flex items-center gap-1.5 transition-all duration-200',
              'focus:outline-none focus-visible:outline-none',
              'hover:scale-[1.03]',
            )}
            style={{
              padding: '5px 10px',
              borderRadius: 20,
              fontSize: '11px',
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
                  fontSize: '10px',
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
            onClick={() => onContextDepthChange(Math.max(0, contextDepth - 1))}
            disabled={contextDepth === 0}
            className="inline-flex items-center justify-center w-[18px] h-[18px] rounded-full transition-colors hover:bg-[var(--bg-surface)] disabled:opacity-25 focus:outline-none"
            style={{ color: 'var(--text-3)' }}
          >
            <Minus size={10} />
          </button>

          {/* Depth dots — visual indicator of context expansion */}
          <div className="flex items-center gap-[3px] px-1">
            {Array.from({ length: MAX_CONTEXT_DEPTH }, (_, i) => (
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
            onClick={() => onContextDepthChange(Math.min(MAX_CONTEXT_DEPTH, contextDepth + 1))}
            disabled={contextDepth >= MAX_CONTEXT_DEPTH}
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
