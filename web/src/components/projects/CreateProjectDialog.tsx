import { useMemo, useState } from 'react'
import { rpcCall } from '../../lib/rpc'
import { useLocations } from '../../hooks/useLocations'
import { locationLabel, locationDescription, locationStatusLabel } from '../../lib/projectFormat'
import { cn } from '@/lib/utils'
import {
  Check, ChevronLeft, ChevronRight, FolderOpen, HardDrive, Plus, Server,
} from 'lucide-react'
import DirectoryPicker, { type DirectoryPickerResult } from '../files/DirectoryPicker'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import type { ProjectCreateResult } from '../../lib/types'

/* ── Create-project wizard ────────────────────────────────────
   3-step flow: pick an execution location, browse to a directory,
   then name + create the project. The location step is skipped
   automatically when only one ready location exists. */

type CreateStep = 'location' | 'directory' | 'details'

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

export default function CreateProjectDialog({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: (result: ProjectCreateResult) => void
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
      const result = await rpcCall<ProjectCreateResult>('project.create', {
        locationId: selectedLocation.id,
        root: selectedPath,
        title: name.trim() || undefined,
      })
      onSuccess(result)
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
