import { useState } from 'react'
import { toast } from 'sonner'
import { Plus, X } from 'lucide-react'
import { useUpdateTask } from '../../hooks/useProjectTasks'
import type { Task } from '../../lib/types'
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
import { Checkbox } from '@/components/ui/checkbox'

interface EditTaskDialogProps {
  task: Task
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface CriterionRow {
  id: string
  text: string
  satisfied: boolean
}

function newCriterionId(): string {
  const rand = typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : Math.random().toString(36).slice(2)
  return `criterion-${rand}`
}

export default function EditTaskDialog({ task, open, onOpenChange }: EditTaskDialogProps) {
  const resetKey = `${open ? 'open' : 'closed'}:${task.id}:${task.updatedAt ?? ''}`

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <EditTaskDialogForm key={resetKey} task={task} onOpenChange={onOpenChange} />
    </Dialog>
  )
}

function EditTaskDialogForm({ task, onOpenChange }: Omit<EditTaskDialogProps, 'open'>) {
  const [title, setTitle] = useState(task.title ?? '')
  const [description, setDescription] = useState(task.description ?? '')
  const [taskKind, setTaskKind] = useState(task.taskKind ?? '')
  const [criteria, setCriteria] = useState<CriterionRow[]>(
    (task.acceptanceCriteria ?? []).map((c) => ({ id: c.id, text: c.text, satisfied: c.satisfied })),
  )
  const updateTask = useUpdateTask()

  function addCriterion() {
    setCriteria((prev) => [...prev, { id: newCriterionId(), text: '', satisfied: false }])
  }

  function updateCriterionText(id: string, text: string) {
    setCriteria((prev) => prev.map((c) => (c.id === id ? { ...c, text } : c)))
  }

  function toggleCriterion(id: string, satisfied: boolean) {
    setCriteria((prev) => prev.map((c) => (c.id === id ? { ...c, satisfied } : c)))
  }

  function removeCriterion(id: string) {
    setCriteria((prev) => prev.filter((c) => c.id !== id))
  }

  async function handleSave() {
    const trimmedDescription = description.trim()
    if (!trimmedDescription) {
      toast.error('Task description is required')
      return
    }

    const acceptanceCriteria = criteria
      .map((c) => ({ id: c.id, text: c.text.trim(), satisfied: c.satisfied }))
      .filter((c) => c.text)

    try {
      await updateTask.mutateAsync({
        taskId: task.id,
        title: title.trim(),
        description: trimmedDescription,
        taskKind: taskKind.trim(),
        acceptanceCriteria,
      })
      toast.success('Task updated')
      onOpenChange(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update task')
    }
  }

  return (
    <DialogContent className="dashboard-dialog-content dashboard-mission-dialog sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle className="dashboard-mission-dialog-title">Edit Task</DialogTitle>
          <DialogDescription className="dashboard-mission-dialog-copy">
            Update the task details and acceptance criteria. The assignee is fixed at creation.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4 py-2">
          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-task-title" className="dashboard-mission-dialog-label">
              Title <span className="text-muted-foreground font-normal">(optional)</span>
            </Label>
            <Input
              id="edit-task-title"
              placeholder="e.g. Wire up the export endpoint"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              autoFocus
              className="dashboard-mission-dialog-field"
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-task-desc" className="dashboard-mission-dialog-label">Description</Label>
            <Textarea
              id="edit-task-desc"
              placeholder="Describe what needs to be done and why..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
              className="dashboard-mission-dialog-field min-h-[118px] resize-none"
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-task-kind" className="dashboard-mission-dialog-label">
              Kind <span className="text-muted-foreground font-normal">(optional)</span>
            </Label>
            <Input
              id="edit-task-kind"
              placeholder="e.g. feature, bugfix, research"
              value={taskKind}
              onChange={(e) => setTaskKind(e.target.value)}
              className="dashboard-mission-dialog-field"
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label className="dashboard-mission-dialog-label">
              Acceptance criteria <span className="text-muted-foreground font-normal">(optional)</span>
            </Label>
            {criteria.length > 0 && (
              <div className="flex flex-col gap-2">
                {criteria.map((row, index) => (
                  <div key={row.id} className="flex items-center gap-2">
                    <Checkbox
                      checked={row.satisfied}
                      onCheckedChange={(value) => toggleCriterion(row.id, value === true)}
                      aria-label="Mark criterion satisfied"
                      className="shrink-0"
                    />
                    <Input
                      value={row.text}
                      onChange={(e) => updateCriterionText(row.id, e.target.value)}
                      placeholder={`Criterion ${index + 1}`}
                      className="dashboard-mission-dialog-field flex-1"
                    />
                    <button
                      type="button"
                      onClick={() => removeCriterion(row.id)}
                      aria-label="Remove criterion"
                      className="shrink-0 flex items-center justify-center h-7 w-7 rounded-full border-none cursor-pointer bg-transparent text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] transition-colors"
                    >
                      <X size={14} />
                    </button>
                  </div>
                ))}
              </div>
            )}
            <button
              type="button"
              onClick={addCriterion}
              className="inline-flex items-center gap-1.5 self-start border-none cursor-pointer bg-transparent text-[var(--text-2)] hover:text-[var(--text-1)] transition-colors"
              style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}
            >
              <Plus size={13} />
              Add acceptance criterion
            </button>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="dashboard-action-button dashboard-action-button-neutral border-0">
            Cancel
          </Button>
          <Button
            variant="outline"
            onClick={handleSave}
            disabled={updateTask.isPending || !description.trim()}
            className="dashboard-action-button dashboard-action-button-accent border-0"
          >
            {updateTask.isPending ? 'Saving...' : 'Save Changes'}
          </Button>
        </DialogFooter>
    </DialogContent>
  )
}
