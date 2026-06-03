import { useState } from 'react'
import { Link } from 'wouter'
import { toast } from 'sonner'
import MissionLifecycleActions from './MissionLifecycleActions'
import {
  useKeyResults,
  useUpdateMission,
  useDeleteMission,
  useCreateKeyResult,
  useUpdateKeyResult,
  useUpdateKRProgress,
  useDeleteKeyResult,
  useSetSpace,
} from '../../hooks/useMissions'
import { useAssignableProjectSpaces } from '../../hooks/useAssignableProjectSpaces'
import { useNavigation, missionDetailLink } from '../../lib/routing'
import { assignableSpaces, keyResultSpaceOwnerLabelFromKR, spaceSummaryLabel } from '../../lib/spaceOwnerLabels'
import type { MissionView, MissionStatus, KeyResultView, KeyResultStatus } from '../../lib/types'
import { formatKRProgress } from '../../lib/missionUtils'
import { cn } from '@/lib/utils'
// Card/CardContent/CardHeader removed — Apple DESIGN.md §7 (borderless
// cards). Mission cards now use plain divs with bg-[var(--bg-surface)].
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
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
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@/components/ui/collapsible'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  ChevronDown,
  ChevronRight,
  Pencil,
  Trash2,
  Plus,
  AlertCircle,
  TrendingUp,
  TrendingDown,
  BarChart2,
  Pin,
} from 'lucide-react'

/* ── Status helpers ──────────────────────────────────── */

function missionStatusBadge(status: MissionStatus): { variant: 'success' | 'warning' | 'info' | 'secondary' | 'danger' | 'accent'; label: string } {
  switch (status) {
    case 'active': return { variant: 'success', label: 'Active' }
    case 'paused': return { variant: 'warning', label: 'Paused' }
    case 'completed': return { variant: 'info', label: 'Completed' }
    case 'archived': return { variant: 'secondary', label: 'Archived' }
    case 'draft':
    default: return { variant: 'accent', label: 'Draft' }
  }
}

function krStatusBadge(status: KeyResultStatus): { variant: 'success' | 'warning' | 'info' | 'secondary' | 'danger'; label: string } {
  switch (status) {
    case 'on_track': return { variant: 'success', label: 'On Track' }
    case 'at_risk': return { variant: 'warning', label: 'At Risk' }
    case 'completed': return { variant: 'info', label: 'Completed' }
    case 'dropped': return { variant: 'danger', label: 'Dropped' }
    case 'open':
    default: return { variant: 'secondary', label: 'Open' }
  }
}

/* ── Measurement type / direction helpers ───────────── */

const MEASUREMENT_TYPES = [
  { value: 'percentage', label: 'Percentage', hint: '0–100%' },
  { value: 'count', label: 'Count', hint: 'Items toward target' },
  { value: 'numeric', label: 'Numeric', hint: 'Score, duration, etc.' },
  { value: 'currency', label: 'Currency', hint: '$, €, etc.' },
  { value: 'binary', label: 'Binary', hint: 'Done / Not done' },
] as const

const DIRECTIONS = [
  { value: 'increase', label: 'Increase', icon: TrendingUp },
  { value: 'decrease', label: 'Decrease', icon: TrendingDown },
] as const

function directionIcon(dir: string) {
  switch (dir) {
    case 'increase': return <TrendingUp size={10} className="text-[var(--green)]" />
    case 'decrease': return <TrendingDown size={10} className="text-[var(--red)]" />
    default: return null
  }
}

// Ghost input / textarea style — visually identical to read-only text, just editable
const ghostInput = "bg-transparent border-none outline-none p-0 m-0 font-[inherit] text-[inherit] w-full"

/* ── Add Key Result form ─────────────────────────────── */

function AddKeyResultForm({
  missionId,
  onCancel,
}: {
  missionId: string
  onCancel: () => void
}) {
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [measurementType, setMeasurementType] = useState('percentage')
  const [direction, setDirection] = useState('increase')
  const [targetValue, setTargetValue] = useState('')
  const [unit, setUnit] = useState('')
  const [baseline, setBaseline] = useState('')
  const [selectedSpace, setSelectedSpace] = useState<string>('')
  const createKR = useCreateKeyResult()
  const setSpaceMut = useSetSpace()
  const { projectId } = useNavigation()
  const spacesQuery = useAssignableProjectSpaces(projectId)
  const availableSpaces = assignableSpaces(spacesQuery.data ?? [])

  const isBinary = measurementType === 'binary'
  const isPercentage = measurementType === 'percentage'

  async function handleSubmit() {
    const trimmedTitle = title.trim()
    if (!trimmedTitle) {
      toast.error('Key result title is required')
      return
    }

    let parsedTarget: number | undefined
    if (isBinary) {
      parsedTarget = 1
    } else if (isPercentage) {
      parsedTarget = targetValue.trim() ? parseFloat(targetValue) : 100
      if (isNaN(parsedTarget) || parsedTarget <= 0 || parsedTarget > 100) {
        toast.error('Percentage target must be between 1 and 100')
        return
      }
    } else {
      parsedTarget = targetValue.trim() ? parseFloat(targetValue) : undefined
      if (parsedTarget !== undefined && (isNaN(parsedTarget) || parsedTarget <= 0)) {
        toast.error('Target value must be a positive number')
        return
      }
    }

    const parsedBaseline = baseline.trim() ? parseFloat(baseline) : undefined
    if (parsedBaseline !== undefined && isNaN(parsedBaseline)) {
      toast.error('Baseline must be a valid number')
      return
    }

    if (direction === 'decrease' && parsedBaseline === undefined) {
      toast.error('Baseline is required for decrease direction')
      return
    }

    try {
      const result = await createKR.mutateAsync({
        missionId,
        title: trimmedTitle,
        description: description.trim() || undefined,
        measurementType,
        direction,
        targetValue: parsedTarget,
        unit: unit.trim() || undefined,
        baseline: parsedBaseline,
      })
      if (selectedSpace) {
        await setSpaceMut.mutateAsync({
          keyResultId: result.keyResult.id,
          spaceId: selectedSpace,
          missionId,
        })
      }
      toast.success('Key result added')
      setTitle(''); setDescription(''); setTargetValue(''); setUnit(''); setBaseline('')
      setMeasurementType('percentage'); setDirection('increase'); setSelectedSpace('')
      onCancel()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create key result')
    }
  }

  return (
    <div className="border border-[var(--border)] rounded-[var(--r-md)] p-4 bg-[var(--bg-surface)]">
      <div className="flex flex-col gap-3">
        {/* Title */}
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">Title</Label>
          <Input
            placeholder="e.g. Reduce build time to under 2 minutes"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Escape') onCancel() }}
            autoFocus
          />
        </div>

        {/* Description */}
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">Description <span className="text-[var(--text-3)] font-normal">(optional)</span></Label>
          <Textarea
            placeholder="Additional context or criteria..."
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className="min-h-[48px] text-xs"
          />
        </div>

        {/* Measurement type + Direction */}
        <div className="flex gap-3">
          <div className={cn('flex flex-col gap-1.5', isBinary ? 'flex-1' : 'flex-1')}>
            <Label className="text-xs">Measurement type</Label>
            <Select value={measurementType} onValueChange={(v) => {
              setMeasurementType(v)
              if (v === 'binary') setDirection('increase') // binary is always 0→1
            }}>
              <SelectTrigger className="text-xs h-9">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {MEASUREMENT_TYPES.map(t => (
                  <SelectItem key={t.value} value={t.value} className="text-xs">
                    {t.label} <span className="text-[var(--text-3)] ml-1">{t.hint}</span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {!isBinary && (
            <div className="flex flex-col gap-1.5 flex-1">
              <Label className="text-xs">Direction</Label>
              <Select value={direction} onValueChange={setDirection}>
                <SelectTrigger className="text-xs h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {DIRECTIONS.map(d => (
                    <SelectItem key={d.value} value={d.value} className="text-xs">
                      <span className="flex items-center gap-1.5">
                        <d.icon size={12} /> {d.label}
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        </div>

        {/* Target + Unit + Baseline */}
        {!isBinary && (
          <div className="flex gap-3">
            <div className="flex flex-col gap-1.5 flex-1">
              <Label className="text-xs">Target value</Label>
              <Input
                type="number"
                placeholder={isPercentage ? '100' : 'e.g. 500'}
                value={targetValue}
                onChange={(e) => setTargetValue(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5 flex-1">
              <Label className="text-xs">Unit <span className="text-[var(--text-3)] font-normal">(optional)</span></Label>
              <Input
                placeholder={measurementType === 'currency' ? 'e.g. USD' : 'e.g. %, tasks'}
                value={unit}
                onChange={(e) => setUnit(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5 flex-1">
              <Label className="text-xs">Baseline <span className="text-[var(--text-3)] font-normal">{direction === 'decrease' ? '(required)' : '(optional)'}</span></Label>
              <Input
                type="number"
                placeholder="Starting value"
                value={baseline}
                onChange={(e) => setBaseline(e.target.value)}
              />
            </div>
          </div>
        )}

        {/* Assigned space (optional) */}
        {availableSpaces.length > 0 && (
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">Assigned space <span className="text-[var(--text-3)] font-normal">(optional)</span></Label>
            <Select
              value={selectedSpace || '__none__'}
              onValueChange={(v) => setSelectedSpace(v === '__none__' ? '' : v)}
            >
              <SelectTrigger className="text-xs h-9">
                <SelectValue placeholder="None — no space assigned" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__none__" className="text-xs">None</SelectItem>
                {availableSpaces.map(space => (
                  <SelectItem key={space.spaceId} value={space.spaceId} className="text-xs">
                    {spaceSummaryLabel(space)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {/* Actions */}
        <div className="flex items-center gap-2 justify-end pt-1">
          <Button size="sm" variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
          <Button size="sm" onClick={handleSubmit} disabled={createKR.isPending || setSpaceMut.isPending || !title.trim()}>
            {createKR.isPending || setSpaceMut.isPending ? 'Adding...' : 'Add Key Result'}
          </Button>
        </div>
      </div>
    </div>
  )
}

/* ── Inline KR editor row ────────────────────────────── */

function KeyResultEditorRow({
  kr,
  missionId,
}: {
  kr: KeyResultView
  missionId: string
}) {
  const [editing, setEditing] = useState(false)
  const [reportingProgress, setReportingProgress] = useState(false)

  // Structural edit state
  const [title, setTitle] = useState(kr.title)
  const [description, setDescription] = useState(kr.description ?? '')
  const [measurementType, setMeasurementType] = useState(kr.measurementType ?? 'percentage')
  const [direction, setDirection] = useState(kr.direction ?? 'increase')
  const [targetValue, setTargetValue] = useState(String(kr.targetValue))
  const [unit, setUnit] = useState(kr.unit ?? '')
  const [baseline, setBaseline] = useState(kr.baseline != null ? String(kr.baseline) : '')
  const [selectedSpace, setSelectedSpace] = useState<string>(kr.spaceId ?? '')

  // Progress report state
  const [progressValue, setProgressValue] = useState('')
  const [progressNote, setProgressNote] = useState('')

  const updateKR = useUpdateKeyResult()
  const updateProgress = useUpdateKRProgress()
  const deleteKR = useDeleteKeyResult()
  const setSpaceMut = useSetSpace()
  const { projectId } = useNavigation()
  const spacesQuery = useAssignableProjectSpaces(projectId, { includeDeleted: true })
  const allSpaces = spacesQuery.data ?? []
  const availableSpaces = assignableSpaces(allSpaces)
  const badge = krStatusBadge(kr.status)

  const spaceName = keyResultSpaceOwnerLabelFromKR(kr, allSpaces)

  function resetEdit() {
    setTitle(kr.title)
    setDescription(kr.description ?? '')
    setMeasurementType(kr.measurementType ?? 'percentage')
    setDirection(kr.direction ?? 'increase')
    setTargetValue(String(kr.targetValue))
    setUnit(kr.unit ?? '')
    setBaseline(kr.baseline != null ? String(kr.baseline) : '')
    setSelectedSpace(kr.spaceId ?? '')
    setEditing(false)
  }

  function resetProgress() {
    setProgressValue('')
    setProgressNote('')
    setReportingProgress(false)
  }

  async function handleSave() {
    const trimmedTitle = title.trim()
    if (!trimmedTitle) {
      toast.error('Key result title is required')
      return
    }
    const parsedTarget = parseFloat(targetValue)
    if (isNaN(parsedTarget) || parsedTarget <= 0) {
      toast.error('Target value must be a positive number')
      return
    }

    try {
      const parsedBaseline = baseline.trim() ? parseFloat(baseline) : undefined
      if (parsedBaseline !== undefined && isNaN(parsedBaseline)) {
        toast.error('Baseline must be a valid number')
        return
      }
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
      const oldSpace = kr.spaceId ?? ''
      if (selectedSpace !== oldSpace) {
        await setSpaceMut.mutateAsync({
          keyResultId: kr.id,
          spaceId: selectedSpace,
          missionId,
        })
      }
      toast.success('Key result updated')
      setEditing(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update key result')
    }
  }

  async function handleReportProgress() {
    const trimmedNote = progressNote.trim()
    if (!trimmedNote) {
      toast.error('Note is required — explain what you measured')
      return
    }
    const parsedValue = parseFloat(progressValue)
    if (isNaN(parsedValue)) {
      toast.error('Value must be a valid number')
      return
    }

    try {
      await updateProgress.mutateAsync({
        keyResultId: kr.id,
        missionId,
        value: parsedValue,
        note: trimmedNote,
      })
      toast.success('Progress recorded')
      resetProgress()
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

  async function handleAssignSpace(spaceId: string) {
    try {
      await setSpaceMut.mutateAsync({ keyResultId: kr.id, spaceId, missionId })
      toast.success('Key result assigned')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to assign key result')
    }
  }

  // Shorten raw member actor IDs like "member:space-abc123/head-analyst" to "head-analyst".
  const updatedByShort = kr.lastUpdatedBy
    ? kr.lastUpdatedBy.split('/').pop() ?? kr.lastUpdatedBy
    : null

  return (
    <div className="group py-2.5">
      {/* Primary row */}
      <div className="flex items-center gap-2">
        <span className="shrink-0 w-3 flex justify-center">{kr.direction && directionIcon(kr.direction)}</span>

        {/* Title — ghost input when editing */}
        {editing ? (
          <input
            className={cn(ghostInput, 'flex-1 min-w-0 text-[var(--text-1)]')}
            style={{ fontSize: '13px', fontWeight: 500, letterSpacing: '-0.08px' }}
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') { e.preventDefault(); handleSave() }
              if (e.key === 'Escape') resetEdit()
            }}
            autoFocus
          />
        ) : (
          <span className="flex-1 min-w-0 truncate text-[var(--text-1)]" style={{ fontSize: '13px', fontWeight: 500, letterSpacing: '-0.08px' }}>
            {kr.title}
          </span>
        )}

        {!editing && (
          <>
            <span className="tabular-nums text-[var(--text-3)] shrink-0" style={{ fontSize: '11px', fontWeight: 500, letterSpacing: '-0.06px' }}>
              {Math.round(kr.progressPercent)}%
            </span>
            <Badge variant={badge.variant} className="text-[10px] px-1.5 py-0 shrink-0">
              {badge.label}
            </Badge>
          </>
        )}

        {/* Actions — save/cancel when editing, hover-reveal icons otherwise */}
        {editing ? (
          <div className="flex items-center gap-2 shrink-0">
            <button className="text-[12px] text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors" style={{ letterSpacing: '-0.12px' }} onClick={resetEdit}>Cancel</button>
            <button className="text-[12px] text-[var(--accent)] font-medium hover:opacity-80 transition-opacity" style={{ letterSpacing: '-0.12px' }} onClick={handleSave} disabled={updateKR.isPending || setSpaceMut.isPending}>
              {updateKR.isPending || setSpaceMut.isPending ? 'Saving…' : 'Save'}
            </button>
          </div>
        ) : (
          <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity shrink-0">
            <Button size="sm" variant="ghost" className="h-6 w-6 p-0 text-[var(--accent)]" onClick={() => setReportingProgress(true)} title="Report progress">
              <BarChart2 size={11} />
            </Button>
            <Button size="sm" variant="ghost" className="h-6 w-6 p-0" onClick={() => setEditing(true)} title="Edit">
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
                  <AlertDialogDescription>This will permanently delete "{kr.title}". This action cannot be undone.</AlertDialogDescription>
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

      {/* Secondary fields strip — shown while editing */}
      {editing && (
        <div className="pl-5 pt-2 pb-1 flex flex-col gap-2">
          {/* Row 1: numeric fields */}
          <div className="flex items-center gap-4 flex-wrap">
            {[
              { label: 'Target', value: targetValue, set: setTargetValue, type: 'number', width: 'w-16' },
              { label: 'Unit', value: unit, set: setUnit, type: 'text', width: 'w-20', placeholder: 'e.g. %' },
              { label: 'Baseline', value: baseline, set: setBaseline, type: 'number', width: 'w-16', placeholder: '—' },
            ].map(({ label, value, set, type, width, placeholder }) => (
              <div key={label} className="flex items-center gap-1.5">
                <span className="text-[11px] text-[var(--text-3)]" style={{ letterSpacing: '-0.06px' }}>{label}</span>
                <input
                  className={cn(ghostInput, width, 'text-[var(--text-1)] border-b border-[var(--border)]')}
                  style={{ fontSize: '12px', letterSpacing: '-0.12px' }}
                  type={type}
                  value={value}
                  onChange={(e) => set(e.target.value)}
                  placeholder={placeholder}
                />
              </div>
            ))}
          </div>
          {/* Row 2: selects */}
          <div className="flex items-center gap-4 flex-wrap">
            {([
              { label: 'Type', value: measurementType, set: setMeasurementType, options: MEASUREMENT_TYPES.map(t => ({ value: t.value, label: t.label })) },
              { label: 'Direction', value: direction, set: setDirection, options: DIRECTIONS.map(d => ({ value: d.value, label: d.label })) },
              ...(availableSpaces.length > 0 ? [{
                label: 'Space', value: selectedSpace || '__none__',
                set: (v: string) => setSelectedSpace(v === '__none__' ? '' : v),
                options: [{ value: '__none__', label: 'None' }, ...availableSpaces.map(t => ({ value: t.spaceId, label: spaceSummaryLabel(t) }))],
              }] : []),
            ] as Array<{ label: string; value: string; set: (v: string) => void; options: { value: string; label: string }[] }>).map(({ label, value, set, options }) => {
              const currentLabel = options.find(o => o.value === value)?.label ?? value
              return (
                <div key={label} className="flex items-center gap-1.5">
                  <span className="text-[11px] text-[var(--text-3)]" style={{ letterSpacing: '-0.06px' }}>{label}</span>
                  <DropdownMenu>
                    <DropdownMenuTrigger className="inline-flex items-center gap-0.5 text-[var(--text-1)] bg-transparent border-none outline-none cursor-pointer font-[inherit]" style={{ fontSize: '12px', letterSpacing: '-0.12px' }}>
                      {currentLabel}
                      <ChevronDown size={9} className="text-[var(--text-3)] ml-0.5 shrink-0" />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="start" sideOffset={4} className="min-w-[100px] p-1 !shadow-[0_4px_24px_rgba(0,0,0,0.07)] dark:!shadow-[0_4px_24px_rgba(0,0,0,0.25)]">
                      {options.map(o => (
                        <DropdownMenuItem
                          key={o.value}
                          className={cn(
                            'text-[12px] py-1 px-2 cursor-pointer',
                            o.value === value && 'text-[var(--accent)] font-medium'
                          )}
                          style={{ letterSpacing: '-0.12px' }}
                          onClick={() => set(o.value)}
                        >
                          {o.label}
                        </DropdownMenuItem>
                      ))}
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              )
            })}
          </div>
          {/* Row 3: description */}
          <div className="flex items-start gap-1.5">
            <span className="text-[11px] text-[var(--text-3)] pt-0.5" style={{ letterSpacing: '-0.06px' }}>Note</span>
            <textarea
              className={cn(ghostInput, 'flex-1 text-[var(--text-1)] border-b border-[var(--border)] resize-none leading-snug')}
              style={{ fontSize: '12px', letterSpacing: '-0.12px', minHeight: '48px' }}
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

      {/* Metadata row — hidden while editing or reporting */}
      {!editing && !reportingProgress && (
        <div className="flex items-center gap-2.5 mt-0.5 pl-5">
          <span className="text-[11px] text-[var(--text-3)] tabular-nums">{formatKRProgress(kr)}</span>
          {spaceName && <span className="text-[11px] text-[var(--text-3)]">{spaceName}</span>}
          {!spaceName && availableSpaces.length > 0 && (
            <DropdownMenu>
              <DropdownMenuTrigger className="text-[11px] text-[var(--accent)] hover:opacity-80 transition-opacity bg-transparent border-none cursor-pointer p-0">
                Assign space
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" sideOffset={4} className="min-w-[140px] p-1">
                {availableSpaces.map(space => (
                  <DropdownMenuItem
                    key={space.spaceId}
                    className="text-[12px] py-1 px-2 cursor-pointer"
                    onClick={() => handleAssignSpace(space.spaceId)}
                  >
                    {spaceSummaryLabel(space)}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          )}
          {updatedByShort && <span className="text-[11px] text-[var(--text-3)]">{updatedByShort}</span>}
        </div>
      )}


      {/* Progress report — minimal inline expand */}
      {reportingProgress && (
        <div className="pl-5 pt-2 pb-1 flex flex-col gap-2">
          <div className="flex items-center gap-3">
            <input
              className={cn(ghostInput, 'w-24 tabular-nums text-[var(--text-1)] border-b border-[var(--border)]')}
              style={{ fontSize: '13px', letterSpacing: '-0.08px' }}
              type="number"
              placeholder={`${kr.currentValue}`}
              value={progressValue}
              onChange={(e) => setProgressValue(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Escape') resetProgress() }}
              autoFocus
            />
            <span className="text-[11px] text-[var(--text-3)]">→ target {kr.targetValue}{kr.unit ? ` ${kr.unit}` : ''}</span>
          </div>
          <input
            className={cn(ghostInput, 'text-[var(--text-3)] border-b border-[var(--border)]')}
            style={{ fontSize: '12px', letterSpacing: '-0.12px' }}
            placeholder="What did you measure?"
            value={progressNote}
            onChange={(e) => setProgressNote(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Escape') resetProgress() }}
          />
          <div className="flex items-center gap-3 pt-0.5">
            <button className="text-[12px] text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors" style={{ letterSpacing: '-0.12px' }} onClick={resetProgress}>Cancel</button>
            <button
              className="text-[12px] text-[var(--accent)] font-medium hover:opacity-80 transition-opacity disabled:opacity-40"
              style={{ letterSpacing: '-0.12px' }}
              onClick={handleReportProgress}
              disabled={updateProgress.isPending || !progressValue.trim() || !progressNote.trim()}
            >
              {updateProgress.isPending ? 'Recording…' : 'Record'}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

/* ── Key Results list ────────────────────────────────── */

function KeyResultsList({
  missionId,
}: {
  missionId: string
}) {
  const { data: keyResults, isLoading, isError, error } = useKeyResults(missionId)
  const [showAddForm, setShowAddForm] = useState(false)

  if (isLoading) {
    return (
      <div className="flex flex-col gap-2 pt-1">
        <Skeleton className="h-10 rounded-[var(--r-sm)]" />
        <Skeleton className="h-10 rounded-[var(--r-sm)]" />
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex items-center gap-1.5 py-2 text-xs text-[var(--red)]">
        <AlertCircle size={12} />
        <span>Failed to load key results: {error instanceof Error ? error.message : 'Unknown error'}</span>
      </div>
    )
  }

  return (
    <div className="flex flex-col">
      {(!keyResults || keyResults.length === 0) && !showAddForm && (
        <div className="py-3 text-[13px] text-[var(--text-3)]">
          No key results yet. Add one to track progress toward this mission.
        </div>
      )}
      <div className="divide-y divide-[var(--border)]">
        {keyResults?.map((kr) => (
          <KeyResultEditorRow key={kr.id} kr={kr} missionId={missionId} />
        ))}
      </div>
      {showAddForm ? (
        <div className="mt-3">
          <AddKeyResultForm missionId={missionId} onCancel={() => setShowAddForm(false)} />
        </div>
      ) : (
        <Button
          size="sm"
          variant="ghost"
          className="self-start mt-2 text-[var(--accent)] px-0 hover:bg-transparent"
          onClick={() => setShowAddForm(true)}
        >
          <Plus size={12} className="mr-1" />
          Add Key Result
        </Button>
      )}
    </div>
  )
}

/* ── Main MissionEditor component ────────────────────── */

interface MissionEditorProps {
  mission: MissionView
  defaultExpanded?: boolean
  /** Controlled expand state. When provided, overrides internal state. */
  expanded?: boolean
  onExpandedChange?: (v: boolean) => void
  isPinned?: boolean
  onTogglePin?: () => void
}

export default function MissionEditor({
  mission,
  defaultExpanded = false,
  expanded: expandedProp,
  onExpandedChange,
  isPinned = false,
  onTogglePin,
}: MissionEditorProps) {
  const [internalExpanded, setInternalExpanded] = useState(defaultExpanded)
  const expanded = expandedProp !== undefined ? expandedProp : internalExpanded
  const setExpanded = (v: boolean) => {
    if (onExpandedChange) onExpandedChange(v)
    else setInternalExpanded(v)
  }
  const [editing, setEditing] = useState(false)
  const [editTitle, setEditTitle] = useState(mission.title)
  const [editDescription, setEditDescription] = useState(mission.description ?? '')

  const { projectId } = useNavigation()
  const updateMission = useUpdateMission()
  const deleteMission = useDeleteMission()

  const { data: keyResults } = useKeyResults(mission.id)
  const overallProgress =
    keyResults && keyResults.length > 0
      ? keyResults.reduce((sum, kr) => sum + kr.progressPercent, 0) / keyResults.length
      : 0
  const hasKeyResults = keyResults !== undefined && keyResults.length > 0

  const statusBadge = missionStatusBadge(mission.status)

  function resetEdit() {
    setEditTitle(mission.title)
    setEditDescription(mission.description ?? '')
    setEditing(false)
  }

  async function handleSaveEdit() {
    const trimmedTitle = editTitle.trim()
    if (!trimmedTitle) {
      toast.error('Mission title is required')
      return
    }

    try {
      await updateMission.mutateAsync({
        missionId: mission.id,
        title: trimmedTitle,
        description: editDescription.trim() || undefined,
      })
      toast.success('Mission updated')
      setEditing(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update mission')
    }
  }

  async function handleStatusChange(status: MissionStatus) {
    try {
      await updateMission.mutateAsync({ missionId: mission.id, status })
      toast.success(`Mission ${status === 'active' ? 'activated' : status}`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update mission status')
    }
  }

  async function handleDelete() {
    try {
      await deleteMission.mutateAsync({ missionId: mission.id })
      toast.success('Mission deleted')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete mission')
    }
  }

  return (
    <div className={cn(expanded && 'bg-[var(--bg-surface)] rounded-[var(--r-md)]')}>
    <Collapsible open={expanded} onOpenChange={setExpanded}>
        {/* ── Row — single line, hover-reveal actions ── */}
        <div className={cn(
          'group flex items-center gap-2 px-2 py-2 hover:bg-[var(--bg-hover)] transition-colors',
          expanded ? 'rounded-t-[var(--r-md)]' : 'rounded-[var(--r-md)]',
        )}>
          {/* Chevron — expand/collapse only */}
          <CollapsibleTrigger asChild>
            <button className="flex items-center justify-center w-4 h-4 shrink-0 bg-transparent border-none cursor-pointer p-0 text-[var(--text-3)]">
              {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            </button>
          </CollapsibleTrigger>

          {/* Status dot */}
          <span className={cn('w-2 h-2 rounded-full shrink-0', {
            'bg-[var(--green)]': mission.status === 'active',
            'bg-[var(--amber)]': mission.status === 'paused',
            'bg-[var(--blue)]': mission.status === 'completed',
            'bg-[var(--text-3)] opacity-50': mission.status === 'archived',
            'bg-[var(--text-3)]': mission.status === 'draft',
          })} />

          {/* Title — ghost input when editing, link to detail otherwise */}
          {editing ? (
            <input
              className={cn(ghostInput, 'flex-1 min-w-0 text-[var(--text-1)]')}
              style={{ fontSize: '14px', fontWeight: 600, letterSpacing: '-0.224px', lineHeight: 1.43 }}
              value={editTitle}
              onChange={(e) => setEditTitle(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') { e.preventDefault(); handleSaveEdit() }
                if (e.key === 'Escape') resetEdit()
              }}
              autoFocus
            />
          ) : (
            <Link
              href={projectId ? missionDetailLink(projectId, mission.id) : '#'}
              className="flex-1 min-w-0 no-underline"
            >
              <span className="block truncate text-[var(--text-1)] transition-colors duration-100 hover:text-[var(--accent)]" style={{ fontSize: '14px', fontWeight: 600, letterSpacing: '-0.224px', lineHeight: 1.43 }}>
                {mission.title}
              </span>
            </Link>
          )}

          {!editing && (
            <>
              {hasKeyResults && overallProgress > 0 && (
                <span className="tabular-nums text-[var(--text-3)] shrink-0" style={{ fontSize: '11px', fontWeight: 500, letterSpacing: '-0.06px' }}>
                  {Math.round(overallProgress)}%
                </span>
              )}
              <Badge variant={statusBadge.variant} className="text-[10px] px-1.5 py-0 shrink-0">
                {statusBadge.label}
              </Badge>
            </>
          )}

          {/* Actions — save/cancel when editing, hover-reveal icons otherwise */}
          {editing ? (
            <div className="flex items-center gap-2 shrink-0">
              <button className="text-[12px] text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors" style={{ letterSpacing: '-0.12px' }} onClick={resetEdit}>Cancel</button>
              <button className="text-[12px] text-[var(--accent)] font-medium hover:opacity-80 transition-opacity" style={{ letterSpacing: '-0.12px' }} onClick={handleSaveEdit} disabled={updateMission.isPending}>
                {updateMission.isPending ? 'Saving…' : 'Save'}
              </button>
            </div>
          ) : (
            <div className="flex items-center gap-0.5 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
              {onTogglePin && (
                <Button size="sm" variant="ghost" className="h-7 w-7 p-0" onClick={(e) => { e.stopPropagation(); onTogglePin() }} title={isPinned ? 'Unpin mission' : 'Pin mission'}>
                  <Pin size={12} className={cn(isPinned && 'fill-[var(--accent)] text-[var(--accent)]')} />
                </Button>
              )}
              <Button size="sm" variant="ghost" className="h-7 w-7 p-0" onClick={() => { setExpanded(true); setEditing(true) }} title="Edit mission">
                <Pencil size={13} />
              </Button>
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button size="sm" variant="ghost" className="h-7 w-7 p-0 text-[var(--red)]" title="Delete mission">
                    <Trash2 size={13} />
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>Delete mission</AlertDialogTitle>
                    <AlertDialogDescription>
                      This will permanently delete "{mission.title}" and all its key results. This action cannot be undone.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction
                      onClick={handleDelete}
                      className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                    >
                      {deleteMission.isPending ? 'Deleting...' : 'Delete'}
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </div>
          )}
        </div>

        <CollapsibleContent>
          <div className="px-4 pb-4 pt-0 border-t border-[var(--border)]/50">
            {/* Description — ghost textarea when editing, read-only text otherwise */}
            {(editing || mission.description) && (
              <div className="mt-3">
                {editing ? (
                  <textarea
                    className={cn(ghostInput, 'text-[var(--text-3)] leading-relaxed resize-none')}
                    style={{ fontSize: '13px', letterSpacing: '-0.08px', lineHeight: 1.5, minHeight: '48px' }}
                    value={editDescription}
                    onChange={(e) => setEditDescription(e.target.value)}
                    placeholder="Mission description…"
                    rows={3}
                    onInput={(e) => {
                      const t = e.currentTarget
                      t.style.height = 'auto'
                      t.style.height = t.scrollHeight + 'px'
                    }}
                    onKeyDown={(e) => { if (e.key === 'Escape') resetEdit() }}
                  />
                ) : (
                  <p className="mb-0 text-[var(--text-3)]" style={{ fontSize: '13px', letterSpacing: '-0.08px', lineHeight: 1.5 }}>
                    {mission.description}
                  </p>
                )}
              </div>
            )}

            {/* Lifecycle actions */}
            <div className="mt-3 mb-3">
              <MissionLifecycleActions
                mission={mission}
                onStatusChange={handleStatusChange}
                isPending={updateMission.isPending}
              />
            </div>

            {/* Key Results section */}
            <div className="pt-3">
              <div className="flex items-center gap-1.5 mb-1">
                <span className="text-[var(--text-3)]" style={{ fontSize: '11px', fontWeight: 500, letterSpacing: '-0.04px' }}>
                  Key Results
                </span>
                {keyResults && keyResults.length > 0 && (
                  <span className="text-[11px] text-[var(--text-3)] tabular-nums">{keyResults.length}</span>
                )}
              </div>
              <KeyResultsList missionId={mission.id} />
            </div>
          </div>
        </CollapsibleContent>
    </Collapsible>
    </div>
  )
}
