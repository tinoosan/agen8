import { useState } from 'react'
import { Pencil } from 'lucide-react'
import type { Project } from '../../lib/types'
import { updateProject } from '../../lib/projectClient'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'

/* ── Rename a project ─────────────────────────────────────────
   A project's identity (id, root) is fixed, but its display name
   is editable. The name field is pre-filled with the current
   title; the folder name is shown as the placeholder so clearing
   the field falls back to it rather than leaving the project
   nameless. */

export default function EditProjectDialog({
  project,
  onClose,
  onSaved,
}: {
  project: Project
  onClose: () => void
  onSaved: (project: Project) => void
}) {
  const folderName = project.root.split('/').filter(Boolean).at(-1) || project.id
  const [name, setName] = useState(project.title ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSave() {
    setBusy(true)
    setError(null)
    try {
      // Always send title (even ''): clearing the name is a real edit that
      // drops the custom title so the folder name takes over.
      const updated = await updateProject(project.id, { title: name.trim() })
      onSaved(updated)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to rename project')
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !busy) onClose() }}>
      <DialogContent className="max-w-[min(92vw,440px)] rounded-[var(--r-xl)] border-[var(--border)] bg-[var(--bg-panel)] p-0 shadow-[var(--shadow-lg)] gap-0">
        <DialogHeader className="border-b border-[var(--border)] px-5 pt-5 pb-3">
          <DialogTitle className="flex items-center gap-2 text-[0.9375rem] font-semibold text-[var(--text-1)]">
            <Pencil size={14} className="text-[var(--accent)]" />
            Rename project
          </DialogTitle>
          <DialogDescription className="text-[0.75rem] text-[var(--text-3)]">
            Change the display name for{' '}
            <span className="font-mono text-[var(--text-2)]">{folderName}</span>.
            This does not move or rename the folder on disk.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4 px-5 py-4">
          <div>
            <label className="mb-2 block text-[0.6875rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-3)]">
              Display name
            </label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={folderName}
              className="h-9 text-[0.8125rem]"
              autoFocus
              onKeyDown={(e) => { if (e.key === 'Enter' && !busy) handleSave() }}
            />
            <p className="mt-1.5 text-[0.6875rem] text-[var(--text-4)]">
              Leave blank to use the folder name.
            </p>
          </div>
          {error && <div className="text-[0.75rem] text-[var(--red)]">{error}</div>}
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-[var(--border)] px-5 py-3">
          <Button variant="ghost" size="sm" onClick={onClose} disabled={busy} className="text-[var(--text-3)]">
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={handleSave}
            disabled={busy || name.trim() === (project.title ?? '').trim()}
            className="gap-1.5"
          >
            {busy ? 'Saving…' : 'Save'}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
