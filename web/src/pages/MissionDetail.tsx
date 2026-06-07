import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useRoute, Link } from 'wouter'
import { toast } from 'sonner'
import { rpcCall } from '../lib/rpc'
import {
  useKeyResults,
  useUpdateKeyResult,
  useUpdateKRProgress,
  useDeleteKeyResult,
} from '../hooks/useMissions'
import { useProjectTasks } from '../hooks/useProjectTasks'
import { useRecentDecisions } from '../hooks/useDecisions'
import { formatKRProgress } from '../lib/missionUtils'
import { entityDisplayTitle } from '../lib/displaySanitizers'
import { missionsPanelLink, taskDetailLink, decisionDetailLink } from '../lib/routing'
import ProgressHistory from '../components/mission/ProgressHistory'
import { DetailNotFound, DetailError } from '../components/detail/DetailStates'
import { DetailSkeleton } from '../components/detail/DetailSkeleton'
import { DetailHeader } from '../components/detail/DetailHeader'
import { RelatedList } from '../components/detail/RelatedList'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
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
import { cn } from '@/lib/utils'
import { formatDate } from '@/lib/format'
import { confidenceColor } from '@/lib/decisionDisplay'
import {
  ChevronRight,
  ChevronDown,
  TrendingUp,
  TrendingDown,
  Calendar,
  Pencil,
  Trash2,
  BarChart2,
  ChevronsUpDown,
} from 'lucide-react'
import type {
  MissionView,
  KeyResultView,
  KeyResultStatus,
  KeyResultDirection,
} from '../lib/types'

/* ── Constants ──────────────────────────────────────────── */

const MEASUREMENT_TYPES = [
  { value: 'percentage', label: 'Percentage' },
  { value: 'count',      label: 'Count' },
  { value: 'numeric',    label: 'Numeric' },
  { value: 'currency',   label: 'Currency' },
  { value: 'binary',     label: 'Binary' },
] as const

const DIRECTIONS = [
  { value: 'increase', label: 'Increase' },
  { value: 'decrease', label: 'Decrease' },
] as const

// Ghost input — blends with surrounding text, only keyboard reveals it
const ghostInput = 'bg-transparent border-none outline-none p-0 m-0 font-[inherit] text-[inherit] w-full'

/* ── Status / direction helpers ─────────────────────────── */


function missionStatusBadge(status: string): { variant: 'success' | 'warning' | 'info' | 'secondary' | 'accent'; label: string } {
  switch (status) {
    case 'active':    return { variant: 'success',   label: 'Active' }
    case 'paused':    return { variant: 'warning',   label: 'Paused' }
    case 'completed': return { variant: 'info',      label: 'Completed' }
    case 'archived':  return { variant: 'secondary', label: 'Archived' }
    default:          return { variant: 'accent',    label: 'Draft' }
  }
}

function krStatusBadge(status: KeyResultStatus): { variant: 'success' | 'warning' | 'info' | 'secondary' | 'danger'; label: string } {
  switch (status) {
    case 'on_track':  return { variant: 'success',   label: 'On Track' }
    case 'at_risk':   return { variant: 'warning',   label: 'At Risk' }
    case 'completed': return { variant: 'info',      label: 'Completed' }
    case 'dropped':   return { variant: 'danger',    label: 'Dropped' }
    default:          return { variant: 'secondary', label: 'Open' }
  }
}

function directionIcon(dir: KeyResultDirection) {
  switch (dir) {
    case 'increase': return <TrendingUp  size={10} className="text-[var(--green)]" />
    case 'decrease': return <TrendingDown size={10} className="text-[var(--red)]" />
  }
}

function measurementLabel(type: string): string {
  return MEASUREMENT_TYPES.find(t => t.value === type)?.label ?? type
}

function directionLabel(dir: string): string {
  return DIRECTIONS.find(d => d.value === dir)?.label ?? dir
}

function shortAgent(raw: string): string {
  const slashPart = raw.split('/').pop() ?? raw
  const label = slashPart.split(':').pop() ?? slashPart
  return label.replace(/[_-]/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}

/* ── KR row ─────────────────────────────────────────────── */

function KRRow({
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

  // Progress report state
  const [progressValue, setProgressValue] = useState('')
  const [progressNote, setProgressNote]   = useState('')

  const updateKR      = useUpdateKeyResult()
  const updateProgress = useUpdateKRProgress()
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

  function cancelProgress() {
    setProgressValue('')
    setProgressNote('')
    setReporting(false)
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

  async function handleReportProgress() {
    const trimmedNote = progressNote.trim()
    if (!trimmedNote) { toast.error('Note is required — explain what you measured'); return }
    const parsedValue = parseFloat(progressValue)
    if (isNaN(parsedValue)) { toast.error('Value must be a valid number'); return }

    try {
      await updateProgress.mutateAsync({ keyResultId: kr.id, missionId, value: parsedValue, note: trimmedNote })
      toast.success('Progress recorded')
      cancelProgress()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to record progress')
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
            <div className="flex flex-col gap-2 pt-1 pb-1">
              <div className="flex items-center gap-3">
                <input
                  className={cn(ghostInput, 'w-24 tabular-nums text-[var(--text-1)] border-b border-[var(--border)]')}
                  style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}
                  type="number"
                  placeholder={`${kr.currentValue ?? 0}`}
                  value={progressValue}
                  onChange={(e) => setProgressValue(e.target.value)}
                  onKeyDown={(e) => { if (e.key === 'Escape') cancelProgress() }}
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
                onKeyDown={(e) => { if (e.key === 'Escape') cancelProgress() }}
              />
              <div className="flex items-center gap-3 pt-0.5">
                <button
                  className="text-[0.75rem] text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors bg-transparent border-none cursor-pointer p-0"
                  style={{ letterSpacing: '-0.12px' }}
                  onClick={cancelProgress}
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
          )}

          {/* ── Detail view (not editing) ── */}
          {!editing && !reportingProgress && (
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
          )}
        </div>
      )}
    </div>
  )
}

/* ── Loading skeleton ────────────────────────────────────── */

function MissionDetailSkeleton() {
  return (
    <DetailSkeleton>
      <div className="flex flex-col gap-0.5">
        {[1, 2, 3].map(i => (
          <div key={i} className="flex items-center gap-3 px-3 py-2">
            <Skeleton className="h-3 w-3 rounded-sm shrink-0" />
            <Skeleton className="h-3 flex-1 max-w-[280px]" />
            <Skeleton className="h-3 w-8 ml-auto" />
            <Skeleton className="h-4 w-14 rounded-full" />
          </div>
        ))}
      </div>
    </DetailSkeleton>
  )
}

/* ── Main component ──────────────────────────────────────── */

export default function MissionDetail() {
  const [, params] = useRoute('/project/:projectId/missions/:missionId')
  const projectId  = params?.projectId  ? decodeURIComponent(params.projectId)  : null
  const missionId  = params?.missionId  ? decodeURIComponent(params.missionId)  : null
  const [expandedKrIds, setExpandedKrIds] = useState<Record<string, boolean>>({})

  const { data: mission, isLoading: missionLoading, isError: missionError, error: missionErr } =
    useQuery<MissionView>({
      queryKey: ['mission.get', missionId ?? ''],
      queryFn: async () => {
        const res = await rpcCall<{ mission: MissionView }>('mission.get', { missionId: missionId ?? '' })
        return res.mission
      },
      enabled: !!missionId,
      refetchInterval: 10_000,
    })

  const { data: keyResults, isLoading: krsLoading, isError: krsError, error: krsErr } =
    useKeyResults(missionId)

  // Related cross-references — the same set the strategy map's mission panel
  // surfaces. Called unconditionally (rules of hooks); both no-op until
  // projectId resolves.
  const { data: projectTasks } = useProjectTasks(projectId)
  const { data: projectDecisions } = useRecentDecisions(projectId)

  if (!projectId || !missionId) {
    return <DetailNotFound entity="mission" />
  }

  if (missionLoading || krsLoading) return <MissionDetailSkeleton />

  if (missionError || krsError) {
    const msg = missionErr instanceof Error ? missionErr.message
      : krsErr instanceof Error ? (krsErr as Error).message : 'Unknown error'
    return <DetailError entity="mission" message={msg} />
  }

  if (!mission) {
    return <DetailNotFound entity="mission" />
  }

  // Related cross-references (tasks + decisions). Filtering mirrors the strategy
  // map mission panel; rendered as the same flat, per-row-labelled list the task
  // and decision detail pages use.
  const krIds = new Set((keyResults ?? []).map((kr) => kr.id))
  const relatedTasks = (projectTasks ?? []).filter((t) => t.keyResultRef && krIds.has(t.keyResultRef))
  const relatedDecisions = (projectDecisions ?? []).filter(
    (d) => d.missionRef === mission.id || (d.keyResultRef && krIds.has(d.keyResultRef)),
  )
  const related: Array<{ key: string; label: string; title: string; to: string; suffix?: string; suffixColor?: string }> = []
  for (const t of relatedTasks) {
    related.push({
      key: t.id,
      label: 'Task',
      title: entityDisplayTitle(t.id, t.title, t.description),
      to: taskDetailLink(projectId, t.id),
    })
  }
  for (const d of relatedDecisions) {
    related.push({
      key: d.id,
      label: 'Decision',
      title: d.title,
      to: decisionDetailLink(projectId, d.id),
      ...(d.confidence > 0
        ? {
            suffix: `${Math.round(d.confidence * 100)}%`,
            suffixColor: confidenceColor(d.confidence),
          }
        : {}),
    })
  }

  const statusBadge = missionStatusBadge(mission.status)
  const overallProgress = keyResults && keyResults.length > 0
    ? keyResults.reduce((sum, kr) => sum + kr.progressPercent, 0) / keyResults.length
    : 0
  return (
    <div className="flex flex-col h-full overflow-y-auto">
      {/* Sticky header — full-width outer for background coverage, max-w inner for centering */}
      <DetailHeader backTo={missionsPanelLink(projectId)} backLabel="Missions">

          {/* Title row */}
          <div className="flex items-center gap-2.5 mb-1">
            <h1
              className="m-0 text-[var(--text-1)] flex-1 min-w-0"
              style={{ fontSize: '1.75rem', fontWeight: 700, letterSpacing: '-0.56px', lineHeight: 1.14 }}
            >
              {mission.title}
            </h1>
            <Badge variant={statusBadge.variant} className="shrink-0">
              {statusBadge.label}
            </Badge>
            {keyResults && keyResults.length > 0 && overallProgress > 0 && (
              <span className="tabular-nums text-[var(--text-3)] shrink-0" style={{ fontSize: '0.8125rem', fontWeight: 500, letterSpacing: '-0.06px' }}>
                {Math.round(overallProgress)}%
              </span>
            )}
          </div>

          {/* Description */}
          {mission.description && (
            <p className="mt-2 mb-0 text-[var(--text-3)] ml-[18px]" style={{ fontSize: '0.875rem', letterSpacing: '-0.14px', lineHeight: 1.5 }}>
              {mission.description}
            </p>
          )}

          {/* Metadata */}
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 mt-2.5 ml-[18px]">
            {(mission.startDate || mission.endDate) && (
              <span className="inline-flex items-center gap-1 text-[var(--text-3)]" style={{ fontSize: '0.75rem', letterSpacing: '-0.08px' }}>
                <Calendar size={11} />
                {mission.startDate && mission.endDate
                  ? `${formatDate(mission.startDate, { style: 'numeric' })} – ${formatDate(mission.endDate, { style: 'numeric' })}`
                  : mission.endDate
                    ? `Due ${formatDate(mission.endDate, { style: 'numeric' })}`
                    : 'No deadline'}
              </span>
            )}
            {keyResults && keyResults.length > 0 && (
              <span className="text-[var(--text-3)]" style={{ fontSize: '0.75rem', letterSpacing: '-0.08px' }}>
                {keyResults.length} KR{keyResults.length !== 1 ? 's' : ''}
              </span>
            )}
          </div>

      </DetailHeader>

      {/* Scrollable KR list */}
      <div className="px-6 py-5 max-w-4xl mx-auto w-full">
        <div className="flex items-center gap-1.5 mb-2">
          <span className="text-[var(--text-3)]" style={{ fontSize: '0.6875rem', fontWeight: 500, letterSpacing: '-0.04px' }}>
            Key Results
          </span>
          {keyResults && keyResults.length > 0 && (
            <span className="text-[0.6875rem] text-[var(--text-3)] tabular-nums">{keyResults.length}</span>
          )}
          {keyResults && keyResults.length > 0 && (
            <button
              className="ml-auto inline-flex items-center gap-1 text-[var(--text-3)] hover:text-[var(--text-2)] transition-colors bg-transparent border-none cursor-pointer p-0"
              style={{ fontSize: '0.6875rem', letterSpacing: '-0.06px' }}
              onClick={() => {
                const allExpanded = keyResults.every(kr => !!expandedKrIds[kr.id])
                const newState = !allExpanded
                setExpandedKrIds(prev => {
                  const next = { ...prev }
                  for (const kr of keyResults) next[kr.id] = newState
                  return next
                })
              }}
            >
              <ChevronsUpDown size={11} />
              {keyResults.every(kr => !!expandedKrIds[kr.id]) ? 'Collapse all' : 'Expand all'}
            </button>
          )}
        </div>

        {(!keyResults || keyResults.length === 0) ? (
          <div className="rounded-[8px] bg-[var(--bg-surface)] px-4 py-8 text-center">
            <p className="text-[var(--text-3)] m-0" style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}>
              No key results defined yet.{' '}
              <Link
                to={missionsPanelLink(projectId)}
                className="text-[var(--accent)]"
              >
                Add from the missions list.
              </Link>
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-0.5">
            {keyResults.map(kr => (
              <KRRow
                key={kr.id}
                kr={kr}
                missionId={missionId}
                expanded={!!expandedKrIds[kr.id]}
                onExpandedChange={(v) => setExpandedKrIds(prev => ({ ...prev, [kr.id]: v }))}
              />
            ))}
          </div>
        )}
      </div>

      {/* Related — tasks + decisions linked to this mission, rendered with the
          same flat, per-row-labelled list the task and decision detail pages use */}
      {related.length > 0 && (
        <div className="px-6 pb-8 max-w-4xl mx-auto w-full">
          <RelatedList items={related} storageKey="mission-detail-related" />
        </div>
      )}
    </div>
  )
}
