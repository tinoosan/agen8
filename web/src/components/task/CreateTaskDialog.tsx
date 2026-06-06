import { useState } from 'react'
import { toast } from 'sonner'
import { Plus, X } from 'lucide-react'
import { useCreateTask } from '../../hooks/useProjectTasks'
import { useProjectMembers } from '../../hooks/useProjectMembers'
import { memberDisplayName } from '../../lib/memberDisplay'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'

interface CreateTaskDialogProps {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export default function CreateTaskDialog({ projectId, open, onOpenChange }: CreateTaskDialogProps) {
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [assignedTo, setAssignedTo] = useState('')
  const [taskKind, setTaskKind] = useState('')
  const [criteria, setCriteria] = useState<string[]>([])
  const createTask = useCreateTask()
  const { data: members, isLoading: membersLoading } = useProjectMembers(open ? projectId : null)

  const hasMembers = !!members && members.length > 0

  function handleClose() {
    setTitle('')
    setDescription('')
    setAssignedTo('')
    setTaskKind('')
    setCriteria([])
    onOpenChange(false)
  }

  function addCriterion() {
    setCriteria((prev) => [...prev, ''])
  }

  function updateCriterion(index: number, value: string) {
    setCriteria((prev) => prev.map((c, i) => (i === index ? value : c)))
  }

  function removeCriterion(index: number) {
    setCriteria((prev) => prev.filter((_, i) => i !== index))
  }

  async function handleCreate() {
    const trimmedDescription = description.trim()
    if (!trimmedDescription) {
      toast.error('Task description is required')
      return
    }
    if (!assignedTo) {
      toast.error('Assign the task to a project member')
      return
    }

    const acceptanceCriteria = criteria.map((c) => c.trim()).filter(Boolean)

    try {
      await createTask.mutateAsync({
        projectId,
        assignedTo,
        description: trimmedDescription,
        title: title.trim() || undefined,
        taskKind: taskKind.trim() || undefined,
        acceptanceCriteria: acceptanceCriteria.length ? acceptanceCriteria : undefined,
      })
      toast.success('Task created')
      handleClose()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create task')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="dashboard-dialog-content dashboard-mission-dialog sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle className="dashboard-mission-dialog-title">New Task</DialogTitle>
          <DialogDescription className="dashboard-mission-dialog-copy">
            Create a task and assign it to a project member. Acceptance criteria are optional.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4 py-2">
          <div className="flex flex-col gap-2">
            <Label htmlFor="task-title" className="dashboard-mission-dialog-label">
              Title <span className="text-muted-foreground font-normal">(optional)</span>
            </Label>
            <Input
              id="task-title"
              placeholder="e.g. Wire up the export endpoint"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              autoFocus
              className="dashboard-mission-dialog-field"
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="task-desc" className="dashboard-mission-dialog-label">Description</Label>
            <Textarea
              id="task-desc"
              placeholder="Describe what needs to be done and why..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
              className="dashboard-mission-dialog-field min-h-[118px] resize-none"
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="task-assignee" className="dashboard-mission-dialog-label">Assignee</Label>
            <Select value={assignedTo} onValueChange={setAssignedTo} disabled={!hasMembers}>
              <SelectTrigger id="task-assignee" className="dashboard-mission-dialog-field">
                <SelectValue
                  placeholder={
                    membersLoading
                      ? 'Loading members…'
                      : hasMembers
                        ? 'Select a project member'
                        : 'No members available'
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {(members ?? []).map((member) => (
                  <SelectItem key={member.id} value={member.id}>
                    {memberDisplayName(member.displayName, member.id) ?? member.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {!membersLoading && !hasMembers && (
              <p className="text-[0.75rem] text-[var(--text-3)]" style={{ letterSpacing: '-0.08px' }}>
                Register a member for this project before creating tasks.
              </p>
            )}
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="task-kind" className="dashboard-mission-dialog-label">
              Kind <span className="text-muted-foreground font-normal">(optional)</span>
            </Label>
            <Input
              id="task-kind"
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
                {criteria.map((value, index) => (
                  <div key={index} className="flex items-center gap-2">
                    <Input
                      value={value}
                      onChange={(e) => updateCriterion(index, e.target.value)}
                      placeholder={`Criterion ${index + 1}`}
                      className="dashboard-mission-dialog-field flex-1"
                    />
                    <button
                      type="button"
                      onClick={() => removeCriterion(index)}
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
          <Button variant="outline" onClick={handleClose} className="dashboard-action-button dashboard-action-button-neutral border-0">
            Cancel
          </Button>
          <Button
            variant="outline"
            onClick={handleCreate}
            disabled={createTask.isPending || !description.trim() || !assignedTo}
            className="dashboard-action-button dashboard-action-button-accent border-0"
          >
            {createTask.isPending ? 'Creating...' : 'Create Task'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
