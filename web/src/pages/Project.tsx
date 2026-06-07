import { useEffect, useMemo, useState } from 'react'
import { useLocation } from 'wouter'
import { useProjects } from '../hooks/useProjects'
import { useLocations } from '../hooks/useLocations'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import { formatDate, formatRelative } from '@/lib/format'
import { projectDisplayName } from '@/lib/spaceHelpers'
import {
  Archive, Check, ChevronLeft, ChevronRight, FolderOpen,
  HardDrive, Link as LinkIcon, MoreHorizontal, Plus, Search, Server, Trash2,
} from 'lucide-react'
import type { ExecutionLocation, Project } from '../lib/types'
import DirectoryPicker, { type DirectoryPickerResult } from '../components/files/DirectoryPicker'
import LinkFolderDialog from '../components/projects/LinkFolderDialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogAction,
  AlertDialogCancel,
} from '@/components/ui/alert-dialog'
import { useStore } from '../lib/store'
import { cn } from '@/lib/utils'
import { brandIconFor } from '../lib/brandIcon'

/* ── Utility helpers ─────────────────────────────── */

function shortenPath(path: string): string {
  const parts = path.split('/')
  if (parts.length <= 3) return path
  return '…/' + parts.slice(-2).join('/')
}

function locationLabel(location: ExecutionLocation): string {
  if (location.label?.trim()) return location.label.trim()
  if (location.kind === 'local') return 'This machine'
  if (location.address?.host) return location.address.host
  return location.id
}

function locationDescription(location: ExecutionLocation): string {
  if (location.kind === 'local') return 'Local execution'
  const address = location.address
  if (address?.host) {
    const user = address.username ? `${address.username}@` : ''
    const port = address.port ? `:${address.port}` : ''
    return `${user}${address.host}${port}`
  }
  if (location.kind === 'managed') return 'Managed execution'
  return location.kind
}

function locationStatusLabel(location: ExecutionLocation): string {
  if (location.ready) return 'Ready'
  if (location.lastProbe?.failureCode) return location.lastProbe.failureCode.replaceAll('_', ' ')
  return location.status.replaceAll('_', ' ')
}

/* ── Types ───────────────────────────────────────── */

type StatusFilter = 'all' | 'active' | 'archived'
type ProjectRemoveAction = 'archive' | 'delete'
type CreateStep = 'location' | 'directory' | 'details'

/* ── Stepper ─────────────────────────────────────── */

const STEPS: { key: CreateStep; label: string }[] = [
  { key: 'location', label: 'Location' },
  { key: 'directory', label: 'Directory' },
  { key: 'details', label: 'Details' },
]

function StepIndicator({ currentStep, skipLocation }: { currentStep: CreateStep; skipLocation: boolean }) {
  const visibleSteps = skipLocation ? STEPS.filter(s => s.key !== 'location') : STEPS
  const currentIdx = visibleSteps.findIndex(s => s.key === currentStep)

  return (
    <div className="flex items-center gap-0 mb-4">
      {visibleSteps.map((step, i) => {
        const isDone = i < currentIdx
        const isActive = i === currentIdx
        return (
          <div key={step.key} className="flex items-center gap-0">
            {i > 0 && (
              <div className={cn(
                'mx-2 h-px w-6 sm:w-8',
                isDone ? 'bg-[var(--green)]' : 'bg-[var(--border)]',
              )} />
            )}
            <div className="flex items-center gap-1.5">
              <span className={cn(
                'flex h-[22px] w-[22px] items-center justify-center rounded-full text-[0.6875rem] font-semibold',
                isDone && 'bg-[var(--green-dim)] text-[var(--green)]',
                isActive && 'bg-[var(--accent)] text-white',
                !isDone && !isActive && 'bg-[var(--bg-surface)] text-[var(--text-4)]',
              )}>
                {isDone ? <Check size={12} /> : i + (skipLocation ? 2 : 1)}
              </span>
              <span className={cn(
                'text-[0.75rem] font-medium',
                isDone && 'text-[var(--text-3)]',
                isActive && 'text-[var(--text-1)]',
                !isDone && !isActive && 'text-[var(--text-4)]',
              )}>
                {step.label}
              </span>
            </div>
          </div>
        )
      })}
    </div>
  )
}

/* ── CreateProjectDialog ─────────────────────────── */

function CreateProjectDialog({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: (project: Project) => void
}) {
  const locationsQuery = useLocations()
  const locations = useMemo(() => locationsQuery.data ?? [], [locationsQuery.data])
  const readyLocations = useMemo(() => locations.filter(l => l.ready), [locations])
  const hasMultipleLocations = readyLocations.length > 1

  const [selectedLocationId, setSelectedLocationId] = useState<string | null>(null)
  const [selectedPath, setSelectedPath] = useState<string | null>(null)
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // Auto-select the first ready location
  const selectedLocation = useMemo(() => {
    const local = readyLocations.find(l => l.id === 'local') ?? null
    const preferredID = selectedLocationId ?? local?.id ?? readyLocations[0]?.id ?? null
    return locations.find(l => l.id === preferredID) ?? null
  }, [locations, readyLocations, selectedLocationId])

  // Skip location step when only one location
  const skipLocation = !hasMultipleLocations && readyLocations.length > 0
  const initialStep: CreateStep = skipLocation ? 'directory' : 'location'
  const [step, setStep] = useState<CreateStep>(initialStep)
  const currentStep: CreateStep = skipLocation && step === 'location' ? 'directory' : step

  async function handleCreate() {
    if (!selectedPath || !selectedLocation) return
    setBusy(true)
    setError(null)
    try {
      const result = await rpcCall<{ project: Project }>('project.create', {
        locationId: selectedLocation.id,
        root: selectedPath,
        title: name.trim() || undefined,
      })
      onSuccess(result.project)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create project')
      setBusy(false)
    }
  }

  function handleDirectorySelected(path: string) {
    setSelectedPath(path)
    setStep('details')
  }

  const noLocations = !locationsQuery.isLoading && readyLocations.length === 0

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!busy) onOpenChange(v) }}>
      <DialogContent data-tour-dialog="project" className="max-h-[calc(100vh-2rem)] max-w-[min(92vw,500px)] overflow-hidden rounded-[var(--r-xl)] border-[var(--border)] bg-[var(--bg-panel)] p-0 shadow-[var(--shadow-lg)] gap-0 [&>button]:hidden">
        {/* Header */}
        <DialogHeader className="px-5 pt-5 pb-3 border-b border-[var(--border)]">
          <DialogTitle className="text-[0.9375rem] font-semibold text-[var(--text-1)]">
            New project
          </DialogTitle>
          <DialogDescription className="sr-only">
            Create a new project by choosing a location and directory.
          </DialogDescription>
        </DialogHeader>

        {/* Body */}
        <div className="min-w-0 overflow-x-hidden overflow-y-auto px-5 py-4">
          {locationsQuery.isLoading ? (
            <div className="flex h-[180px] items-center justify-center">
              <span className="spinner spinner-sm" />
            </div>
          ) : noLocations ? (
            <div className="flex flex-col gap-3 rounded-[var(--r-lg)] border border-[var(--border)] bg-[var(--bg-surface)] p-4 text-[0.8125rem] text-[var(--text-2)]">
              <div>No ready execution location is available.</div>
            </div>
          ) : (
            <>
              <StepIndicator currentStep={currentStep} skipLocation={skipLocation} />

              {/* Step 1: Location */}
              {currentStep === 'location' && (
                <div className="flex min-w-0 flex-col gap-3">
                  <label className="text-[0.6875rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-3)]">
                    Choose execution location
                  </label>
                  <div className="grid gap-2">
                    {locations.map((location) => {
                      const selected = selectedLocation?.id === location.id
                      const disabled = !location.ready
                      const Icon = location.kind === 'local' ? HardDrive : Server
                      return (
                        <button
                          key={location.id}
                          type="button"
                          disabled={disabled}
                          onClick={() => {
                            setSelectedLocationId(location.id)
                            setSelectedPath(null)
                          }}
                          className={cn(
                            'flex w-full items-center gap-3 rounded-[var(--r-md)] border px-3 py-2.5 text-left font-[inherit] transition-colors',
                            disabled
                              ? 'cursor-not-allowed border-[var(--border)] bg-[var(--bg-surface)] opacity-60'
                              : 'cursor-pointer hover:bg-[var(--bg-active)]',
                            selected
                              ? 'border-[var(--accent)] bg-[var(--accent-dim)]'
                              : 'border-[var(--border)] bg-[var(--bg-panel)]',
                          )}
                        >
                          <span className={cn(
                            'flex h-8 w-8 items-center justify-center rounded-[var(--r-md)] shrink-0',
                            selected ? 'bg-[rgba(107,138,253,0.15)] text-[var(--accent)]' : 'bg-[var(--bg-surface)] text-[var(--text-3)]',
                          )}>
                            <Icon size={15} />
                          </span>
                          <span className="min-w-0 flex-1">
                            <span className="block truncate text-[0.8125rem] font-semibold text-[var(--text-1)]">
                              {locationLabel(location)}
                            </span>
                            <span className="block truncate text-[0.6875rem] text-[var(--text-3)]">
                              {locationDescription(location)}
                            </span>
                          </span>
                          <span className={cn(
                            'shrink-0 text-[0.6875rem]',
                            location.ready ? 'text-[var(--green)]' : 'text-[var(--text-3)]',
                          )}>
                            {locationStatusLabel(location)}
                          </span>
                        </button>
                      )
                    })}
                  </div>
                </div>
              )}

              {/* Step 2: Directory */}
              {currentStep === 'directory' && selectedLocation && (
                <div className="flex flex-col gap-3">
                  {/* Selected location chip */}
                  <div className="flex items-center gap-2">
                    <span className="text-[0.6875rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-3)]">
                      Browsing
                    </span>
                    <span className="inline-flex items-center gap-1.5 rounded-[var(--r-md)] border border-[rgba(107,138,253,0.2)] bg-[var(--accent-dim)] px-2 py-1 font-mono text-[0.6875rem] text-[var(--accent)]">
                      {locationLabel(selectedLocation)}
                    </span>
                  </div>
                  <DirectoryPicker
                    key={selectedLocation.id}
                    initialPath="~"
                    onSelect={handleDirectorySelected}
                    emptyLabel="No subdirectories in this location"
                    formatCurrentPath={(path) => `${locationLabel(selectedLocation)} ${path}`}
                    loadDirectory={async (path) => {
                      const res = await rpcCall<{ entries: Array<{ name: string; path: string; type: string }> }>(
                        'location.fs.listDir',
                        { locationId: selectedLocation.id, path },
                      )
                      return {
                        path,
                        entries: (res.entries ?? []).map((entry) => ({
                          name: entry.name,
                          path: entry.path,
                          isDir: entry.type === 'directory',
                        })),
                      } satisfies DirectoryPickerResult
                    }}
                  />
                </div>
              )}

              {/* Step 3: Details */}
              {currentStep === 'details' && selectedPath && (
                <div className="flex flex-col gap-4">
                  {/* Selected path */}
                  <div>
                    <label className="text-[0.6875rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-3)] mb-2 block">
                      Directory
                    </label>
                    <div className="flex items-center gap-2 rounded-[var(--r-md)] border border-[rgba(107,138,253,0.2)] bg-[var(--accent-dim)] px-3 py-2">
                      <FolderOpen size={14} className="text-[var(--accent)] shrink-0" />
                      <span className="truncate font-mono text-[0.75rem] text-[var(--accent)]">
                        {selectedPath}
                      </span>
                      {selectedLocation && (
                        <span className="ml-auto shrink-0 rounded-full bg-[var(--bg-surface)] px-2 py-0.5 text-[0.625rem] text-[var(--text-3)]">
                          {locationLabel(selectedLocation)}
                        </span>
                      )}
                    </div>
                  </div>
                  {/* Name input */}
                  <div>
                    <label className="text-[0.6875rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-3)] mb-2 block">
                      Display name{' '}
                      <span className="normal-case font-normal text-[var(--text-4)]">(optional)</span>
                    </label>
                    <Input
                      value={name}
                      onChange={e => setName(e.target.value)}
                      placeholder="My project"
                      className="h-9 text-[0.8125rem]"
                      autoFocus
                    />
                  </div>
                  {error && (
                    <div className="text-[0.75rem] text-[var(--red)]">{error}</div>
                  )}
                </div>
              )}
            </>
          )}
        </div>

        {/* Footer */}
        {!locationsQuery.isLoading && !noLocations && (
          <div className="flex items-center gap-2 border-t border-[var(--border)] px-5 py-3">
            {/* Back button */}
            {((currentStep === 'directory' && !skipLocation) || currentStep === 'details') && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  if (currentStep === 'details') {
                    setSelectedPath(null)
                    setStep('directory')
                  } else {
                    setStep('location')
                  }
                  setError(null)
                }}
                className="gap-1"
              >
                <ChevronLeft size={12} />
                Back
              </Button>
            )}
            <div className="flex-1" />
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onOpenChange(false)}
              className="text-[var(--text-3)]"
            >
              Cancel
            </Button>
            {currentStep === 'location' && (
              <Button
                size="sm"
                onClick={() => setStep('directory')}
                disabled={!selectedLocation}
                className="gap-1"
              >
                Next
                <ChevronRight size={12} />
              </Button>
            )}
            {currentStep === 'details' && (
              <Button
                size="sm"
                onClick={handleCreate}
                disabled={busy || !selectedPath}
                className="gap-1"
              >
                {busy ? (
                  'Creating...'
                ) : (
                  <>
                    <Plus size={13} />
                    Create project
                  </>
                )}
              </Button>
            )}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

/* ── ProjectTableRow ─────────────────────────────── */

function ProjectTableRow({
  project,
  onRemove,
  onLink,
}: {
  project: Project
  onRemove: (action: ProjectRemoveAction) => void
  onLink: () => void
}) {
  const [, navigate] = useLocation()
  const active = project.status === 'open'
  const archived = project.status === 'archived'
  const lastActivity = project.updatedAt

  return (
    <TableRow
      onClick={() => { if (!archived) navigate(`/project/${encodeURIComponent(project.id)}`) }}
      className={cn(
        'group cursor-pointer border-[var(--border)] transition-colors hover:bg-[var(--bg-hover)]',
        archived && 'opacity-70',
      )}
    >
      {/* Status dot */}
      <TableCell className="w-[28px] px-3 py-2.5">
        <span
          className={cn(
            'block h-[6px] w-[6px] rounded-full',
            active ? 'bg-[var(--green)]' : 'bg-[var(--text-4)]',
          )}
        />
      </TableCell>

      {/* Project name + path (stacked) */}
      <TableCell className="px-3 py-2.5">
        <div className="min-w-0">
          <div className={cn(
            'truncate text-[0.8125rem] font-semibold leading-tight',
            archived ? 'text-[var(--text-3)]' : 'text-[var(--text-1)]',
          )}>
            {projectDisplayName(project)}
          </div>
          <div
            className="mt-0.5 truncate font-mono text-[0.6875rem] leading-tight text-[var(--text-3)]"
            title={project.root}
          >
            {shortenPath(project.root)}
          </div>
        </div>
      </TableCell>

      {/* Status badge — hidden below 640px */}
      <TableCell className="hidden w-[80px] px-3 py-2.5 text-center @[640px]:table-cell">
        <Badge
          variant={active ? 'success' : 'secondary'}
          className="text-[0.625rem] px-1.5 py-0"
        >
          {active ? 'Active' : 'Archived'}
        </Badge>
      </TableCell>

      {/* Activity */}
      <TableCell className="w-[80px] px-3 py-2.5 text-right">
        <span
          className="whitespace-nowrap tabular-nums text-[0.75rem] text-[var(--text-3)]"
          title={lastActivity ? `Updated ${formatDate(lastActivity)}` : undefined}
        >
          {lastActivity ? formatRelative(lastActivity) : ''}
        </span>
      </TableCell>

      {/* Actions kebab */}
      <TableCell className="w-[36px] px-2 py-2.5" onClick={e => e.stopPropagation()}>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              aria-label="Project actions"
              className="flex h-6 w-6 items-center justify-center rounded-[var(--r-sm)] border-0 bg-transparent text-[var(--text-3)] opacity-0 transition-opacity cursor-pointer hover:bg-[var(--bg-active)] hover:text-[var(--text-1)] group-hover:opacity-100 focus:opacity-100"
            >
              <MoreHorizontal size={13} />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="text-xs min-w-[160px]">
            {!archived && (
              <>
                <DropdownMenuItem onSelect={onLink}>
                  <LinkIcon size={12} className="mr-2" />
                  Link this folder
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onSelect={() => onRemove('archive')}
                  className="text-[var(--red)] focus:text-[var(--red)]"
                >
                  <Archive size={12} className="mr-2" />
                  Archive project
                </DropdownMenuItem>
              </>
            )}
            <DropdownMenuItem
              onSelect={() => onRemove('delete')}
              className="text-[var(--red)] focus:text-[var(--red)]"
            >
              <Trash2 size={12} className="mr-2" />
              Delete project
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </TableCell>
    </TableRow>
  )
}

/* ── RemoveProjectDialog ─────────────────────────── */

function RemoveProjectDialog({
  project,
  action,
  onClose,
  onRemoved,
}: {
  project: Project
  action: ProjectRemoveAction
  onClose: () => void
  onRemoved: () => void
}) {
  const [busy, setBusy] = useState(false)
  const deleting = action === 'delete'

  async function handleRemove() {
    setBusy(true)
    try {
      if (deleting) {
        if (project.status !== 'archived') {
          await rpcCall('project.archive', { projectId: project.id })
        }
        await rpcCall('project.delete', { projectId: project.id })
      } else {
        await rpcCall('project.archive', { projectId: project.id })
      }
      onRemoved()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : deleting ? 'Failed to delete project' : 'Failed to archive project')
      setBusy(false)
    }
  }

  return (
    <AlertDialog open onOpenChange={(open) => { if (!open && !busy) onClose() }}>
      <AlertDialogContent className="bg-[var(--bg-panel)] border-[var(--border)] rounded-[var(--r-lg)] shadow-[var(--shadow-lg)] max-w-[min(92vw,420px)] p-0 gap-0">
        <AlertDialogHeader className="px-4 py-4">
          <AlertDialogTitle className="font-semibold text-[0.8125rem] text-[var(--text-1)]">
            {deleting ? 'Delete project' : 'Archive project'}
          </AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="text-[0.8125rem] text-[var(--text-2)] leading-[1.6]">
              {deleting ? 'Delete' : 'Archive'} <span className="font-semibold text-[var(--text-1)]">{projectDisplayName(project)}</span>? {deleting ? 'This removes the project record. Project files on disk are not deleted.' : 'It will leave active project views immediately.'}
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>

        <AlertDialogFooter className="flex-row justify-end gap-2 px-4 pb-4 sm:space-x-0">
          <AlertDialogCancel onClick={onClose} disabled={busy} className="mt-0 sm:mt-0">
            Cancel
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={handleRemove}
            disabled={busy}
            style={{ background: 'var(--red)', borderColor: 'var(--red)', color: 'white' }}
          >
            {busy ? (deleting ? 'Deleting...' : 'Archiving...') : deleting ? 'Delete' : 'Archive'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

/* ── FilterChip ──────────────────────────────────── */

function FilterChip({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'inline-flex items-center gap-1 whitespace-nowrap rounded-full border px-2.5 py-1 text-[0.6875rem] font-medium transition-colors cursor-pointer',
        active
          ? 'border-[var(--accent)] bg-[var(--accent-dim)] text-[var(--accent)]'
          : 'border-[var(--border)] bg-transparent text-[var(--text-3)] hover:border-[var(--border-strong)] hover:text-[var(--text-2)]',
      )}
    >
      {children}
    </button>
  )
}

/* ── Main Project Page ───────────────────────────── */

export default function ProjectPage() {
  const theme = useStore((s) => s.theme)
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [searchQuery, setSearchQuery] = useState('')
  const projectsQuery = useProjects({ includeArchived: true })
  const allProjects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data])
  const isLoading = projectsQuery.isLoading
  const queryClient = useQueryClient()
  const [, navigate] = useLocation()
  const [createOpen, setCreateOpen] = useState(() =>
    new URLSearchParams(window.location.search).get('new') === 'true'
  )
  const [removeTarget, setRemoveTarget] = useState<{ project: Project; action: ProjectRemoveAction } | null>(null)
  const [linkTarget, setLinkTarget] = useState<Project | null>(null)

  // Filter projects by status + search
  const filteredProjects = useMemo(() => {
    let list = allProjects
    if (statusFilter === 'active') list = list.filter(p => p.status === 'open')
    else if (statusFilter === 'archived') list = list.filter(p => p.status === 'archived')
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase().trim()
      list = list.filter(p =>
        projectDisplayName(p).toLowerCase().includes(q) ||
        p.root.toLowerCase().includes(q)
      )
    }
    return list
  }, [allProjects, statusFilter, searchQuery])

  const hasArchived = allProjects.some(p => p.status === 'archived')

  useEffect(() => {
    if (new URLSearchParams(window.location.search).get('new') === 'true') {
      window.history.replaceState({}, '', '/')
    }
  }, [])

  const handleCreateSuccess = (project: Project) => {
    queryClient.invalidateQueries({ queryKey: ['project.list'] })
    toast.success(`Project "${projectDisplayName(project)}" created`)
    if (project.id) {
      navigate(`/project/${encodeURIComponent(project.id)}`)
    }
  }

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto w-full max-w-[960px] px-4 py-6 sm:px-6 md:px-10 md:py-8">
        {/* Header */}
        <div className="flex items-center gap-3 mb-5 flex-wrap">
          <h1 className="m-0 text-[1.125rem] font-semibold tracking-[-0.03em] text-[var(--text-1)] leading-[1.1] sm:text-[1.25rem] hidden md:block">
            Projects
          </h1>
          <div className="flex-1" />
          {allProjects.length > 0 && (
            <Button
              variant="ghost"
              className="gap-[5px] text-[var(--text-3)]"
              onClick={() => setCreateOpen(true)}
              data-tour="new-project"
            >
              <Plus size={13} />
              New project
            </Button>
          )}
        </div>

        {/* Loading state */}
        {isLoading && (
          <div className="rounded-[var(--r-lg)] border border-[var(--border)] overflow-hidden">
            <div className="bg-[var(--bg-surface)] px-4 py-2.5">
              <Skeleton className="h-4 w-[200px] rounded" />
            </div>
            {[1, 2, 3].map(i => (
              <div key={i} className="flex items-center gap-3 px-4 py-3 border-t border-[var(--border)]">
                <Skeleton className="h-[6px] w-[6px] rounded-full" />
                <div className="flex-1 space-y-1.5">
                  <Skeleton className="h-3.5 w-[140px] rounded" />
                  <Skeleton className="h-3 w-[200px] rounded" />
                </div>
                <Skeleton className="h-3 w-[50px] rounded" />
              </div>
            ))}
          </div>
        )}

        {/* Empty state */}
        {!isLoading && allProjects.length === 0 && (
          <div className="flex flex-col items-center justify-center py-16 gap-5 text-center sm:py-20">
            <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--bg-elevated)]">
              <img
                src={brandIconFor(theme)}
                alt=""
                className="w-7 h-7 rounded-[5px]"
                aria-hidden="true"
              />
            </div>
            <div>
              <div className="font-semibold text-[1rem] text-[var(--text-1)] mb-1.5 tracking-[-0.02em]">
                Create your first project
              </div>
              <div className="text-[0.8125rem] text-[var(--text-3)] leading-[1.6] max-w-[360px] mx-auto">
                A project connects agen8 to a directory on your machine.
                Missions, key results, tasks, decisions, and graph context live inside a project.
              </div>
            </div>
            <Button
              onClick={() => setCreateOpen(true)}
              className="gap-1.5 mt-1"
              data-tour="new-project"
            >
              <Plus size={14} />
              New project
            </Button>
          </div>
        )}

        {/* Project table */}
        {!isLoading && allProjects.length > 0 && (
          <>
            {/* Filter bar: search + status chips */}
            <div className="flex items-center gap-2 mb-4 flex-wrap">
              <div className="relative flex-1 min-w-[140px] max-w-[260px]">
                <Search
                  size={13}
                  className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-3)] pointer-events-none"
                />
                <Input
                  value={searchQuery}
                  onChange={e => setSearchQuery(e.target.value)}
                  placeholder="Search projects..."
                  className="h-[32px] pl-8 text-[0.75rem]"
                />
              </div>
              <FilterChip active={statusFilter === 'all'} onClick={() => setStatusFilter('all')}>
                All
              </FilterChip>
              <FilterChip active={statusFilter === 'active'} onClick={() => setStatusFilter('active')}>
                Active
              </FilterChip>
              {hasArchived && (
                <FilterChip active={statusFilter === 'archived'} onClick={() => setStatusFilter('archived')}>
                  Archived
                </FilterChip>
              )}
            </div>

            {/* Table */}
            <div className="rounded-[var(--r-lg)] border border-[var(--border)] overflow-hidden bg-[var(--bg-panel)] @container">
              <Table className="w-full">
                <TableHeader>
                  <TableRow className="border-[var(--border)] bg-[var(--bg-surface)] hover:bg-[var(--bg-surface)]">
                    <TableHead className="w-[28px] h-8 px-3 text-[0.625rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-2)]" />
                    <TableHead className="h-8 px-3 text-[0.625rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-2)]">
                      Project
                    </TableHead>
                    <TableHead className="hidden w-[80px] h-8 px-3 text-center text-[0.625rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-2)] @[640px]:table-cell">
                      Status
                    </TableHead>
                    <TableHead className="w-[80px] h-8 px-3 text-right text-[0.625rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-2)]">
                      Activity
                    </TableHead>
                    <TableHead className="w-[36px] h-8 px-2" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredProjects.map(project => (
                    <ProjectTableRow
                      key={project.id}
                      project={project}
                      onRemove={(action) => setRemoveTarget({ project, action })}
                      onLink={() => setLinkTarget(project)}
                    />
                  ))}
                </TableBody>
              </Table>

              {/* No results for current filter */}
              {filteredProjects.length === 0 && (
                <div className="flex flex-col items-center justify-center py-12 text-center gap-2">
                  <div className="text-[0.8125rem] text-[var(--text-3)]">
                    {searchQuery.trim()
                      ? `No projects matching "${searchQuery}"`
                      : statusFilter === 'archived'
                        ? 'No archived projects'
                        : 'No active projects'
                    }
                  </div>
                  {searchQuery.trim() && (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-[var(--text-3)]"
                      onClick={() => setSearchQuery('')}
                    >
                      Clear search
                    </Button>
                  )}
                </div>
              )}
            </div>
          </>
        )}

        {/* Create project dialog */}
        {createOpen && (
          <CreateProjectDialog
            open={createOpen}
            onOpenChange={setCreateOpen}
            onSuccess={handleCreateSuccess}
          />
        )}

        {removeTarget && (
          <RemoveProjectDialog
            project={removeTarget.project}
            action={removeTarget.action}
            onClose={() => setRemoveTarget(null)}
            onRemoved={() => {
              const removed = removeTarget
              setRemoveTarget(null)
              queryClient.invalidateQueries({ queryKey: ['project.list'] })
              toast.success(`Project "${projectDisplayName(removed.project)}" ${removed.action === 'delete' ? 'deleted' : 'archived'}`)
            }}
          />
        )}

        {linkTarget && (
          <LinkFolderDialog
            project={linkTarget}
            onClose={() => setLinkTarget(null)}
          />
        )}
      </div>
    </div>
  )
}
