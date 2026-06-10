import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { FileText } from 'lucide-react'
import { rpcCall } from '../../lib/rpc'
import { qk } from '../../lib/queryKeys'
import { basename } from '../files/filePreviewUtils'
import ArtifactViewer from '../files/ArtifactViewer'
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

export function TaskArtifactsSection({ task, projectId }: TaskArtifactsSectionProps) {
  const [openVPath, setOpenVPath] = useState<string | null>(null)

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
                    onClick={() => setOpenVPath(vpath)}
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
            <SheetTitle className="text-[13px] font-semibold tracking-[-0.02em] truncate pr-8">
              {openVPath ? basename(openVPath) : ''}
            </SheetTitle>
            <SheetDescription className="text-[11px] text-[var(--text-3)] truncate" style={{ fontFamily: 'monospace' }}>
              {openVPath ?? ''}
            </SheetDescription>
          </SheetHeader>
          <div className="flex-1 min-h-0">
            {viewerFile && (
              <ArtifactViewer
                file={viewerFile}
                preview={previewQuery.data}
                isLoading={previewQuery.isLoading}
                error={!!previewQuery.error}
                variant="slideover"
              />
            )}
          </div>
        </SheetContent>
      </Sheet>
    </>
  )
}
