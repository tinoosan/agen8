import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { FileText } from 'lucide-react'
import { rpcCall } from '../../lib/rpc'
import { qk } from '../../lib/queryKeys'
import { basename } from '../files/filePreviewUtils'
import ArtifactViewer from '../files/ArtifactViewer'
import DiffView from '../files/DiffView'
import { CollapsibleSection } from '../strategy/CollapsibleSection'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet'
import type { Task, ArtifactNode, ArtifactGetResult } from '../../lib/types'

// Caps the preview fetch; files.get reports `truncated` past this so the
// viewer can say so instead of silently clipping.
const ARTIFACT_PREVIEW_MAX_BYTES = 2_000_000

const FILE_REF_PREFIX = 'file:'

/** files.baseline response: the git HEAD version of a file, when one exists. */
interface FileBaselineResult {
  path: string
  tracked: boolean
  binary?: boolean
  content?: string
  truncated?: boolean
}

/** Extracts the vpath from a file:<vpath> artifact ref, or null for any other ref shape. */
export function fileArtifactVPath(ref: string): string | null {
  if (!ref.startsWith(FILE_REF_PREFIX)) return null
  const vpath = ref.slice(FILE_REF_PREFIX.length).trim()
  return vpath ? vpath : null
}

interface TaskArtifactsSectionProps {
  task: Task
  projectId: string | null
}

/**
 * Why diff mode cannot show a diff for this file, or null when it can.
 * A non-null reason degrades the viewer to normal view with a notice banner
 * rather than a dead diff pane.
 */
function baselineUnavailableReason(baseline: FileBaselineResult | undefined, error: boolean): string | null {
  if (error) return 'Could not load the git baseline for this file; showing normal view.'
  if (!baseline) return null
  if (!baseline.tracked) return 'No git baseline — this file is new or untracked, so there is nothing to diff against. Showing normal view.'
  if (baseline.binary) return 'The committed version of this file is binary; diff view is unavailable. Showing normal view.'
  return null
}

export function TaskArtifactsSection({ task, projectId }: TaskArtifactsSectionProps) {
  const [openVPath, setOpenVPath] = useState<string | null>(null)
  const [diffMode, setDiffMode] = useState(false)

  const previewQuery = useQuery<ArtifactGetResult>({
    queryKey: qk.filePreview(projectId, null, openVPath),
    queryFn: async () =>
      rpcCall<ArtifactGetResult>('files.get', {
        projectId: projectId ?? undefined,
        path: openVPath,
        maxBytes: ARTIFACT_PREVIEW_MAX_BYTES,
      }),
    enabled: !!projectId && !!openVPath,
    retry: false,
    staleTime: 30_000,
  })

  // Diff only applies to text content; the baseline is fetched lazily the
  // first time the toggle is used for this file.
  const diffable = previewQuery.data?.contentKind === 'text'
  const baselineQuery = useQuery<FileBaselineResult>({
    queryKey: ['files.baseline', projectId, openVPath],
    queryFn: async () =>
      rpcCall<FileBaselineResult>('files.baseline', {
        projectId: projectId ?? undefined,
        path: openVPath,
      }),
    enabled: !!projectId && !!openVPath && diffMode && diffable,
    retry: false,
    staleTime: 30_000,
  })

  const openArtifact = (vpath: string) => {
    setOpenVPath(vpath)
    setDiffMode(false)
  }

  if (!task.artifacts || task.artifacts.length === 0) return null

  const viewerFile: ArtifactNode | null = openVPath
    ? {
        nodeKey: FILE_REF_PREFIX + openVPath,
        kind: 'file',
        label: basename(openVPath),
        displayName: basename(openVPath),
        vpath: openVPath,
      }
    : null

  return (
    <>
      <CollapsibleSection
        storageKey="task-detail-artifacts"
        defaultOpen={false}
        label={<>Artifacts <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>({task.artifacts.length})</span></>}
      >
        <div style={{ borderTop: '1px solid var(--border)' }}>
          {task.artifacts.map((ref, i) => {
            const vpath = fileArtifactVPath(ref)
            return (
              <div
                key={i}
                style={{
                  paddingTop: 8,
                  paddingBottom: 8,
                  borderBottom: i < task.artifacts!.length - 1 ? '1px solid var(--border)' : 'none',
                }}
              >
                {vpath ? (
                  <button
                    type="button"
                    onClick={() => openArtifact(vpath)}
                    className="group inline-flex items-start gap-1.5 border-none cursor-pointer bg-transparent p-0 text-left"
                    aria-label={`View ${basename(vpath)}`}
                  >
                    <FileText size={13} className="shrink-0 mt-px text-[var(--text-3)] group-hover:text-[var(--text-1)] transition-colors" />
                    <span style={{ fontSize: '0.75rem', wordBreak: 'break-all' }}>
                      <span className="text-[var(--accent)] group-hover:underline underline-offset-2" style={{ fontFamily: 'monospace' }}>
                        {basename(vpath)}
                      </span>
                      <span className="text-[var(--text-3)]" style={{ fontFamily: 'monospace' }}> {vpath}</span>
                    </span>
                  </button>
                ) : (
                  <span style={{ fontSize: '0.75rem', color: 'var(--accent)', fontFamily: 'monospace', wordBreak: 'break-all' }}>{ref}</span>
                )}
              </div>
            )
          })}
        </div>
      </CollapsibleSection>

      <Sheet open={!!openVPath} onOpenChange={(open) => { if (!open) setOpenVPath(null) }}>
        <SheetContent
          side="right"
          className="w-screen sm:w-[min(720px,90vw)] sm:max-w-none p-0 gap-0 flex flex-col"
        >
          <SheetHeader className="shrink-0 border-b border-[var(--border)] px-4 py-3 space-y-0">
            <div className="flex items-center gap-2 pr-8">
              <SheetTitle className="text-[13px] font-semibold tracking-[-0.02em] truncate flex-1 min-w-0">
                {openVPath ? basename(openVPath) : ''}
              </SheetTitle>
              {diffable && (
                <div className="flex shrink-0 rounded-[var(--r-md)] border border-[var(--border)] overflow-hidden" role="group" aria-label="View mode">
                  <button
                    type="button"
                    onClick={() => setDiffMode(false)}
                    aria-pressed={!diffMode}
                    className="px-2 py-0.5 text-[11px] cursor-pointer border-none transition-colors"
                    style={{ background: diffMode ? 'transparent' : 'var(--bg-elevated)', color: diffMode ? 'var(--text-3)' : 'var(--text-1)' }}
                  >
                    Normal
                  </button>
                  <button
                    type="button"
                    onClick={() => setDiffMode(true)}
                    aria-pressed={diffMode}
                    className="px-2 py-0.5 text-[11px] cursor-pointer border-none transition-colors"
                    style={{ background: diffMode ? 'var(--bg-elevated)' : 'transparent', color: diffMode ? 'var(--text-1)' : 'var(--text-3)' }}
                  >
                    Diff
                  </button>
                </div>
              )}
            </div>
            <SheetDescription className="text-[11px] text-[var(--text-3)] truncate" style={{ fontFamily: 'monospace' }}>
              {openVPath ?? ''}
            </SheetDescription>
          </SheetHeader>
          <div className="flex-1 min-h-0 flex flex-col">
            {viewerFile && (() => {
              if (diffMode && diffable) {
                if (baselineQuery.isLoading) {
                  return (
                    <div className="flex items-center justify-center h-full">
                      <span className="spinner spinner-md" />
                    </div>
                  )
                }
                const reason = baselineUnavailableReason(baselineQuery.data, !!baselineQuery.error)
                if (!reason) {
                  return <DiffView baseline={baselineQuery.data?.content ?? ''} current={previewQuery.data?.content ?? ''} />
                }
                // Degrade: notice banner + the normal view, never a dead pane.
                return (
                  <>
                    <div className="shrink-0 px-4 py-2 text-[11px] text-[var(--text-3)] border-b border-[var(--border)]" data-testid="diff-unavailable-notice">
                      {reason}
                    </div>
                    <div className="flex-1 min-h-0">
                      <ArtifactViewer file={viewerFile} preview={previewQuery.data} isLoading={previewQuery.isLoading} error={!!previewQuery.error} variant="slideover" />
                    </div>
                  </>
                )
              }
              return (
                <ArtifactViewer
                  file={viewerFile}
                  preview={previewQuery.data}
                  isLoading={previewQuery.isLoading}
                  error={!!previewQuery.error}
                  variant="slideover"
                />
              )
            })()}
          </div>
        </SheetContent>
      </Sheet>
    </>
  )
}
