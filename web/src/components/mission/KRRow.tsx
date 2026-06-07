import { useState } from 'react'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import {
  ChevronRight,
  ChevronDown,
  Pencil,
  Trash2,
  BarChart2,
} from 'lucide-react'
import { useUpdateKeyResult, useDeleteKeyResult } from '../../hooks/useMissions'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import {
  MEASUREMENT_TYPES,
  DIRECTIONS,
  ghostInput,
  krStatusBadge,
  directionIcon,
} from './krFields'
import { KRProgressStrip } from './KRProgressStrip'
import { KRDetailView } from './KRDetailView'
import type { KeyResultView } from '../../lib/types'

/* ── KR row ─────────────────────────────────────────────── */

export function KRRow({
  kr, missionId,
  expanded: expandedProp, onExpandedChange,
}: {
  kr: KeyResultView
  missionId: string
  expanded?: boolean
  onExpandedChange?: (v: boolean) => void
}) {
  const [internalExpanded, setInternalExpanded] = useState(false)
  const expanded = expandedProp !== undefined ? expandedProp : internalExpanded
  const setExpanded = (v: boolean) => {
    if (onExpandedChange) onExpandedChange(v)
    else setInternalExpanded(v)
  }
  const [editing, setEditing]                 = useState(false)
  const [reportingProgress, setReporting]     = useState(false)

  // Edit state
  const [title, setTitle]                     = useState(kr.title)
  const [description, setDescription]         = useState(kr.description ?? '')
  const [measurementType, setMeasurementType] = useState(kr.measurementType ?? 'percentage')
  const [direction, setDirection]             = useState(kr.direction ?? 'increase')
  const [targetValue, setTargetValue]         = useState(String(kr.targetValue ?? ''))
  const [unit, setUnit]                       = useState(kr.unit ?? '')
  const [baseline, setBaseline]               = useState(kr.baseline != null ? String(kr.baseline) : '')

  const updateKR      = useUpdateKeyResult()
  const deleteKR      = useDeleteKeyResult()

  const badge    = krStatusBadge(kr.status)

  function startEdit() {
    setTitle(kr.title)
    setDescription(kr.description ?? '')
    setMeasurementType(kr.measurementType ?? 'percentage')
    setDirection(kr.direction ?? 'increase')
    setTargetValue(String(kr.targetValue ?? ''))
    setUnit(kr.unit ?? '')
    setBaseline(kr.baseline != null ? String(kr.baseline) : '')
    setExpanded(true)
    setEditing(true)
  }

  function cancelEdit() {
    setEditing(false)
  }

  async function handleSave() {
    const trimmedTitle = title.trim()
    if (!trimmedTitle) { toast.error('Key result title is required'); return }
    const parsedTarget = parseFloat(targetValue)
    if (isNaN(parsedTarget) || parsedTarget <= 0) { toast.error('Target value must be a positive number'); return }
    const parsedBaseline = baseline.trim() ? parseFloat(baseline) : undefined
    if (parsedBaseline !== undefined && isNaN(parsedBaseline)) { toast.error('Baseline must be a valid number'); return }

    try {
      await updateKR.mutateAsync({
        keyResultId: kr.id,
        missionId,
        title: trimmedTitle,
        description: description.trim() || undefined,
        measurementType,
        direction,
        targetValue: parsedTarget,
        unit: unit.trim() || undefined,
        baseline: parsedBaseline,
      })
      toast.success('Key result updated')
      setEditing(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update key result')
    }
  }

  async function handleDelete() {
    try {
      await deleteKR.mutateAsync({ keyResultId: kr.id, missionId })
      toast.success('Key result deleted')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete key result')
    }
  }

  // Selects rendered in the edit secondary strip
  const editSelects = [
    { label: 'Type',      value: measurementType,          set: setMeasurementType, options: MEASUREMENT_TYPES.map(t => ({ value: t.value, label: t.label })) },
    { label: 'Direction', value: direction,                 set: setDirection,       options: DIRECTIONS.map(d => ({ value: d.value, label: d.label })) },
  ] as Array<{ label: string; value: string; set: (v: string) => void; options: { value: string; label: string }[] }>

  return (
    <div className={cn('group', expanded && 'bg-[var(--bg-surface)] rounded-[var(--r-md)]')}>
      {/* Primary row */}
      <div className={cn(
        'flex items-center gap-2 px-3 py-2 hover:bg-[var(--bg-hover)] transition-colors',
        expanded ? 'rounded-t-[var(--r-md)]' : 'rounded-[var(--r-md)]',
      )}>
        {/* Expand chevron */}
        <button
          className="flex items-center justify-center w-4 h-4 shrink-0 bg-transparent border-none cursor-pointer p-0 text-[var(--text-3)]"
          onClick={() => { if (!editing) setExpanded(!expanded) }}
        >
          {expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        </button>

        <span className="shrink-0 w-3 flex justify-center">
          {directionIcon(kr.direction)}
        </span>

        {/* Title — ghost input when editing */}
        {editing ? (
          <input
            className={cn(ghostInput, 'flex-1 min-w-0 text-[var(--text-1)]')}
            style={{ fontSize: '0.8125rem', fontWeight: 500, letterSpacing: '-0.08px' }}
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') { e.preventDefault(); handleSave() }
              if (e.key === 'Escape') cancelEdit()
            }}
            autoFocus
          />
        ) : (
          <span className="flex-1 min-w-0 truncate text-[var(--text-1)]" style={{ fontSize: '0.8125rem', fontWeight: 500, letterSpacing: '-0.08px' }}>
            {kr.title}
          </span>
        )}

        {!editing && (
          <>
            {kr.progressPercent > 0 && (
              <span className="tabular-nums text-[var(--text-3)] shrink-0" style={{ fontSize: '0.6875rem', fontWeight: 500, letterSpacing: '-0.06px' }}>
                {Math.round(kr.progressPercent)}%
              </span>
            )}
            <Badge variant={badge.variant} className="text-[0.625rem] px-1.5 py-0 shrink-0">
              {badge.label}
            </Badge>
          </>
        )}

        {/* Actions */}
        {editing ? (
          <div className="flex items-center gap-2 shrink-0">
            <button
              className="text-[0.75rem] text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors bg-transparent border-none cursor-pointer p-0"
              style={{ letterSpacing: '-0.12px' }}
              onClick={cancelEdit}
            >
              Cancel
            </button>
            <button
              className="text-[0.75rem] text-[var(--accent)] font-medium hover:opacity-80 transition-opacity bg-transparent border-none cursor-pointer p-0 disabled:opacity-40"
              style={{ letterSpacing: '-0.12px' }}
              onClick={handleSave}
              disabled={updateKR.isPending}
            >
              {updateKR.isPending ? 'Saving…' : 'Save'}
            </button>
          </div>
        ) : (
          <div className="flex items-center gap-0.5 opacity-100 md:opacity-0 md:group-hover:opacity-100 transition-opacity shrink-0">
            <Button size="sm" variant="ghost" className="h-6 w-6 p-0 text-[var(--accent)]" onClick={() => { setExpanded(true); setReporting(true) }} title="Report progress">
              <BarChart2 size={11} />
            </Button>
            <Button size="sm" variant="ghost" className="h-6 w-6 p-0" onClick={startEdit} title="Edit">
              <Pencil size={11} />
            </Button>
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button size="sm" variant="ghost" className="h-6 w-6 p-0 text-[var(--red)]" title="Delete">
                  <Trash2 size={11} />
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete key result</AlertDialogTitle>
                  <AlertDialogDescription>
                    This will permanently delete "{kr.title}". This action cannot be undone.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={handleDelete} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
                    {deleteKR.isPending ? 'Deleting...' : 'Delete'}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        )}
      </div>

      {/* Expanded detail / edit area */}
      {expanded && (
        <div className="px-10 pb-3 pt-0.5">

          {/* ── Edit strip ── */}
          {editing && (
            <div className="flex flex-col gap-2 pt-1 pb-1">
              {/* Row 1: numeric fields */}
              <div className="flex items-center gap-4 flex-wrap">
                {([
                  { label: 'Target',   value: targetValue, set: setTargetValue, type: 'number', width: 'w-16' },
                  { label: 'Unit',     value: unit,        set: setUnit,        type: 'text',   width: 'w-20', placeholder: 'e.g. %' },
                  { label: 'Baseline', value: baseline,    set: setBaseline,    type: 'number', width: 'w-16', placeholder: '—' },
                ] as Array<{ label: string; value: string; set: (v: string) => void; type: string; width: string; placeholder?: string }>).map(({ label, value, set, type, width, placeholder }) => (
                  <div key={label} className="flex items-center gap-1.5">
                    <span className="text-[0.6875rem] text-[var(--text-3)]" style={{ letterSpacing: '-0.06px' }}>{label}</span>
                    <input
                      className={cn(ghostInput, width, 'text-[var(--text-1)] border-b border-[var(--border)]')}
                      style={{ fontSize: '0.75rem', letterSpacing: '-0.12px' }}
                      type={type}
                      value={value}
                      onChange={(e) => set(e.target.value)}
                      placeholder={placeholder}
                    />
                  </div>
                ))}
              </div>

              {/* Row 2: dropdowns */}
              <div className="flex items-center gap-4 flex-wrap">
                {editSelects.map(({ label, value, set, options }) => {
                  return (
                    <div key={label} className="flex items-center gap-1.5">
                      <span className="text-[0.6875rem] text-[var(--text-3)]" style={{ letterSpacing: '-0.06px' }}>{label}</span>
                      <select
                        className="bg-transparent border-none text-[var(--text-1)] outline-none"
                        style={{ fontSize: '0.75rem', letterSpacing: '-0.12px' }}
                        value={value}
                        onChange={(event) => set(event.target.value)}
                        aria-label={label}
                      >
                        {options.map(o => (
                          <option key={o.value} value={o.value}>{o.label}</option>
                        ))}
                      </select>
                    </div>
                  )
                })}
              </div>

              {/* Row 3: description */}
              <div className="flex items-start gap-1.5">
                <span className="text-[0.6875rem] text-[var(--text-3)] pt-0.5" style={{ letterSpacing: '-0.06px' }}>Note</span>
                <textarea
                  className={cn(ghostInput, 'flex-1 text-[var(--text-1)] border-b border-[var(--border)] resize-none leading-snug')}
                  style={{ fontSize: '0.75rem', letterSpacing: '-0.12px', minHeight: '48px' }}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Description…"
                  rows={3}
                  onInput={(e) => {
                    const t = e.currentTarget
                    t.style.height = 'auto'
                    t.style.height = t.scrollHeight + 'px'
                  }}
                />
              </div>
            </div>
          )}

          {/* ── Progress report strip ── */}
          {!editing && reportingProgress && (
            <KRProgressStrip kr={kr} missionId={missionId} onDone={() => setReporting(false)} />
          )}

          {/* ── Detail view (not editing) ── */}
          {!editing && !reportingProgress && (
            <KRDetailView kr={kr} />
          )}
        </div>
      )}
    </div>
  )
}
