import { X } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from '@/components/ui/dialog'

interface StrategyMapHelpProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface Shortcut {
  keys: string[]
  desc: string
  group: 'Navigation' | 'View' | 'Selection' | 'Search' | 'Filters' | 'Help'
}

const SHORTCUTS: Shortcut[] = [
  // Navigation
  { keys: ['↑', '↓', '←', '→'], desc: 'Move focus to the nearest node in that direction (within current cluster)', group: 'Navigation' },
  { keys: ['Tab'], desc: 'Cycle forward to the next mission cluster', group: 'Navigation' },
  { keys: ['Shift', 'Tab'], desc: 'Cycle backward to the previous mission cluster', group: 'Navigation' },

  // View
  { keys: ['F'], desc: 'Fit current cluster into viewport', group: 'View' },
  { keys: ['Shift', 'F'], desc: 'Fit entire map into viewport', group: 'View' },
  { keys: ['+'], desc: 'Zoom in', group: 'View' },
  { keys: ['−'], desc: 'Zoom out', group: 'View' },

  // Selection
  { keys: ['Enter'], desc: 'Open detail panel for the focused node', group: 'Selection' },
  { keys: ['Space'], desc: 'Open detail panel (alternative to Enter)', group: 'Selection' },
  { keys: ['Esc'], desc: 'Close detail panel and clear selection', group: 'Selection' },

  // Filters
  { keys: ['A'], desc: 'Toggle Attention filter (needs human action)', group: 'Filters' },
  { keys: ['X'], desc: 'Toggle Failed filter (dead/cancelled work)', group: 'Filters' },
  { keys: ['T'], desc: 'Toggle Trace Path from selected node', group: 'Filters' },
  { keys: ['['], desc: 'Decrease context depth (trace mode)', group: 'Filters' },
  { keys: [']'], desc: 'Increase context depth (trace mode)', group: 'Filters' },

  // Search
  { keys: ['/'], desc: 'Search for a node by name', group: 'Search' },

  // Help
  { keys: ['?'], desc: 'Show this help overlay', group: 'Help' },
]

const GROUP_ORDER: Shortcut['group'][] = [
  'Navigation',
  'View',
  'Selection',
  'Filters',
  'Search',
  'Help',
]

/**
 * Strategy-map keyboard shortcut help overlay, rendered as a modal.
 * Two-column list grouped by workflow. Opened via `?` and dismissed via Escape,
 * a custom close icon, or click-outside. Styled per designs/apple/DESIGN.md —
 * 12px radius, the single diffused Apple shadow, SF Pro–tuned typography, and
 * Caption-grade descriptions with SF Mono key caps.
 */
export function StrategyMapHelp({ open, onOpenChange }: StrategyMapHelpProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        // `[&>button]:hidden` hides shadcn's default absolute close X — we
        // render our own below to avoid the thick focus ring and to match the
        // custom chrome used by StrategyMapSearch.
        className="sm:max-w-[580px] p-0 overflow-hidden gap-0 border-0 [&>button]:hidden"
        // Prevent Radix from auto-focusing the first focusable element (our
        // close X) on open — keyboard opens via `?` otherwise leave it with
        // a `:focus-visible` ring, which the user finds distracting. Focus
        // trapping still works; only the initial autofocus is skipped.
        onOpenAutoFocus={(e) => e.preventDefault()}
        style={{
          borderRadius: '12px',
          // Apple DESIGN.md §6 — the single canonical soft diffused shadow.
          boxShadow: 'rgba(0, 0, 0, 0.22) 3px 5px 30px 0px',
          background: 'var(--bg-panel)',
        }}
      >
        {/* Header: title on the left, custom close X on the right. */}
        <div
          className="relative flex items-center justify-between"
          style={{ padding: '20px 24px 16px 24px' }}
        >
          <DialogTitle
            // Override shadcn's default text-lg + tracking-tight classes so we
            // can hit Apple's spec precisely. The `!` prefixes beat shadcn's
            // base utilities on specificity.
            className="!text-[20px] !font-semibold !tracking-normal !leading-none"
            style={{
              fontSize: '20px',
              fontWeight: 600,
              lineHeight: 1.14,
              letterSpacing: '-0.2px',
              color: 'var(--text-1)',
              margin: 0,
            }}
          >
            Keyboard shortcuts
          </DialogTitle>
          <button
            type="button"
            onClick={() => onOpenChange(false)}
            aria-label="Close"
            // Stack every known focus-indicator suppression: outline, ring,
            // and box-shadow at both :focus and :focus-visible. Some browsers
            // render focus indicators via box-shadow on buttons with a
            // border-radius, so `outline: none` alone isn't enough.
            className={
              'inline-flex items-center justify-center rounded-full transition-colors duration-150 ' +
              'hover:bg-[var(--bg-surface)] ' +
              'outline-none focus:outline-none focus-visible:outline-none ' +
              'focus:ring-0 focus-visible:ring-0 ' +
              'focus:shadow-none focus-visible:shadow-none'
            }
            style={{
              width: '28px',
              height: '28px',
              color: 'var(--text-3)',
              background: 'transparent',
              outline: 'none',
              border: 'none',
              boxShadow: 'none',
            }}
          >
            <X size={16} strokeWidth={1.75} />
          </button>
        </div>

        {/* Shortcut groups. Scrollable body for narrow viewports. */}
        <div
          className="flex flex-col overflow-y-auto"
          style={{
            padding: '4px 24px 24px 24px',
            gap: '24px',
            maxHeight: '72vh',
          }}
        >
          {GROUP_ORDER.map((group) => {
            const items = SHORTCUTS.filter((s) => s.group === group)
            if (items.length === 0) return null
            return (
              <div key={group} className="flex flex-col" style={{ gap: '10px' }}>
                <h3
                  className="uppercase"
                  style={{
                    // Apple-style eyebrow — 11px semibold with wide tracking.
                    fontSize: '11px',
                    fontWeight: 600,
                    lineHeight: 1,
                    letterSpacing: '0.08em',
                    color: 'var(--text-3)',
                    margin: 0,
                  }}
                >
                  {group}
                </h3>
                <div className="flex flex-col" style={{ gap: '8px' }}>
                  {items.map((shortcut, i) => (
                    <div
                      key={i}
                      className="flex items-center justify-between"
                      style={{ gap: '24px', minHeight: '28px' }}
                    >
                      <span
                        style={{
                          // Apple Caption — 14px, weight 400, tracking -0.224px.
                          fontSize: '14px',
                          fontWeight: 400,
                          lineHeight: 1.43,
                          letterSpacing: '-0.224px',
                          color: 'var(--text-2)',
                        }}
                      >
                        {shortcut.desc}
                      </span>
                      <div
                        className="flex shrink-0 items-center"
                        style={{ gap: '4px' }}
                      >
                        {shortcut.keys.map((k, ki) => (
                          <kbd
                            key={ki}
                            className="inline-flex items-center justify-center shrink-0"
                            style={{
                              // Matches the Esc kbd in StrategyMapSearch for
                              // visual consistency across both modals.
                              minWidth: '24px',
                              height: '22px',
                              padding: '0 7px',
                              borderRadius: '5px',
                              border: '1px solid var(--border)',
                              background: 'var(--bg-surface)',
                              color: 'var(--text-2)',
                              fontFamily: 'SF Mono, Menlo, monospace',
                              fontSize: '11px',
                              fontWeight: 500,
                              lineHeight: 1,
                              letterSpacing: '-0.12px',
                            }}
                          >
                            {k}
                          </kbd>
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )
          })}
        </div>
      </DialogContent>
    </Dialog>
  )
}
