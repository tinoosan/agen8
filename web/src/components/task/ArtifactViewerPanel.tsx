import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { FolderTree, X } from 'lucide-react'
import { rpcCall } from '../../lib/rpc'
import { qk } from '../../lib/queryKeys'
import { basename } from '../files/filePreviewUtils'
import ArtifactViewer from '../files/ArtifactViewer'
import DiffView from '../files/DiffView'
import { FileBrowserPane } from './FileBrowserPane'
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
  /** Reason this location cannot produce a baseline at all (e.g. remote SSH). */
  unsupported?: string
}

/**
 * Why diff mode cannot show a diff for this file, or null when it can.
 * A non-null reason degrades the viewer to normal view with a notice banner
 * rather than a dead diff pane.
 */
function baselineUnavailableReason(baseline: FileBaselineResult | undefined, error: boolean): string | null {
  if (error) return 'Could not load the git baseline for this file; showing normal view.'
  if (!baseline) return null
  if (baseline.unsupported) return `${baseline.unsupported} Showing normal view.`
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
function dirOf(path: string): string {
  const trimmed = path.replace(/\/+$/, '')
  const cut = trimmed.lastIndexOf('/')
  return cut <= 0 ? '/' : trimmed.slice(0, cut)
}

export function ArtifactViewerPanel({ projectId, vpath, onClose, layout }: ArtifactViewerPanelProps) {
  const [diffMode, setDiffMode] = useState(false)
  const [browsing, setBrowsing] = useState(false)
  // The browser can swap which file the viewer shows without remounting the
  // panel, so the shown file is internal state seeded from the opened artifact.
  const [activeVPath, setActiveVPath] = useState(vpath)

  const selectFromBrowser = (next: string) => {
    setActiveVPath(next)
    setDiffMode(false) // a freshly picked file starts in normal view
  }

  const previewQuery = useQuery<ArtifactGetResult>({
    queryKey: qk.filePreview(projectId, null, activeVPath),
    queryFn: async () =>
      rpcCall<ArtifactGetResult>('files.get', {
        projectId: projectId ?? undefined,
        path: activeVPath,
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
    queryKey: ['files.baseline', projectId, activeVPath],
    queryFn: async () =>
      rpcCall<FileBaselineResult>('files.baseline', {
        projectId: projectId ?? undefined,
        path: activeVPath,
      }),
    enabled: !!projectId && diffMode && diffable,
    retry: false,
    staleTime: 30_000,
  })

  const viewerFile: ArtifactNode = {
    nodeKey: 'file:' + activeVPath,
    kind: 'file',
    label: basename(activeVPath),
    displayName: basename(activeVPath),
    vpath: activeVPath,
  }

  const browseToggle = (
    <button
      type="button"
      onClick={() => setBrowsing((b) => !b)}
      aria-pressed={browsing}
      aria-label="Browse files"
      title="Browse files in this project"
      className="flex h-6 w-6 shrink-0 items-center justify-center rounded-[var(--r-md)] border cursor-pointer transition-colors"
      style={{
        borderColor: 'var(--border)',
        background: browsing ? 'var(--bg-elevated)' : 'transparent',
        color: browsing ? 'var(--text-1)' : 'var(--text-3)',
      }}
    >
      <FolderTree size={13} />
    </button>
  )

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
        return <DiffView baseline={baselineQuery.data?.content ?? ''} current={previewQuery.data?.content ?? ''} filePath={activeVPath} />
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

  // With the browser open, the file tree sits to the left of the viewer body.
  const bodyWithBrowser = browsing ? (
    <div className="flex h-full min-h-0">
      <FileBrowserPane
        projectId={projectId}
        initialDir={dirOf(activeVPath)}
        activeVPath={activeVPath}
        onSelectFile={selectFromBrowser}
      />
      <div className="flex-1 min-h-0 flex flex-col">{body}</div>
    </div>
  ) : (
    body
  )

  if (layout === 'inline') {
    return (
      <aside
        className="flex flex-col h-full min-h-0 border-l border-[var(--border)] bg-[var(--bg-surface,transparent)]"
        data-testid="artifact-inline-panel"
        aria-label={`Artifact viewer: ${basename(activeVPath)}`}
      >
        <div className="shrink-0 border-b border-[var(--border)] px-4 py-3">
          <div className="flex items-center gap-2">
            <span className="text-[13px] font-semibold tracking-[-0.02em] truncate flex-1 min-w-0">{basename(activeVPath)}</span>
            {browseToggle}
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
          <div className="text-[11px] text-[var(--text-3)] truncate" style={{ fontFamily: 'monospace' }}>{activeVPath}</div>
        </div>
        <div className="flex-1 min-h-0 flex flex-col">{bodyWithBrowser}</div>
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
              {basename(activeVPath)}
            </SheetTitle>
            {browseToggle}
            {modeToggle}
          </div>
          <SheetDescription className="text-[11px] text-[var(--text-3)] truncate" style={{ fontFamily: 'monospace' }}>
            {activeVPath}
          </SheetDescription>
        </SheetHeader>
        <div className="flex-1 min-h-0 flex flex-col">{bodyWithBrowser}</div>
      </SheetContent>
    </Sheet>
  )
}
