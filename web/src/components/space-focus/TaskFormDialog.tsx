import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { SpaceMember, Task } from '../../lib/types'
import { useCreateTask, useUpdateTask } from '../../hooks/useTasks'

interface TaskFormDialogProps {
  spaceId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  members: SpaceMember[]
  mode: 'create' | 'edit'
  task?: Task | null
}

/**
 * Create/edit form for a board task.
 *
 * Create writes via task.create (assignee required by the backend). Edit
 * writes via task.update, which is a descriptive-only edit — it cannot move a
 * task between columns or reassign it, so the assignee picker is create-only.
 */
export default function TaskFormDialog({
  spaceId,
  open,
  onOpenChange,
  members,
  mode,
  task,
}: TaskFormDialogProps) {
  const isEdit = mode === 'edit'
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [taskKind, setTaskKind] = useState('')
  const [assignedTo, setAssignedTo] = useState('')

  const createTask = useCreateTask(spaceId)
  const updateTask = useUpdateTask(spaceId)
  const pending = createTask.isPending || updateTask.isPending

  // Seed fields when the dialog opens; reset to blanks for create.
  useEffect(() => {
    if (!open) return
    if (isEdit && task) {
      setTitle(task.title ?? '')
      setDescription(task.description ?? '')
      setTaskKind(task.taskKind ?? '')
      setAssignedTo(task.assignedTo ?? '')
    } else {
      setTitle('')
      setDescription('')
      setTaskKind('')
      setAssignedTo('')
    }
  }, [open, isEdit, task])

  async function handleSubmit() {
    const trimmedDescription = description.trim()
    if (!trimmedDescription) {
      toast.error('Description is required')
      return
    }
    try {
      if (isEdit && task) {
        await updateTask.mutateAsync({
          taskId: task.id,
          title: title.trim(),
          description: trimmedDescription,
          taskKind: taskKind.trim(),
        })
        toast.success('Task updated')
      } else {
        if (!assignedTo) {
          toast.error('Assignee is required')
          return
        }
        await createTask.mutateAsync({
          assignedTo,
          description: trimmedDescription,
          title: title.trim() || undefined,
          taskKind: taskKind.trim() || undefined,
        })
        toast.success('Task created')
      }
      onOpenChange(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save task')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="dashboard-dialog-content sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>{isEdit ? 'Edit task' : 'New task'}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? "Update this task's title, description, or kind."
              : 'Create a task and assign it to a member of this space.'}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4 py-2">
          <div className="flex flex-col gap-2">
            <Label htmlFor="task-title">
              Title <span className="text-muted-foreground font-normal">(optional)</span>
            </Label>
            <Input
              id="task-title"
              placeholder="e.g. Write disconnect checklist"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              autoFocus
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="task-desc">Description</Label>
            <Textarea
              id="task-desc"
              placeholder="Describe what needs to be done…"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
              className="min-h-[100px] resize-none"
            />
          </div>
          {!isEdit && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="task-assignee">Assignee</Label>
              <Select value={assignedTo} onValueChange={setAssignedTo}>
                <SelectTrigger id="task-assignee" aria-label="Assignee">
                  <SelectValue placeholder="Select a member" />
                </SelectTrigger>
                <SelectContent>
                  {members.map((m) => (
                    <SelectItem key={m.id} value={m.id}>
                      {m.displayName || m.memberType || 'Member'}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
          <div className="flex flex-col gap-2">
            <Label htmlFor="task-kind">
              Kind <span className="text-muted-foreground font-normal">(optional)</span>
            </Label>
            <Input
              id="task-kind"
              placeholder="e.g. research, implementation"
              value={taskKind}
              onChange={(e) => setTaskKind(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={pending}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={pending || !description.trim() || (!isEdit && !assignedTo)}
          >
            {pending ? 'Saving…' : isEdit ? 'Save changes' : 'Create task'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
