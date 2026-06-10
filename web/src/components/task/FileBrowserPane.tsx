import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight, File as FileIcon, Folder, FolderOpen } from 'lucide-react'
import { rpcCall } from '../../lib/rpc'
import { basename } from '../files/filePreviewUtils'
import type { FilesListDirResult } from '../../lib/types'

/** Parent directory of a vpath, or the vpath itself when already at a root. */
function parentDir(path: string): string {
  const trimmed = path.replace(/\/+$/, '')
  const cut = trimmed.lastIndexOf('/')
  if (cut <= 0) return '/'
  return trimmed.slice(0, cut)
}

interface FileBrowserPaneProps {
  projectId: string | null
  /** Directory to open at — usually the folder containing the active artifact. */
  initialDir: string
  /** vpath currently shown in the viewer, highlighted in the list. */
  activeVPath: string
  /** Reports a file the user picked from the tree. */
  onSelectFile: (vpath: string) => void
}

/**
 * A directory navigator for the artifact viewer: lists one directory at a
 * time via files.listDir, navigates into folders, and reports file clicks to
 * the parent so the same viewer renders the chosen file. Location-agnostic —
 * remote (SSH) project trees list through the identical RPC path.
 */
export function FileBrowserPane({ projectId, initialDir, activeVPath, onSelectFile }: FileBrowserPaneProps) {
  const [dir, setDir] = useState(initialDir)

  const listQuery = useQuery<FilesListDirResult>({
    queryKey: ['files.listDir', projectId, dir],
    queryFn: async () =>
      rpcCall<FilesListDirResult>('files.listDir', {
        projectId: projectId ?? undefined,
        path: dir,
      }),
    enabled: !!projectId,
    retry: false,
    staleTime: 15_000,
  })

  const entries = listQuery.data?.entries ?? []
  // Folders first, then files; each group alphabetized.
  const sorted = [...entries].sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
    return a.name.localeCompare(b.name)
  })
  const atRoot = dir === '/' || dir === '/project' || dir === '/workspace'

  return (
    <div
      className="flex h-full min-h-0 w-[240px] shrink-0 flex-col border-r border-[var(--border)] bg-[var(--bg-surface,transparent)]"
      data-testid="file-browser-pane"
    >
      {/* current directory + up control */}
      <div className="flex shrink-0 items-center gap-1.5 border-b border-[var(--border)] px-3 py-2">
        <button
          type="button"
          onClick={() => setDir(parentDir(dir))}
          disabled={atRoot}
          aria-label="Up one directory"
          className="flex h-5 w-5 shrink-0 items-center justify-center rounded border-none bg-transparent text-[var(--text-3)] hover:text-[var(--text-1)] disabled:opacity-40 disabled:cursor-default cursor-pointer"
        >
          <ChevronRight size={13} className="rotate-180" />
        </button>
        <FolderOpen size={12} className="shrink-0 text-[var(--text-3)]" />
        <span className="truncate text-[11px] text-[var(--text-2)]" style={{ fontFamily: 'monospace' }} title={dir}>
          {basename(dir) || dir}
        </span>
      </div>

      <div className="flex-1 min-h-0 overflow-auto py-1">
        {listQuery.isLoading ? (
          <div className="flex items-center justify-center py-6">
            <span className="spinner spinner-sm" />
          </div>
        ) : listQuery.error ? (
          <div className="px-3 py-3 text-[11px] text-[var(--text-3)]">Could not list this directory.</div>
        ) : sorted.length === 0 ? (
          <div className="px-3 py-3 text-[11px] text-[var(--text-3)]">Empty directory.</div>
        ) : (
          sorted.map((entry) => {
            const isActive = !entry.isDir && entry.path === activeVPath
            return (
              <button
                key={entry.path}
                type="button"
                onClick={() => (entry.isDir ? setDir(entry.path) : onSelectFile(entry.path))}
                className="flex w-full items-center gap-1.5 border-none bg-transparent px-3 py-1 text-left cursor-pointer hover:bg-[var(--bg-hover)]"
                style={{ background: isActive ? 'var(--bg-elevated)' : undefined }}
                aria-current={isActive ? 'true' : undefined}
              >
                {entry.isDir ? (
                  <Folder size={12} className="shrink-0 text-[var(--text-3)]" />
                ) : (
                  <FileIcon size={12} className="shrink-0 text-[var(--text-3)]" />
                )}
                <span
                  className="truncate text-[12px]"
                  style={{ color: isActive ? 'var(--text-1)' : 'var(--text-2)' }}
                >
                  {entry.displayName || entry.name}
                </span>
              </button>
            )
          })
        )}
      </div>
    </div>
  )
}
