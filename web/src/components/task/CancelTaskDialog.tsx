import { useState } from 'react'
import { toast } from 'sonner'
import { useCancelTask } from '../../hooks/useProjectTasks'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import type { Task } from '../../lib/types'

/* ── Cancel-task confirmation dialog ── */

export function CancelTaskDialog({
  task,
  open,
  onOpenChange,
}: {
  task: Task
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [reason, setReason] = useState('')
  const cancelTask = useCancelTask()

  async function handleCancel() {
    const trimmed = reason.trim()
    if (!trimmed) {
      toast.error('A reason is required to cancel a task')
      return
    }
    try {
      await cancelTask.mutateAsync({ taskId: task.id, reason: trimmed })
      toast.success('Task canceled')
      setReason('')
      onOpenChange(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to cancel task')
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Cancel task</AlertDialogTitle>
          <AlertDialogDescription>
            This moves the task to a canceled state. The reason is recorded on the task and shared
            with whoever was assigned. This can't be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="flex flex-col gap-2 py-1">
          <Label htmlFor="cancel-task-reason" className="dashboard-mission-dialog-label">Reason</Label>
          <Textarea
            id="cancel-task-reason"
            placeholder="Why is this task being canceled?"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            rows={3}
            autoFocus
            className="dashboard-mission-dialog-field min-h-[88px] resize-none"
          />
        </div>
        <AlertDialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            className="dashboard-action-button dashboard-action-button-neutral border-0"
          >
            Keep task
          </Button>
          <Button
            onClick={handleCancel}
            disabled={cancelTask.isPending || !reason.trim()}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {cancelTask.isPending ? 'Canceling…' : 'Cancel task'}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
