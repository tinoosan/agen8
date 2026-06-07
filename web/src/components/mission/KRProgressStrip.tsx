import { useState } from 'react'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { useUpdateKRProgress } from '../../hooks/useMissions'
import { ghostInput } from './krFields'
import type { KeyResultView } from '../../lib/types'

/* ── Inline "report progress" strip (shown in an expanded KR row) ──
   Owns its own value/note state and the progress mutation. Unmounting
   when the row leaves report mode discards the draft, which is exactly
   the old behavior (cancel cleared the fields). */

export function KRProgressStrip({
  kr,
  missionId,
  onDone,
}: {
  kr: KeyResultView
  missionId: string
  onDone: () => void
}) {
  const [progressValue, setProgressValue] = useState('')
  const [progressNote, setProgressNote] = useState('')
  const updateProgress = useUpdateKRProgress()

  function cancel() {
    setProgressValue('')
    setProgressNote('')
    onDone()
  }

  async function handleReportProgress() {
    const trimmedNote = progressNote.trim()
    if (!trimmedNote) { toast.error('Note is required — explain what you measured'); return }
    const parsedValue = parseFloat(progressValue)
    if (isNaN(parsedValue)) { toast.error('Value must be a valid number'); return }

    try {
      await updateProgress.mutateAsync({ keyResultId: kr.id, missionId, value: parsedValue, note: trimmedNote })
      toast.success('Progress recorded')
      cancel()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to record progress')
    }
  }

  return (
    <div className="flex flex-col gap-2 pt-1 pb-1">
      <div className="flex items-center gap-3">
        <input
          className={cn(ghostInput, 'w-24 tabular-nums text-[var(--text-1)] border-b border-[var(--border)]')}
          style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}
          type="number"
          placeholder={`${kr.currentValue ?? 0}`}
          value={progressValue}
          onChange={(e) => setProgressValue(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Escape') cancel() }}
          autoFocus
        />
        <span className="text-[0.6875rem] text-[var(--text-3)]">→ target {kr.targetValue}{kr.unit ? ` ${kr.unit}` : ''}</span>
      </div>
      <input
        className={cn(ghostInput, 'text-[var(--text-3)] border-b border-[var(--border)]')}
        style={{ fontSize: '0.75rem', letterSpacing: '-0.12px' }}
        placeholder="What did you measure?"
        value={progressNote}
        onChange={(e) => setProgressNote(e.target.value)}
        onKeyDown={(e) => { if (e.key === 'Escape') cancel() }}
      />
      <div className="flex items-center gap-3 pt-0.5">
        <button
          className="text-[0.75rem] text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors bg-transparent border-none cursor-pointer p-0"
          style={{ letterSpacing: '-0.12px' }}
          onClick={cancel}
        >
          Cancel
        </button>
        <button
          className="text-[0.75rem] text-[var(--accent)] font-medium hover:opacity-80 transition-opacity bg-transparent border-none cursor-pointer p-0 disabled:opacity-40"
          style={{ letterSpacing: '-0.12px' }}
          onClick={handleReportProgress}
          disabled={updateProgress.isPending || !progressValue.trim() || !progressNote.trim()}
        >
          {updateProgress.isPending ? 'Recording…' : 'Record'}
        </button>
      </div>
    </div>
  )
}
