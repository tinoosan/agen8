import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import type { ApproveRejectPayload, ApproveRejectResult } from '../../hooks/useHumanInput'

interface Props {
  payload: ApproveRejectPayload
  busy: boolean
  onSubmit: (result: ApproveRejectResult) => void
  onCancel: () => void
  onCancelRequest?: () => void
}

export default function ApproveRejectPanel({ payload, busy, onSubmit, onCancel, onCancelRequest }: Props) {
  const [decision, setDecision] = useState<'approve' | 'reject' | null>(null)
  const [note, setNote] = useState('')

  const canSubmit = decision !== null

  function submit() {
    if (!decision) return
    onSubmit({
      decision,
      ...(note.trim() ? { note: note.trim() } : {}),
    })
  }

  return (
    <div className="overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--bg-panel)] shadow-[var(--shadow-lg)]">
      <div className="px-5 pt-5 pb-4">
        <div className="mb-3 text-[11px] text-[var(--text-3)]" style={{ letterSpacing: '-0.04px' }}>
          Approval request
        </div>
        <p className="m-0 text-[15px] font-semibold leading-snug text-[var(--text-1)]" style={{ letterSpacing: '-0.02em' }}>
          {payload.title}
        </p>
        {payload.description && (
          <p className="m-0 mt-2 text-[13px] leading-relaxed text-[var(--text-2)]" style={{ letterSpacing: '-0.06px' }}>
            {payload.description}
          </p>
        )}
        {payload.context && (
          <p className="m-0 mt-2 text-[12px] leading-relaxed text-[var(--text-3)]" style={{ letterSpacing: '-0.06px' }}>
            {payload.context}
          </p>
        )}
      </div>

      <div className="px-3 pb-3 flex flex-col gap-0.5">
        {([
          { value: 'approve', label: 'Approve' },
          { value: 'reject', label: 'Reject' },
        ] as const).map((option) => {
          const active = decision === option.value
          return (
            <button
              key={option.value}
              type="button"
              disabled={busy}
              onClick={() => setDecision(option.value)}
              className={cn(
                'w-full flex items-center gap-3 rounded-[var(--r-md)] px-3 py-2.5 text-left transition-colors duration-100 focus:outline-none',
                active
                  ? 'bg-[color-mix(in_srgb,var(--accent)_10%,var(--bg-surface))]'
                  : 'hover:bg-[var(--bg-hover)]',
                busy && 'cursor-default opacity-60',
              )}
            >
              <span className={cn(
                'flex h-4 w-4 shrink-0 items-center justify-center rounded-full border-2 transition-colors duration-100',
                active ? 'border-[var(--accent)] bg-[var(--accent)]' : 'border-[var(--border)]',
              )}>
                {active && <span className="h-1.5 w-1.5 rounded-full bg-white" />}
              </span>
              <span className="flex-1 text-[13px] font-medium leading-snug text-[var(--text-2)]" style={{ letterSpacing: '-0.08px' }}>
                {option.label}
              </span>
            </button>
          )
        })}
      </div>

      <div className="px-4 pb-3">
        <p className="m-0 mb-1.5 text-[11px] text-[var(--text-3)]" style={{ letterSpacing: '-0.04px' }}>
          Optional note
        </p>
        <Textarea
          value={note}
          disabled={busy}
          rows={decision === 'reject' ? 4 : 2}
          onChange={(e) => setNote(e.target.value)}
          placeholder={decision === 'reject' ? 'Explain what should change…' : 'Add context if needed…'}
          className="resize-none rounded-[var(--r-md)] border-[var(--border)] bg-[var(--bg-surface)] px-3 py-2.5 text-[13px]"
        />
      </div>

      <div className="flex items-center justify-between gap-3 border-t border-[var(--border)]/50 px-4 py-3">
        <div className="flex items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onCancel}
            disabled={busy}
            className="text-[var(--text-3)] hover:text-[var(--text-2)]"
          >
            Dismiss
          </Button>
          {onCancelRequest && (
            <Button type="button" variant="ghost" size="sm" onClick={onCancelRequest} disabled={busy}
              className="text-[var(--red)]/60 hover:text-[var(--red)]">
              Cancel request
            </Button>
          )}
        </div>
        <Button type="button" size="sm" onClick={submit} disabled={busy || !canSubmit}>
          Submit
        </Button>
      </div>
    </div>
  )
}
