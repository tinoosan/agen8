import { useProgressHistory } from '../../hooks/useMissions'
import type { ProgressEntryView } from '../../hooks/useMissions'
import { Skeleton } from '@/components/ui/skeleton'
import { formatRelative } from '@/lib/format'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useState } from 'react'

/* ── Helpers ──────────────────────────────────────────── */

function shortAgent(raw: string): string {
  const slashPart = raw.split('/').pop() ?? raw
  const label = slashPart.split(':').pop() ?? slashPart
  return label.replace(/[_-]/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}

/* ── Single entry row ─────────────────────────────────── */

function EntryRow({ entry }: { entry: ProgressEntryView }) {
  return (
    <div className="py-1.5 min-w-0">
      {/* Header line: value → % · agent · time */}
      <div className="flex items-center gap-1.5">
        <span className="shrink-0 w-1 h-1 rounded-full bg-[var(--accent)]" />
        <span className="tabular-nums text-[var(--text-1)] shrink-0" style={{ fontSize: '0.6875rem', fontWeight: 500 }}>
          {entry.value} <span className="text-[var(--text-3)] font-normal">→ {entry.progress}%</span>
        </span>
        <span className="text-[var(--text-3)] shrink-0 ml-auto" style={{ fontSize: '0.625rem' }}>
          {shortAgent(entry.updatedBy)}
        </span>
        <span className="text-[var(--text-3)] shrink-0 tabular-nums" style={{ fontSize: '0.625rem' }}>
          {formatRelative(entry.createdAt, { seconds: true })}
        </span>
      </div>
      {/* Note — wraps freely below */}
      {entry.note && (
        <p className="text-[var(--text-2)] m-0 mt-0.5 pl-2.5" style={{ fontSize: '0.6875rem', lineHeight: 1.5 }}>
          {entry.note}
        </p>
      )}
    </div>
  )
}

/* ── Main component ───────────────────────────────────── */

export default function ProgressHistory({ keyResultId }: { keyResultId: string }) {
  const [expanded, setExpanded] = useState(false)
  const { data: entries, isLoading } = useProgressHistory(expanded ? keyResultId : null)

  return (
    <div className="mt-1.5 pl-5">
      <button
        onClick={() => setExpanded(e => !e)}
        className="inline-flex items-center gap-1 text-[var(--text-3)] hover:text-[var(--text-2)] transition-colors bg-transparent border-none cursor-pointer p-0"
        style={{ fontSize: '0.6875rem', fontWeight: 500, letterSpacing: '-0.06px' }}
      >
        {expanded ? <ChevronDown size={10} /> : <ChevronRight size={10} />}
        Progress History
      </button>

      {expanded && (
        <div className="mt-1 pl-2 border-l border-[var(--border)]">
          {isLoading && (
            <div className="space-y-1 py-0.5">
              {[1, 2].map(i => <Skeleton key={i} className="h-3 w-full" />)}
            </div>
          )}
          {!isLoading && entries && entries.length === 0 && (
            <p className="text-[0.625rem] text-[var(--text-3)] py-1">No progress updates yet</p>
          )}
          {!isLoading && entries && entries.length > 0 && (
            <div className="overflow-y-auto" style={{ maxHeight: '220px' }}>
              {entries.map(entry => <EntryRow key={entry.id} entry={entry} />)}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
