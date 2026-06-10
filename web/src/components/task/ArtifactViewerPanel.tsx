import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { rpcCall } from '../../lib/rpc'
import { qk } from '../../lib/queryKeys'
import { basename } from '../files/filePreviewUtils'
import ArtifactViewer from '../files/ArtifactViewer'
import DiffView from '../files/DiffView'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet'
import type { ArtifactNode, ArtifactGetResult } from '../../lib/types'

// Caps the preview fetch; files.get reports `truncated` past this so the
// viewer can say so instead of silently clipping.
const ARTIFACT_PREVIEW_MAX_BYTES = 2_000_000

/** files.baseline response: the git HEAD version of a file, when one exists. */
interface FileBaselineResult {
  path: string
  tracked: boolean
  binary?: boolean
  content?: string
  truncated?: boolean
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

interface ArtifactViewerPanelProps {
  projectId: string | null
  vpath: string
  onClose: () => void
  /**
   * sheet: overlay drawer for narrow viewports.
   * inline: split-panel column rendered beside the page content on wide
   * viewports, so the task stays in view while reviewing.
   */
  layout: 'sheet' | 'inline'
}

/**
 * Single-file viewer host: fetches the preview (and, in diff mode, the git
 * baseline) for one vpath and renders it in the chosen layout. Mount with
 * key={vpath} so switching files resets the diff toggle.
 */
export function ArtifactViewerPanel({ projectId, vpath, onClose, layout }: ArtifactViewerPanelProps) {
  const [diffMode, setDiffMode] = useState(false)

  const previewQuery = useQuery<ArtifactGetResult>({
    queryKey: qk.filePreview(projectId, null, vpath),
    queryFn: async () =>
      rpcCall<ArtifactGetResult>('files.get', {
        projectId: projectId ?? undefined,
        path: vpath,
        maxBytes: ARTIFACT_PREVIEW_MAX_BYTES,
      }),
    enabled: !!projectId,
    retry: false,
    staleTime: 30_000,
  })

  // Diff only applies to text content; the baseline is fetched lazily the
  // first time the toggle is used for this file.
  const diffable = previewQuery.data?.contentKind === 'text'
  const baselineQuery = useQuery<FileBaselineResult>({
    queryKey: ['files.baseline', projectId, vpath],
    queryFn: async () =>
      rpcCall<FileBaselineResult>('files.baseline', {
        projectId: projectId ?? undefined,
        path: vpath,
      }),
    enabled: !!projectId && diffMode && diffable,
    retry: false,
    staleTime: 30_000,
  })

  const viewerFile: ArtifactNode = {
    nodeKey: 'file:' + vpath,
    kind: 'file',
    label: basename(vpath),
    displayName: basename(vpath),
    vpath,
  }

  const modeToggle = diffable ? (
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
  ) : null

  const body = (() => {
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
  })()

  if (layout === 'inline') {
    return (
      <aside
        className="flex flex-col h-full min-h-0 border-l border-[var(--border)] bg-[var(--bg-surface,transparent)]"
        data-testid="artifact-inline-panel"
        aria-label={`Artifact viewer: ${basename(vpath)}`}
      >
        <div className="shrink-0 border-b border-[var(--border)] px-4 py-3">
          <div className="flex items-center gap-2">
            <span className="text-[13px] font-semibold tracking-[-0.02em] truncate flex-1 min-w-0">{basename(vpath)}</span>
            {modeToggle}
            <button
              type="button"
              onClick={onClose}
              aria-label="Close artifact viewer"
              className="shrink-0 flex items-center justify-center h-6 w-6 rounded-full border-none cursor-pointer bg-transparent text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] transition-colors"
            >
              <X size={14} />
            </button>
          </div>
          <div className="text-[11px] text-[var(--text-3)] truncate" style={{ fontFamily: 'monospace' }}>{vpath}</div>
        </div>
        <div className="flex-1 min-h-0 flex flex-col">{body}</div>
      </aside>
    )
  }

  return (
    <Sheet open onOpenChange={(open) => { if (!open) onClose() }}>
      <SheetContent
        side="right"
        className="w-screen sm:w-[min(720px,90vw)] sm:max-w-none p-0 gap-0 flex flex-col"
      >
        <SheetHeader className="shrink-0 border-b border-[var(--border)] px-4 py-3 space-y-0">
          <div className="flex items-center gap-2 pr-8">
            <SheetTitle className="text-[13px] font-semibold tracking-[-0.02em] truncate flex-1 min-w-0">
              {basename(vpath)}
            </SheetTitle>
            {modeToggle}
          </div>
          <SheetDescription className="text-[11px] text-[var(--text-3)] truncate" style={{ fontFamily: 'monospace' }}>
            {vpath}
          </SheetDescription>
        </SheetHeader>
        <div className="flex-1 min-h-0 flex flex-col">{body}</div>
      </SheetContent>
    </Sheet>
  )
}
