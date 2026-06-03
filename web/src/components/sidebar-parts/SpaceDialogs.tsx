/**
 * SpaceDialogs — confirmation and form dialogs used by the space list.
 * Extracted so SpaceList doesn't embed 100+ lines of dialog markup.
 */
import React, { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { normalizeSpaceTitle } from '../../lib/spaceHelpers'
import { useSpaceUpdate, useSpaceDelete, useSpaceMemberRemove } from '../../hooks/useSpace'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

/* ── Rename Dialog ─────────────────────────────────── */

interface RenameDialogProps {
  open: boolean
  spaceId: string | null
  initialName: string
  onClose: () => void
  onRenamed?: () => void
}

export function SpaceRenameDialog({ open, spaceId, initialName, onClose, onRenamed }: RenameDialogProps) {
  const renameMutation = useSpaceUpdate()
  const [input, setInput] = useState(initialName)
  const normalized = useMemo(() => normalizeSpaceTitle(input), [input])

  // Reset input when dialog opens with new values
  React.useEffect(() => {
    if (open) setInput(initialName)
  }, [open, initialName])

  const handleConfirm = () => {
    if (!spaceId) return
    if (!normalized) {
      toast.error('Space name must include letters or numbers')
      return
    }
    renameMutation.mutate(
      { spaceId, title: normalized },
      {
        onSuccess: () => {
          toast.success('Space name updated')
          onRenamed?.()
          onClose()
        },
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to rename space'),
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onClose() }}>
      <DialogContent data-tour-dialog="space" className="max-w-[min(92vw,420px)] border-[var(--border)] bg-[var(--bg-panel)] text-[var(--text-1)]">
        <DialogHeader>
          <DialogTitle className="text-[15px] tracking-[-0.01em]">Rename space</DialogTitle>
          <DialogDescription className="text-[12px] text-[var(--text-3)]">
            Enter a space name. We automatically convert it into a stable identifier.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => { e.preventDefault(); handleConfirm() }}
        >
          <div className="space-y-1.5">
            <label htmlFor="rename-space-name" className="text-[11px] font-semibold text-[var(--text-2)]">
              Space name
            </label>
            <Input
              id="rename-space-name"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="Market research TNXP"
              autoFocus
              disabled={renameMutation.isPending}
            />
          </div>
          <div className="rounded-[8px] border border-[var(--border)] bg-[var(--bg-surface)] px-2.5 py-2 text-[11px] text-[var(--text-3)]">
            Identifier preview: <span className="font-mono text-[var(--text-2)]">{normalized || 'invalid-name'}</span>
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose} disabled={renameMutation.isPending}>
              Cancel
            </Button>
            <Button type="submit" disabled={renameMutation.isPending || !normalized}>
              {renameMutation.isPending ? 'Saving…' : 'Save name'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/* ── Delete Confirmation ───────────────────────────── */

interface DeleteDialogProps {
  open: boolean
  spaceId: string | null
  onClose: () => void
  onDeleted?: (spaceId: string) => void
}

export function SpaceDeleteDialog({ open, spaceId, onClose, onDeleted }: DeleteDialogProps) {
  const deleteMutation = useSpaceDelete()

  const handleConfirm = () => {
    if (!spaceId) return
    const id = spaceId
    onClose()
    deleteMutation.mutate(
      { spaceId: id },
      {
        onSuccess: () => {
          toast.success('Space deleted')
          onDeleted?.(id)
        },
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to delete space'),
      },
    )
  }

  return (
    <AlertDialog open={open} onOpenChange={(v) => { if (!v) onClose() }}>
      <AlertDialogContent className="max-w-[min(92vw,420px)] border-[var(--border)] bg-[var(--bg-panel)] text-[var(--text-1)] shadow-[var(--shadow-lg)]">
        <AlertDialogHeader>
          <AlertDialogTitle>Delete space?</AlertDialogTitle>
          <AlertDialogDescription>
            This will permanently delete the space and all its conversation history. This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={handleConfirm}
            className="bg-[var(--red)] hover:bg-[var(--red)]/90 text-white"
          >
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

/* ── Member Remove Confirmation ────────────────────── */

interface MemberRemoveDialogProps {
  open: boolean
  spaceId: string | null
  memberId: string | null
  displayName: string
  onClose: () => void
}

export function MemberRemoveDialog({ open, spaceId, memberId, displayName, onClose }: MemberRemoveDialogProps) {
  const removeMutation = useSpaceMemberRemove()

  const handleConfirm = () => {
    if (!memberId || !spaceId) return
    const target = { memberId, spaceId, displayName }
    onClose()
    removeMutation.mutate(
      { memberId: target.memberId, spaceId: target.spaceId },
      {
        onSuccess: () => toast.success(`Removed ${target.displayName}`),
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to remove member'),
      },
    )
  }

  return (
    <AlertDialog open={open} onOpenChange={(v) => { if (!v) onClose() }}>
      <AlertDialogContent className="max-w-[min(92vw,420px)] border-[var(--border)] bg-[var(--bg-panel)] text-[var(--text-1)] shadow-[var(--shadow-lg)]">
        <AlertDialogHeader>
          <AlertDialogTitle>Remove {displayName || 'member'}?</AlertDialogTitle>
          <AlertDialogDescription>
            This will remove the member from the space and permanently delete
            their channel and conversation history. This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={handleConfirm}
            className="bg-[var(--red)] hover:bg-[var(--red)]/90 text-white"
            data-testid="confirm-remove-member"
          >
            Remove
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
