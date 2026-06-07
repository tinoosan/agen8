import { formatKRProgress } from '../../lib/missionUtils'
import ProgressHistory from './ProgressHistory'
import { measurementLabel, directionLabel, shortAgent } from './krFields'
import type { KeyResultView } from '../../lib/types'

/* ── Read-only KR detail (shown when a row is expanded and not editing) ── */

export function KRDetailView({ kr }: { kr: KeyResultView }) {
  return (
    <div className="flex flex-col gap-2.5">
      {kr.description && (
        <p className="text-[var(--text-2)] m-0" style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px', lineHeight: 1.5 }}>
          {kr.description}
        </p>
      )}

      {/* Metadata chips */}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
        {formatKRProgress(kr) && (
          <span className="text-[0.6875rem] text-[var(--text-3)]">
            <span className="opacity-60">Progress</span>{' '}{formatKRProgress(kr)}
          </span>
        )}
        <span className="text-[0.6875rem] text-[var(--text-3)]">
          <span className="opacity-60">Type</span>{' '}{measurementLabel(kr.measurementType)}
        </span>
        <span className="text-[0.6875rem] text-[var(--text-3)]">
          <span className="opacity-60">Direction</span>{' '}{directionLabel(kr.direction)}
        </span>
        {kr.unit && (
          <span className="text-[0.6875rem] text-[var(--text-3)]">
            <span className="opacity-60">Unit</span>{' '}{kr.unit}
          </span>
        )}
        {kr.baseline != null && (
          <span className="text-[0.6875rem] text-[var(--text-3)]">
            <span className="opacity-60">Baseline</span>{' '}{kr.baseline}
          </span>
        )}
        {kr.lastUpdatedBy && (
          <span className="text-[0.6875rem] text-[var(--text-3)]">
            <span className="opacity-60">Updated by</span>{' '}{shortAgent(kr.lastUpdatedBy)}
          </span>
        )}
      </div>

      {/* Progress history */}
      <ProgressHistory keyResultId={kr.id} />
    </div>
  )
}
