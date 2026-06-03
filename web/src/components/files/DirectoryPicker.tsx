import { useCallback, useEffect, useRef, useState } from 'react'
import { ChevronRight, Folder, FolderOpen } from 'lucide-react'
import { Button } from '@/components/ui/button'

export interface DirectoryPickerEntry {
  name: string
  path: string
  isDir: boolean
}

export interface DirectoryPickerResult {
	path: string
	entries: DirectoryPickerEntry[]
}

export interface DirectoryPickerRoot {
	label: string
	path: string
}

export default function DirectoryPicker({
	loadDirectory,
	onSelect,
	emptyLabel = 'No subdirectories',
	initialPath = '',
	formatCurrentPath,
	formatEntryLabel,
	roots,
	autoSelectCurrentPath = false,
}: {
	loadDirectory: (path: string) => Promise<DirectoryPickerResult>
	onSelect: (path: string) => void
	emptyLabel?: string
	initialPath?: string
	formatCurrentPath?: (path: string) => string
	formatEntryLabel?: (entry: DirectoryPickerEntry) => string
	roots?: DirectoryPickerRoot[]
	autoSelectCurrentPath?: boolean
}) {
	const [currentPath, setCurrentPath] = useState('')
	const [entries, setEntries] = useState<DirectoryPickerEntry[]>([])
	const [loading, setLoading] = useState(false)
	const [error, setError] = useState<string | null>(null)
	const loadDirectoryRef = useRef(loadDirectory)
	const onSelectRef = useRef(onSelect)

	useEffect(() => {
		loadDirectoryRef.current = loadDirectory
		onSelectRef.current = onSelect
	}, [loadDirectory, onSelect])

	const loadDir = useCallback(async (path: string) => {
		setLoading(true)
		setError(null)
		try {
			const res = await loadDirectoryRef.current(path)
			setCurrentPath(res.path)
			setEntries(res.entries)
			if (autoSelectCurrentPath) onSelectRef.current(res.path)
		} catch (err) {
			setError(err instanceof Error ? err.message : 'Failed to read directory')
		} finally {
			setLoading(false)
		}
	}, [autoSelectCurrentPath])

  useEffect(() => {
    void loadDir(initialPath)
  }, [initialPath, loadDir])

  const parentPath = currentPath ? currentPath.split('/').slice(0, -1).join('/') || '/' : null
  const dirs = entries.filter((entry) => entry.isDir).sort((a, b) => a.name.localeCompare(b.name))
  const currentLabel = formatCurrentPath ? formatCurrentPath(currentPath) : (currentPath || '...')

  return (
	<div className="flex w-full min-w-0 max-w-full flex-col gap-2 overflow-hidden">
	  {roots && roots.length > 0 && (
		<div className="flex items-center gap-2 flex-wrap">
		  {roots.map((root) => {
			const active = currentPath === root.path || currentPath.startsWith(`${root.path}/`)
			return (
			  <Button
				key={root.path}
				variant="secondary"
				type="button"
				onClick={() => { void loadDir(root.path) }}
				className="font-medium"
				style={active ? { background: 'var(--bg-active)', borderColor: 'var(--accent)', color: 'var(--text-1)' } : undefined}
			  >
				{root.label}
			  </Button>
			)
		  })}
		</div>
	  )}

	  <div className="flex min-w-0 items-center gap-2">
		<div
		  className="min-w-0 flex-1 overflow-hidden rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-elevated)] px-3 py-2 font-[var(--font-mono,monospace)] text-xs text-[var(--text-1)]"
		  title={currentLabel}
		>
		  <span className="block truncate">{currentLabel}</span>
		</div>
		{!autoSelectCurrentPath && currentPath && (
		  <Button
			variant="secondary"
			type="button"
			onClick={() => onSelect(currentPath)}
			className="shrink-0 py-2 font-medium"
		  >
			Select folder
		  </Button>
		)}
	  </div>

      <div className="bg-[var(--bg-panel)] border border-[var(--border)] rounded-[var(--r-lg)] overflow-hidden max-h-[280px] overflow-y-auto">
        {loading ? (
          <div className="px-4 py-6 text-center">
            <span className="spinner spinner-sm" />
          </div>
        ) : error ? (
          <div className="px-4 py-3 text-xs text-[var(--red)]">{error}</div>
        ) : (
          <>
            {parentPath !== null && (
              <button
                type="button"
                onClick={() => { void loadDir(parentPath) }}
                className="flex items-center gap-2 w-full px-3 py-2 text-xs text-[var(--text-2)] border-none bg-transparent cursor-pointer font-[inherit] row-hover transition-[background] duration-100"
              >
                <FolderOpen size={13} className="text-[var(--amber)] shrink-0" />
                ..
              </button>
            )}
            {dirs.length === 0 && !parentPath && (
              <div className="px-4 py-3 text-xs text-[var(--text-3)]">{emptyLabel}</div>
            )}
            {dirs.map((entry) => (
              <button
                key={entry.path}
                type="button"
                onClick={() => { void loadDir(entry.path) }}
                className="flex items-center gap-2 w-full px-3 py-2 text-xs text-[var(--text-2)] border-none bg-transparent cursor-pointer font-[inherit] row-hover transition-[background] duration-100"
              >
                <Folder size={13} className="text-[var(--amber)] shrink-0" />
                <span className="truncate">{formatEntryLabel ? formatEntryLabel(entry) : entry.name}</span>
                <ChevronRight size={10} className="text-[var(--text-3)] opacity-50 shrink-0 ml-auto" />
              </button>
            ))}
          </>
        )}
      </div>
    </div>
  )
}
