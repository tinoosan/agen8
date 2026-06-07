import { useState } from 'react'
import { toast } from 'sonner'
import { rpcCall } from '../../lib/rpc'
import { projectDisplayName } from '@/lib/projectHelpers'
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
import type { Project } from '../../lib/types'

/* ── Project removal action ───────────────────────────────────
   The "remove" concept (archive vs. delete) lives here, alongside
   the dialog that performs it. The list page and table row import
   this type to describe a pending removal. */
export type ProjectRemoveAction = 'archive' | 'delete'

export default function RemoveProjectDialog({
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
