/**
 * Sub-components shared by the ActionDetail full page.
 * OpActionDetailPanel (dashboard modal) keeps its own parallel copy
 * intentionally — the two surfaces may diverge over time.
 *
 * Pure helpers (statusConfig, timeAgo) live in ./opActionUtils.ts
 * to satisfy the react-refresh/only-export-components rule.
 */
import { useState } from 'react'
import { timeAgo } from './opActionUtils'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Play, CheckCircle2, Ban, Lock, Unlock, Plus, Trash2, CornerDownLeft } from 'lucide-react'
import { cn } from '@/lib/utils'
import { toast } from 'sonner'
import {
  useStartOpAction,
  useCompleteOpAction,
  useVerifyOpAction,
  useBlockOpAction,
  useUnblockOpAction,
  useCancelOpAction,
  useAddOpActionNote,
} from '../../hooks/useOpActions'
import type { OpActionStatus, OpActionOutcomeStatus, OpActionNote } from '../../lib/types'

/* ── Metadata item ───────────────────────────────────── */

export function MetaItem({ label, value, icon }: { label: string; value: string; icon?: React.ReactNode }) {
  return (
    <div>
      <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-0.5">
        {label}
      </div>
      <div className="flex items-center gap-1 text-xs text-[var(--text-2)]">
        {icon && <span className="text-[var(--text-3)]">{icon}</span>}
        <span className="truncate">{value}</span>
      </div>
    </div>
  )
}

/* ── Lifecycle timeline ──────────────────────────────── */

export function LifecycleTimeline({ status, requiresVerification }: { status: OpActionStatus; requiresVerification: boolean }) {
  const steps = requiresVerification
    ? [{ key: 'pending', label: 'Created' }, { key: 'in_progress', label: 'Working' }, { key: 'pending_verification', label: 'Verify' }, { key: 'completed', label: 'Done' }]
    : [{ key: 'pending', label: 'Created' }, { key: 'in_progress', label: 'Working' }, { key: 'completed', label: 'Done' }]

  const statusOrder: Record<string, number> = { pending: 0, acknowledged: 0, in_progress: 1, pending_verification: 2, completed: requiresVerification ? 3 : 2 }
  const currentStep = statusOrder[status] ?? -1
  const isTerminal = status === 'completed' || status === 'canceled'
  const isBlocked = status === 'blocked'

  return (
    <div className="flex items-center gap-1 py-3">
      {steps.map((step, i) => {
        const isPast = i < currentStep
        const isCurrent = i === currentStep && !isTerminal
        const isDone = isTerminal && i <= currentStep

        return (
          <div key={step.key} className="flex items-center gap-1 flex-1">
            <div className="flex flex-col items-center gap-1 flex-1">
              <div
                className={cn(
                  'w-3 h-3 rounded-full transition-all duration-300',
                  isDone ? 'bg-[var(--green)]'
                    : isCurrent ? 'bg-[var(--accent)] ring-2 ring-[var(--accent)]/30'
                    : isPast ? 'bg-[var(--green)]'
                    : 'bg-[var(--border-strong)]',
                )}
              />
              <span className={cn(
                'text-[10px] font-medium',
                (isPast || isDone) ? 'text-[var(--text-2)]' : isCurrent ? 'text-[var(--accent)]' : 'text-[var(--text-3)]',
              )}>
                {step.label}
              </span>
            </div>
            {i < steps.length - 1 && (
              <div className={cn(
                'h-px flex-1 -mt-4',
                isPast || isDone ? 'bg-[var(--green)]' : 'bg-[var(--border)]',
              )} />
            )}
          </div>
        )
      })}
      {isBlocked && (
        <div className="flex flex-col items-center gap-1 ml-2">
          <div className="w-3 h-3 rounded-full bg-[var(--red)] ring-2 ring-[var(--red)]/30" />
          <span className="text-[10px] font-medium text-[var(--red)]">Blocked</span>
        </div>
      )}
    </div>
  )
}

/* ── Progress notes ──────────────────────────────────── */

export function NotesSection({ notes, actionId, inputOnly }: { notes: OpActionNote[]; actionId: string; inputOnly?: boolean }) {
  const [text, setText] = useState('')
  const addNote = useAddOpActionNote()

  const handleAdd = () => {
    if (!text.trim()) return
    addNote.mutate(
      { actionId, text: text.trim() },
      { onSuccess: () => { setText(''); toast.success('Note added') } },
    )
  }

  return (
    <div>
      {!inputOnly && (
        notes.length > 0 ? (
          <div className="space-y-1.5 mb-3">
            {notes.map((note, i) => (
              <div key={i} className="flex gap-2 text-xs">
                <span className="text-[var(--text-3)] tabular-nums shrink-0 text-[10px] mt-px">{timeAgo(note.createdAt)}</span>
                <span className="text-[var(--text-2)]">{note.text}</span>
              </div>
            ))}
          </div>
        ) : (
          <div className="text-xs text-[var(--text-3)] mb-3">No notes yet</div>
        )
      )}
      <div className="note-composer rounded-[var(--r-md)] border border-[var(--border)] focus-within:border-[var(--border-strong)] transition-colors">
        <Textarea
          value={text}
          onChange={e => setText(e.target.value)}
          placeholder="Leave a note..."
          className="min-h-[72px] text-sm resize-none border-0 bg-transparent shadow-none focus-visible:ring-0 focus-visible:ring-offset-0 pb-1"
          onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleAdd() } }}
        />
        <div className="flex items-center justify-between px-3 pb-2.5">
          <span className="text-[10px] text-[var(--text-3)] flex items-center gap-1">
            <CornerDownLeft size={10} className="opacity-60" /> to post
          </span>
          <Button
            size="xs"
            onClick={handleAdd}
            disabled={!text.trim() || addNote.isPending}
            className="text-xs h-6 px-2.5 bg-[var(--accent)] text-white hover:bg-[var(--accent)]/90"
          >
            {addNote.isPending ? 'Posting...' : 'Post note'}
          </Button>
        </div>
      </div>
    </div>
  )
}

/* ── Outcome form ────────────────────────────────────── */

export function OutcomeForm({ actionId, onCancel }: { actionId: string; onCancel: () => void }) {
  const [outcomeStatus, setOutcomeStatus] = useState<OpActionOutcomeStatus>('completed')
  const [summary, setSummary] = useState('')
  const [pairs, setPairs] = useState<{ key: string; value: string }[]>([])
  const complete = useCompleteOpAction()

  const addPair = () => setPairs(p => [...p, { key: '', value: '' }])
  const removePair = (i: number) => setPairs(p => p.filter((_, idx) => idx !== i))
  const updatePair = (i: number, field: 'key' | 'value', val: string) => {
    setPairs(p => p.map((pair, idx) => idx === i ? { ...pair, [field]: val } : pair))
  }

  const handleSubmit = () => {
    if (!summary.trim()) return
    const outcomePairs: Record<string, string> = {}
    for (const p of pairs) {
      if (p.key.trim() && p.value.trim()) outcomePairs[p.key.trim()] = p.value.trim()
    }
    complete.mutate(
      { actionId, outcomeStatus, outcomeSummary: summary.trim(), outcomePairs: Object.keys(outcomePairs).length > 0 ? outcomePairs : undefined },
      { onSuccess: () => { toast.success('Action completed'); onCancel() } },
    )
  }

  return (
    <div className="bg-[var(--bg-surface)] rounded-[var(--r-md)] p-4 space-y-3 animate-fade-in">
      <div className="text-xs font-semibold text-[var(--text-1)]">Complete Action</div>

      <div>
        <label className="label-sm mb-1 block">Outcome</label>
        <Select value={outcomeStatus} onValueChange={v => setOutcomeStatus(v as OpActionOutcomeStatus)}>
          <SelectTrigger className="text-xs h-8">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="completed" className="text-xs">Completed</SelectItem>
            <SelectItem value="partial" className="text-xs">Partial</SelectItem>
            <SelectItem value="failed" className="text-xs">Failed</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div>
        <label className="label-sm mb-1 block">Summary *</label>
        <Textarea
          value={summary}
          onChange={e => setSummary(e.target.value)}
          placeholder="What was accomplished..."
          className="min-h-[60px] text-xs"
        />
      </div>

      <div>
        <div className="flex items-center justify-between mb-1">
          <label className="label-sm">Data</label>
          <Button variant="ghost" size="xs" onClick={addPair} className="text-[10px] text-[var(--accent)]">
            <Plus size={10} className="mr-0.5" /> Add field
          </Button>
        </div>
        {pairs.length > 0 && (
          <div className="space-y-1.5">
            {pairs.map((pair, i) => (
              <div key={i} className="flex items-center gap-1.5">
                <Input
                  value={pair.key}
                  onChange={e => updatePair(i, 'key', e.target.value)}
                  placeholder="Key"
                  className="text-xs h-7 flex-1"
                />
                <span className="text-[var(--text-3)] text-xs">:</span>
                <Input
                  value={pair.value}
                  onChange={e => updatePair(i, 'value', e.target.value)}
                  placeholder="Value"
                  className="text-xs h-7 flex-1"
                />
                <Button variant="ghost" size="xs" onClick={() => removePair(i)} className="text-[var(--text-3)] hover:text-[var(--red)] shrink-0">
                  <Trash2 size={10} />
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="flex justify-end gap-2 pt-1">
        <Button variant="outline" size="sm" onClick={onCancel} className="text-xs">Cancel</Button>
        <Button
          size="sm"
          onClick={handleSubmit}
          disabled={!summary.trim() || complete.isPending}
          className="text-xs bg-[var(--green)] text-white hover:bg-[var(--green)]/90"
        >
          {complete.isPending ? 'Completing...' : 'Complete'}
        </Button>
      </div>
    </div>
  )
}

/* ── Lifecycle action buttons ────────────────────────── */

export function LifecycleActions({ actionId, status, onShowOutcomeForm }: { actionId: string; status: OpActionStatus; onShowOutcomeForm: () => void }) {
  const start = useStartOpAction()
  const block = useBlockOpAction()
  const unblock = useUnblockOpAction()
  const cancel = useCancelOpAction()
  const verify = useVerifyOpAction()
  const [blockReason, setBlockReason] = useState('')
  const [showBlockInput, setShowBlockInput] = useState(false)
  const [feedback, setFeedback] = useState('')
  const [showRejectInput, setShowRejectInput] = useState(false)

  if (status === 'completed' || status === 'canceled') return null

  return (
    <div className="space-y-2">
      {(status === 'pending' || status === 'acknowledged') && (
        <div className="flex gap-2">
          <Button
            size="sm"
            onClick={() => start.mutate({ actionId }, { onSuccess: () => toast.success('Action started') })}
            disabled={start.isPending}
            className="text-xs bg-[var(--accent)] text-white hover:bg-[var(--accent)]/90"
          >
            <Play size={12} className="mr-1" />
            {start.isPending ? 'Starting...' : 'Start Working'}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => cancel.mutate({ actionId }, { onSuccess: () => toast.success('Action canceled') })}
            disabled={cancel.isPending}
            className="text-xs text-[var(--text-3)]"
          >
            Cancel
          </Button>
        </div>
      )}

      {status === 'in_progress' && (
        <>
          <div className="flex gap-2">
            <Button
              size="sm"
              onClick={onShowOutcomeForm}
              className="text-xs bg-[var(--green)] text-white hover:bg-[var(--green)]/90"
            >
              <CheckCircle2 size={12} className="mr-1" /> Complete
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowBlockInput(b => !b)}
              className="text-xs"
            >
              <Lock size={11} className="mr-1" /> Block
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => cancel.mutate({ actionId }, { onSuccess: () => toast.success('Action canceled') })}
              disabled={cancel.isPending}
              className="text-xs text-[var(--text-3)]"
            >
              <Ban size={11} />
            </Button>
          </div>
          {showBlockInput && (
            <div className="rounded-[var(--r-md)] border border-[var(--border)] p-3 space-y-2.5 animate-fade-in">
              <div className="text-xs font-medium text-[var(--text-2)]">What&apos;s blocking this?</div>
              <div className="note-composer rounded-[var(--r-md)] border border-[var(--border)] focus-within:border-[var(--border-strong)] transition-colors">
                <Textarea
                  autoFocus
                  value={blockReason}
                  onChange={e => setBlockReason(e.target.value)}
                  placeholder="Describe what's blocking this action..."
                  className="min-h-[60px] text-sm resize-none border-0 bg-transparent shadow-none focus-visible:ring-0 focus-visible:ring-offset-0 pb-1"
                  onKeyDown={e => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                      e.preventDefault()
                      if (blockReason.trim()) block.mutate({ actionId, reason: blockReason }, { onSuccess: () => { toast.success('Blocked'); setBlockReason(''); setShowBlockInput(false) } })
                    }
                    if (e.key === 'Escape') { setShowBlockInput(false); setBlockReason('') }
                  }}
                />
              </div>
              <div className="flex justify-end gap-2">
                <Button
                  variant="ghost"
                  size="xs"
                  onClick={() => { setShowBlockInput(false); setBlockReason('') }}
                  className="text-xs text-[var(--text-3)]"
                >
                  Cancel
                </Button>
                <Button
                  size="xs"
                  onClick={() => block.mutate({ actionId, reason: blockReason }, { onSuccess: () => { toast.success('Blocked'); setBlockReason(''); setShowBlockInput(false) } })}
                  disabled={!blockReason.trim() || block.isPending}
                  className="text-xs bg-[var(--amber)] text-white hover:bg-[var(--amber)]/90"
                >
                  {block.isPending ? 'Blocking...' : 'Confirm block'}
                </Button>
              </div>
            </div>
          )}
        </>
      )}

      {status === 'blocked' && (
        <div className="flex gap-2">
          <Button
            size="sm"
            onClick={() => unblock.mutate({ actionId }, { onSuccess: () => toast.success('Unblocked') })}
            disabled={unblock.isPending}
            className="text-xs"
          >
            <Unlock size={12} className="mr-1" /> Unblock
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => cancel.mutate({ actionId }, { onSuccess: () => toast.success('Canceled') })}
            disabled={cancel.isPending}
            className="text-xs text-[var(--text-3)]"
          >
            Cancel
          </Button>
        </div>
      )}

      {status === 'pending_verification' && (
        <>
          <div className="flex gap-2">
            <Button
              size="sm"
              onClick={() => verify.mutate({ actionId, accepted: true }, { onSuccess: () => toast.success('Verified and completed') })}
              disabled={verify.isPending}
              className="text-xs bg-[var(--green)] text-white hover:bg-[var(--green)]/90"
            >
              <CheckCircle2 size={12} className="mr-1" /> Accept
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowRejectInput(true)}
              className="text-xs"
            >
              Request Changes
            </Button>
          </div>
          {showRejectInput && (
            <div className="flex gap-2 animate-fade-in">
              <Textarea
                value={feedback}
                onChange={e => setFeedback(e.target.value)}
                placeholder="What needs to change..."
                className="text-xs min-h-[36px] flex-1"
              />
              <Button
                size="xs"
                className="self-end"
                onClick={() => { verify.mutate({ actionId, accepted: false, feedback }, { onSuccess: () => { toast.success('Sent back for rework'); setFeedback(''); setShowRejectInput(false) } }) }}
                disabled={!feedback.trim() || verify.isPending}
              >
                Send
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
