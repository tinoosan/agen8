import { useState } from 'react'
import { useLocation } from 'wouter'
import { useOpAction, useStartOpAction, useCompleteOpAction, useVerifyOpAction, useBlockOpAction, useUnblockOpAction, useCancelOpAction, useAddOpActionNote } from '../../hooks/useOpActions'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Play, CheckCircle2, Ban, Lock, Unlock, Plus, Trash2, Clock, User, FileText, ExternalLink, CornerDownLeft } from 'lucide-react'
import { cn } from '@/lib/utils'
import { toast } from 'sonner'
import type { OpActionStatus, OpActionOutcomeStatus, OpActionNote } from '../../lib/types'

/* ── Status display ──────────────────────────────────── */

function statusConfig(status: OpActionStatus): { label: string; color: string; bg: string } {
  switch (status) {
    case 'pending': return { label: 'Pending', color: 'var(--amber)', bg: 'var(--amber-dim)' }
    case 'acknowledged': return { label: 'Seen', color: 'var(--blue)', bg: 'var(--blue-dim, rgba(96,165,250,0.12))' }
    case 'in_progress': return { label: 'In Progress', color: 'var(--accent)', bg: 'rgba(59,130,246,0.12)' }
    case 'pending_verification': return { label: 'Awaiting Verification', color: 'var(--amber)', bg: 'var(--amber-dim)' }
    case 'completed': return { label: 'Completed', color: 'var(--green)', bg: 'var(--green-dim)' }
    case 'blocked': return { label: 'Blocked', color: 'var(--red)', bg: 'var(--red-dim)' }
    case 'canceled': return { label: 'Canceled', color: 'var(--text-3)', bg: 'var(--bg-surface)' }
    default: return { label: status, color: 'var(--text-3)', bg: 'var(--bg-surface)' }
  }
}

function statusToneClass(status: OpActionStatus): string {
  switch (status) {
    case 'pending': return 'dashboard-meta-chip-warning'
    case 'acknowledged': return 'dashboard-meta-chip-info'
    case 'in_progress': return 'dashboard-meta-chip-accent'
    case 'pending_verification': return 'dashboard-meta-chip-warning'
    case 'completed': return 'dashboard-meta-chip-success'
    case 'blocked': return 'dashboard-meta-chip-critical'
    case 'canceled': return 'dashboard-meta-chip-muted'
    default: return 'dashboard-meta-chip-muted'
  }
}

function metaLabelToneClass(tone: 'critical' | 'warning' | 'muted'): string {
  switch (tone) {
    case 'critical':
      return 'dashboard-inline-label dashboard-inline-label-critical'
    case 'warning':
      return 'dashboard-inline-label dashboard-inline-label-warning'
    default:
      return 'dashboard-inline-label'
  }
}

function actionButtonClass(tone: 'neutral' | 'accent' | 'success' | 'warning' | 'danger' = 'neutral'): string {
  return cn(
    'dashboard-action-button text-xs',
    tone === 'accent' && 'dashboard-action-button-accent',
    tone === 'success' && 'dashboard-action-button-success',
    tone === 'warning' && 'dashboard-action-button-warning',
    tone === 'danger' && 'dashboard-action-button-danger',
    tone === 'neutral' && 'dashboard-action-button-neutral',
  )
}

/* ── Lifecycle timeline ──────────────────────────────── */

function LifecycleTimeline({ status, requiresVerification }: { status: OpActionStatus; requiresVerification: boolean }) {
  const steps = requiresVerification
    ? [{ key: 'pending', label: 'Created' }, { key: 'in_progress', label: 'Working' }, { key: 'pending_verification', label: 'Verify' }, { key: 'completed', label: 'Done' }]
    : [{ key: 'pending', label: 'Created' }, { key: 'in_progress', label: 'Working' }, { key: 'completed', label: 'Done' }]

  const statusOrder: Record<string, number> = { pending: 0, acknowledged: 0, in_progress: 1, pending_verification: 2, completed: requiresVerification ? 3 : 2 }
  const currentStep = statusOrder[status] ?? -1
  const isTerminal = status === 'completed' || status === 'canceled'
  const isBlocked = status === 'blocked'

  return (
    <div className="flex py-3">
      {steps.map((step, i) => {
        const isPast = i < currentStep
        const isCurrent = i === currentStep && !isTerminal
        const isDone = isTerminal && i <= currentStep
        const filled = isPast || isDone

        return (
          <div key={step.key} className="flex-1 flex flex-col items-center relative">
            {/* Left half-connector */}
            {i > 0 && (
              <div className={cn(
                'absolute top-[5px] left-0 right-1/2 h-px',
                (isPast || isCurrent || isDone) ? 'bg-[var(--green)]' : 'bg-[var(--border)]',
              )} />
            )}
            {/* Right half-connector */}
            {i < steps.length - 1 && (
              <div className={cn(
                'absolute top-[5px] left-1/2 right-0 h-px',
                filled ? 'bg-[var(--green)]' : 'bg-[var(--border)]',
              )} />
            )}
            {/* Dot */}
            <div className={cn(
              'relative z-10 w-2.5 h-2.5 rounded-full transition-all duration-300',
              isBlocked && isCurrent ? 'bg-[var(--red)] ring-2 ring-[var(--red)]/30'
                : isDone ? 'bg-[var(--green)]'
                : isCurrent ? 'bg-[var(--accent)] ring-2 ring-[var(--accent)]/30'
                : isPast ? 'bg-[var(--green)]'
                : 'bg-[var(--border-strong)]',
            )} />
            {/* Label */}
            <span className={cn(
              'text-[9px] font-medium mt-1.5',
              (isPast || isDone) ? 'text-[var(--text-2)]'
                : isCurrent && isBlocked ? 'text-[var(--red)]'
                : isCurrent ? 'text-[var(--accent)]'
                : 'text-[var(--text-3)]',
            )}>
              {isBlocked && isCurrent ? 'Blocked' : step.label}
            </span>
          </div>
        )
      })}
    </div>
  )
}

/* ── Time-ago helper ───────────────────────────────── */

function timeAgo(iso: string | undefined): string {
  if (!iso) return ''
  const diffMs = Date.now() - new Date(iso).getTime()
  if (diffMs < 0) return 'just now'
  const seconds = Math.floor(diffMs / 1000)
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

/* ── Progress notes section ──────────────────────────── */

const NOTES_VISIBLE = 4

function NotesSection({ notes, actionId }: { notes: OpActionNote[]; actionId: string }) {
  const [text, setText] = useState('')
  const [showAll, setShowAll] = useState(false)
  const addNote = useAddOpActionNote()

  const hiddenCount = Math.max(0, notes.length - NOTES_VISIBLE)
  const visibleNotes = showAll || hiddenCount === 0 ? notes : notes.slice(-NOTES_VISIBLE)

  const handleAdd = () => {
    if (!text.trim()) return
    addNote.mutate(
      { actionId, text: text.trim() },
      { onSuccess: () => { setText(''); toast.success('Note added') } },
    )
  }

  return (
    <div>
      <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-2">Activity</div>
      {notes.length > 0 ? (
        <div className="mb-3">
          {hiddenCount > 0 && (
            <button
              onClick={() => setShowAll(v => !v)}
              className="text-xs text-[var(--text-3)] hover:text-[var(--text-2)] mb-2 transition-colors"
            >
              {showAll ? 'Collapse earlier notes' : `${hiddenCount} earlier note${hiddenCount !== 1 ? 's' : ''}`}
            </button>
          )}
          <div className="space-y-2">
            {visibleNotes.map((note, i) => (
              <div key={i} className="flex gap-2 text-xs">
                <span className="text-[var(--text-3)] tabular-nums shrink-0 text-[10px] mt-px">{timeAgo(note.createdAt)}</span>
                <span className="text-[var(--text-2)]">{note.text}</span>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <div className="text-xs text-[var(--text-3)] mb-3">No notes yet</div>
      )}
      <div className="note-composer rounded-[var(--r-md)] border border-[var(--border)] focus-within:border-[var(--border-strong)] transition-colors">
        <Textarea
          value={text}
          onChange={e => setText(e.target.value)}
          placeholder="Leave a note..."
          className="min-h-[60px] text-sm resize-none border-0 bg-transparent shadow-none focus-visible:ring-0 focus-visible:ring-offset-0 pb-1"
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
            className={cn(actionButtonClass('accent'), 'h-6 px-2.5')}
          >
            {addNote.isPending ? 'Posting...' : 'Post note'}
          </Button>
        </div>
      </div>
    </div>
  )
}

/* ── Outcome form ────────────────────────────────────── */

function OutcomeForm({ actionId, onCancel }: { actionId: string; onCancel: () => void }) {
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
    <div className="rounded-[var(--r-md)] border border-[var(--border)] p-4 space-y-3 animate-fade-in">
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
        <div className="note-composer rounded-[var(--r-md)] border border-[var(--border)] focus-within:border-[var(--border-strong)] transition-colors">
          <Textarea
            value={summary}
            onChange={e => setSummary(e.target.value)}
            placeholder="What was accomplished..."
            className="min-h-[60px] text-sm resize-none border-0 bg-transparent shadow-none focus-visible:ring-0 focus-visible:ring-offset-0"
          />
        </div>
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
                <Input value={pair.key} onChange={e => updatePair(i, 'key', e.target.value)} placeholder="Key" className="text-xs h-7 flex-1" />
                <span className="text-[var(--text-3)] text-xs">:</span>
                <Input value={pair.value} onChange={e => updatePair(i, 'value', e.target.value)} placeholder="Value" className="text-xs h-7 flex-1" />
                <Button variant="ghost" size="xs" onClick={() => removePair(i)} className="text-[var(--text-3)] hover:text-[var(--red)] shrink-0">
                  <Trash2 size={10} />
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="flex justify-end gap-2 pt-1">
        <Button variant="outline" size="sm" onClick={onCancel} className={actionButtonClass('neutral')}>Cancel</Button>
        <Button
          size="sm"
          onClick={handleSubmit}
          disabled={!summary.trim() || complete.isPending}
          className={actionButtonClass('success')}
        >
          {complete.isPending ? 'Completing...' : 'Complete'}
        </Button>
      </div>
    </div>
  )
}

/* ── Lifecycle action buttons ────────────────────────── */

function LifecycleActions({ actionId, status, onShowOutcomeForm }: { actionId: string; status: OpActionStatus; onShowOutcomeForm: () => void }) {
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
            className={actionButtonClass('accent')}
          >
            <Play size={12} className="mr-1" />
            {start.isPending ? 'Starting...' : 'Start Working'}
          </Button>
          <Button
            variant="outline" size="sm"
            onClick={() => cancel.mutate({ actionId }, { onSuccess: () => toast.success('Action canceled') })}
            disabled={cancel.isPending}
            className={actionButtonClass('neutral')}
          >
            Cancel
          </Button>
        </div>
      )}

      {status === 'in_progress' && (
        <>
          <div className="flex gap-2">
            <Button size="sm" onClick={onShowOutcomeForm} className={actionButtonClass('success')}>
              <CheckCircle2 size={12} className="mr-1" /> Complete
            </Button>
            <Button variant="outline" size="sm" onClick={() => setShowBlockInput(b => !b)} className={actionButtonClass('warning')}>
              <Lock size={11} className="mr-1" /> Block
            </Button>
            <Button
              variant="outline" size="sm"
              onClick={() => cancel.mutate({ actionId }, { onSuccess: () => toast.success('Action canceled') })}
              disabled={cancel.isPending}
              className={actionButtonClass('neutral')}
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
                <Button variant="ghost" size="xs" onClick={() => { setShowBlockInput(false); setBlockReason('') }} className="text-xs text-[var(--text-3)]">
                  Cancel
                </Button>
                <Button
                  size="xs"
                  onClick={() => block.mutate({ actionId, reason: blockReason }, { onSuccess: () => { toast.success('Blocked'); setBlockReason(''); setShowBlockInput(false) } })}
                  disabled={!blockReason.trim() || block.isPending}
                  className={actionButtonClass('warning')}
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
            className={actionButtonClass('accent')}
          >
            <Unlock size={12} className="mr-1" /> Unblock
          </Button>
          <Button
            variant="outline" size="sm"
            onClick={() => cancel.mutate({ actionId }, { onSuccess: () => toast.success('Canceled') })}
            disabled={cancel.isPending}
            className={actionButtonClass('neutral')}
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
              className={actionButtonClass('success')}
            >
              <CheckCircle2 size={12} className="mr-1" /> Accept
            </Button>
            <Button variant="outline" size="sm" onClick={() => setShowRejectInput(r => !r)} className={actionButtonClass('neutral')}>
              Request Changes
            </Button>
          </div>
          {showRejectInput && (
            <div className="rounded-[var(--r-md)] border border-[var(--border)] p-3 space-y-2.5 animate-fade-in">
              <div className="text-xs font-medium text-[var(--text-2)]">What needs to change?</div>
              <div className="note-composer rounded-[var(--r-md)] border border-[var(--border)] focus-within:border-[var(--border-strong)] transition-colors">
                <Textarea
                  autoFocus
                  value={feedback}
                  onChange={e => setFeedback(e.target.value)}
                  placeholder="Describe what needs to be revised..."
                  className="min-h-[60px] text-sm resize-none border-0 bg-transparent shadow-none focus-visible:ring-0 focus-visible:ring-offset-0 pb-1"
                  onKeyDown={e => {
                    if (e.key === 'Escape') { setShowRejectInput(false); setFeedback('') }
                  }}
                />
              </div>
              <div className="flex justify-end gap-2">
                <Button variant="ghost" size="xs" onClick={() => { setShowRejectInput(false); setFeedback('') }} className="text-xs text-[var(--text-3)]">
                  Cancel
                </Button>
                <Button
                  size="xs"
                  onClick={() => verify.mutate({ actionId, accepted: false, feedback }, { onSuccess: () => { toast.success('Sent back for rework'); setFeedback(''); setShowRejectInput(false) } })}
                  disabled={!feedback.trim() || verify.isPending}
                  className={actionButtonClass('accent')}
                >
                  {verify.isPending ? 'Sending...' : 'Send feedback'}
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}

/* ── Main detail panel ───────────────────────────────── */

interface OpActionDetailPanelProps {
  actionId: string | null
  projectId: string | null
  onClose: () => void
}

export default function OpActionDetailPanel({ actionId, projectId, onClose }: OpActionDetailPanelProps) {
  const { data: oa, isLoading } = useOpAction(actionId)
  const [showOutcomeForm, setShowOutcomeForm] = useState(false)
  const [, navigate] = useLocation()

  return (
    <Dialog open={!!actionId} onOpenChange={(open) => { if (!open) { setShowOutcomeForm(false); onClose() } }}>
      <DialogContent className="dashboard-dialog-content max-w-[min(90vw,560px)] p-0 gap-0 flex flex-col max-h-[85vh] bg-[var(--bg-panel)]">
        <DialogHeader className="sr-only">
          <DialogTitle>Operator Action</DialogTitle>
          <DialogDescription>View and manage this operator action</DialogDescription>
        </DialogHeader>

        {isLoading || !oa ? (
          <div className="p-5 space-y-4">
            <Skeleton className="h-6 w-3/4" />
            <Skeleton className="h-4 w-1/2" />
            <Skeleton className="h-20 w-full rounded-[var(--r-md)]" />
          </div>
        ) : (
          <>
            {/* Header */}
            <div className="px-5 py-4 border-b border-[var(--border)] pr-12">
              <div className="flex items-center gap-2 mb-1 flex-wrap">
                <span className={cn('dashboard-meta-chip dashboard-meta-chip-compact', statusToneClass(oa.status))}>
                  {statusConfig(oa.status).label}
                </span>
                {oa.urgency === 'critical' && <span className={metaLabelToneClass('critical')}>Critical</span>}
                {oa.urgency === 'high' && <span className={metaLabelToneClass('warning')}>High urgency</span>}
                {oa.blocking && <span className={metaLabelToneClass('muted')}>Blocking</span>}
              </div>
              <div className="flex items-center gap-3">
                <h3 className="text-[15px] font-semibold text-[var(--text-1)] tracking-[-0.02em] m-0 leading-snug">
                  {oa.title}
                </h3>
                {projectId && actionId && (
                  <button
                    onClick={() => { onClose(); navigate(`/project/${projectId}/actions/${actionId}`) }}
                    className="flex items-center gap-0.5 text-xs text-[var(--accent)] hover:underline shrink-0 bg-transparent border-0 p-0 cursor-pointer"
                  >
                    Open <ExternalLink size={10} className="ml-0.5" />
                  </button>
                )}
              </div>
            </div>

            {/* Scrollable body */}
            <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
              {/* Lifecycle timeline */}
              <LifecycleTimeline status={oa.status} requiresVerification={oa.requiresVerification} />

              {/* Description — flat, no grey card */}
              <div>
                <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1.5">Description</div>
                <div className="text-xs text-[var(--text-2)] leading-relaxed whitespace-pre-wrap">
                  {oa.description ?? <span className="italic text-[var(--text-3)]">No description provided.</span>}
                </div>
              </div>

              {/* Metadata grid */}
              <div className="grid grid-cols-2 gap-x-4 gap-y-2">
                {oa.category && <MetaItem label="Category" value={oa.category} />}
                {oa.sourceMemberLabel && <MetaItem label="Requested by" value={oa.sourceMemberLabel} icon={<User size={10} />} />}
                {oa.taskRef && <MetaItem label="Task" value={oa.taskRef} icon={<FileText size={10} />} />}
                {oa.deadline && <MetaItem label="Deadline" value={new Date(oa.deadline).toLocaleString()} icon={<Clock size={10} />} />}
                <MetaItem label="Created" value={timeAgo(oa.createdAt)} />
                {oa.startedAt && <MetaItem label="Started" value={timeAgo(oa.startedAt)} />}
                {oa.completedAt && <MetaItem label="Completed" value={timeAgo(oa.completedAt)} />}
              </div>

              {/* Outcome display */}
              {oa.outcomeSummary && (
                <div className={cn(
                  'rounded-[var(--r-md)] p-3 border',
                  oa.outcomeStatus === 'failed' ? 'bg-[var(--red-dim)] border-[var(--red)]/20'
                    : oa.outcomeStatus === 'partial' ? 'bg-[var(--amber-dim)] border-[var(--amber)]/20'
                    : 'bg-[var(--green-dim)] border-[var(--green)]/20',
                )}>
                  <div className={cn(
                    'label-sm mb-1',
                    oa.outcomeStatus === 'failed' ? 'text-[var(--red)]'
                      : oa.outcomeStatus === 'partial' ? 'text-[var(--amber)]'
                      : 'text-[var(--green)]',
                  )}>
                    {oa.outcomeStatus ?? 'completed'}
                  </div>
                  <div className="text-xs text-[var(--text-2)]">{oa.outcomeSummary}</div>
                  {oa.outcomePairs && Object.keys(oa.outcomePairs).length > 0 && (
                    <div className="mt-2 space-y-0.5">
                      {Object.entries(oa.outcomePairs).map(([k, v]) => (
                        <div key={k} className="text-[10px]">
                          <span className="text-[var(--text-3)]">{k}:</span>{' '}
                          <span className="text-[var(--text-1)] font-medium">{v}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}

              {/* Progress notes */}
              {(oa.status === 'in_progress' || oa.status === 'blocked' || (oa.progressNotes && oa.progressNotes.length > 0)) && (
                <NotesSection notes={oa.progressNotes ?? []} actionId={oa.id} />
              )}

              {/* Inline outcome form */}
              {showOutcomeForm && (
                <OutcomeForm actionId={oa.id} onCancel={() => setShowOutcomeForm(false)} />
              )}
            </div>

            {/* Sticky footer */}
            {!showOutcomeForm && (
              <div className="border-t border-[var(--border)] px-5 py-3">
                <LifecycleActions
                  actionId={oa.id}
                  status={oa.status}
                  onShowOutcomeForm={() => setShowOutcomeForm(true)}
                />
              </div>
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

/* ── Metadata item helper ────────────────────────────── */

function MetaItem({ label, value, icon }: { label: string; value: string; icon?: React.ReactNode }) {
  return (
    <div>
      <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-0.5">{label}</div>
      <div className="flex items-center gap-1 text-xs text-[var(--text-2)]">
        {icon && <span className="text-[var(--text-3)]">{icon}</span>}
        <span className="truncate">{value}</span>
      </div>
    </div>
  )
}
