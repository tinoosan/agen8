import React, { useState, useMemo } from 'react'
import { createPortal } from 'react-dom'
import { Maximize2, X } from 'lucide-react'
import { parseDiff, type DiffLine } from './activityHelpers'

interface DiffBlockProps {
  unified: string
  truncated?: boolean
  lang?: string
}

export function DiffBlock({ unified, truncated }: DiffBlockProps) {
  const [overlayOpen, setOverlayOpen] = useState(false)
  const { lines, added, deleted } = useMemo(() => parseDiff(unified), [unified])

  function lineStyle(type: DiffLine['type']): React.CSSProperties {
    const base: React.CSSProperties = {
      display: 'block',
      paddingLeft: 10,
      paddingRight: 14,
      whiteSpace: 'pre',
      fontFamily: 'var(--font-mono, ui-monospace, monospace)',
      fontSize: '0.6875rem',
      lineHeight: '1.65',
      borderLeft: '2px solid transparent',
    }
    if (type === 'add') return { ...base, background: 'var(--diff-add-bg)', borderLeft: '2px solid var(--diff-add-border)' }
    if (type === 'del') return { ...base, background: 'var(--diff-del-bg)', borderLeft: '2px solid var(--diff-del-border)' }
    if (type === 'hunk') return { ...base, background: 'var(--diff-hunk-bg)', borderLeft: '2px solid var(--diff-hunk-border)', color: 'var(--accent)' }
    if (type === 'meta') return { ...base, opacity: 0.45, fontSize: '0.625rem'}
    return base
  }

  // Compute old/new line numbers for each line by tracking hunk headers
  const lineNumbers = useMemo(() => {
    let oldN = 0, newN = 0
    return lines.map(line => {
      if (line.type === 'hunk') {
        // Parse @@ -oldStart[,oldCount] +newStart[,newCount] @@
        const m = line.text.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/)
        if (m) { oldN = parseInt(m[1], 10); newN = parseInt(m[2], 10) }
        return { old: null, new: null }
      }
      if (line.type === 'meta') return { old: null, new: null }
      if (line.type === 'del') { const n = oldN++; return { old: n, new: null } }
      if (line.type === 'add') { const n = newN++; return { old: null, new: n } }
      // ctx
      const o = oldN++, n = newN++
      return { old: o, new: n }
    })
  }, [lines])

  if (lines.length === 0) return null

  const numColClass = 'inline-block w-9 min-w-[36px] text-right pr-2.5 text-[var(--text-3)] tabular-nums select-none opacity-60 text-[0.625rem]'

  function renderLines(maxHeight?: number) {
    return (
      <div className="overflow-y-auto overflow-x-auto bg-[var(--bg-app)]" style={maxHeight ? { maxHeight } : undefined}>
        {/* Inner div sized to the widest line so each row's 100% minWidth fills it */}
        <div className="min-w-full w-max">
        {lines.map((line, i) => {
          const nums = lineNumbers[i]
          const s = lineStyle(line.type)
          const rowStyle: React.CSSProperties = { ...s, paddingLeft: 0, display: 'flex', minWidth: '100%', boxSizing: 'border-box' }
          const gutter = (
            <span className="inline-flex shrink-0 border-r border-[var(--border)] mr-2.5">
              <span className={numColClass}>{nums.old ?? ''}</span>
              <span className={numColClass}>{nums.new ?? ''}</span>
            </span>
          )
          return (
            <div key={i} style={rowStyle}>
              {gutter}
              <span className="whitespace-pre flex-1">{line.text}</span>
            </div>
          )
        })}
        </div>
        {truncated && (
          <div className="px-2.5 py-[3px] text-[var(--text-3)] italic text-[0.625rem] border-t border-[var(--border)]">
            ... preview truncated
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="break-normal whitespace-normal">
      {/* Diff lines — always visible */}
      <div className="rounded-[var(--r-md)] border border-[var(--border)] overflow-hidden relative">
        <button
          onClick={(e) => { e.stopPropagation(); setOverlayOpen(true) }}
          title="Expand diff"
          aria-label="Expand diff"
          className="absolute top-1.5 right-1.5 z-10 bg-[var(--bg-elevated)] border border-[var(--border)] rounded cursor-pointer text-[var(--text-3)] p-1 flex items-center opacity-60 hover:opacity-100 transition-opacity"
        >
          <Maximize2 size={10} />
        </button>
        {renderLines(360)}
      </div>

      {/* Full-screen overlay -- portalled to document.body to escape any stacking context */}
      {overlayOpen && createPortal(
        <div
          className="fixed inset-0 z-[9999] bg-[var(--overlay-bg)] flex items-center justify-center p-6"
          onClick={() => setOverlayOpen(false)}
        >
          <div
            className="bg-[var(--bg-panel)] rounded-[var(--r-lg)] border border-[var(--border)] w-[90vw] max-w-[960px] h-[85vh] flex flex-col overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center gap-2 px-3.5 py-2.5 border-b border-[var(--border)] shrink-0">
              {added > 0 && <span className="text-[var(--green)] text-xs font-bold">+{added}</span>}
              {deleted > 0 && <span className="text-[var(--red)] text-xs font-bold">{'\u2212'}{deleted}</span>}
              <div className="flex-1" />
              <button
                onClick={() => setOverlayOpen(false)}
                aria-label="Close diff viewer"
                className="bg-transparent border-none cursor-pointer text-[var(--text-3)] flex p-1"
              >
                <X size={16} />
              </button>
            </div>
            <div className="flex-1 overflow-auto">
              {renderLines()}
            </div>
          </div>
        </div>,
        document.body
      )}
    </div>
  )
}
