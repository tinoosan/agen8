import { useState } from 'react'
import { FolderCog } from 'lucide-react'
import type { Project } from '../../lib/types'
import { rpcCall } from '../../lib/rpc'
import { relocateProject } from '../../lib/projectClient'
import DirectoryPicker, { type DirectoryPickerResult } from '../files/DirectoryPicker'
import { Button } from '@/components/ui/button'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'

export default function RelocateProjectDialog({
  project,
  onClose,
  onRelocated,
}: {
  project: Project
  onClose: () => void
  onRelocated: (project: Project) => void
}) {
  const [selectedRoot, setSelectedRoot] = useState(project.root)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const save = async () => {
    setBusy(true)
    setError(null)
    try {
      onRelocated(await relocateProject(project.id, selectedRoot))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not change the project folder')
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !busy) onClose() }}>
      <DialogContent className="max-h-[calc(100vh-2rem)] max-w-[min(92vw,560px)] overflow-hidden rounded-[var(--r-lg)] border-[var(--border)] bg-[var(--bg-panel)] p-0">
        <DialogHeader className="border-b border-[var(--border)] px-5 py-4 text-left">
          <DialogTitle className="flex items-center gap-2 text-[0.9375rem]">
            <FolderCog size={15} className="text-[var(--accent)]" />
            Change project folder
          </DialogTitle>
          <DialogDescription className="text-[0.75rem] leading-5 text-[var(--text-3)]">
            Choose the folder after moving or renaming it. Agen8 updates the existing project record and does not move files.
          </DialogDescription>
        </DialogHeader>
        <div className="min-h-0 overflow-y-auto px-5 py-4">
          <DirectoryPicker
            initialPath={project.root}
            onSelect={setSelectedRoot}
            loadDirectory={async (path) => {
              const result = await rpcCall<{ entries: Array<{ name: string; path: string; type: string }> }>(
                'location.fs.listDir',
                { locationId: project.locationId, path },
              )
              return {
                path,
                entries: (result.entries ?? []).map((entry) => ({
                  name: entry.name,
                  path: entry.path,
                  isDir: entry.type === 'directory',
                })),
              } satisfies DirectoryPickerResult
            }}
          />
          <div className="mt-3 break-all font-mono text-[0.6875rem] text-[var(--text-3)]">
            Selected: {selectedRoot}
          </div>
          {error && <div className="mt-2 text-[0.75rem] text-[var(--red)]">{error}</div>}
        </div>
        <DialogFooter className="flex-row justify-end gap-2 border-t border-[var(--border)] px-5 py-3">
          <Button type="button" variant="ghost" onClick={onClose} disabled={busy}>Cancel</Button>
          <Button type="button" onClick={() => { void save() }} disabled={busy || selectedRoot === project.root}>
            {busy ? 'Updating...' : 'Use this folder'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
