import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { useUpdateMission } from '../../hooks/useMissions'
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
import { format, parseISO } from 'date-fns'
import { MissionDateField } from './CreateMissionDialog'
import type { MissionView } from '../../lib/types'

interface EditMissionDialogProps {
  mission: MissionView
  open: boolean
  onOpenChange: (open: boolean) => void
}

// A date string the calendar/forms understand: 'yyyy-MM-dd'. Mission dates come
// back as ISO datetimes, so trim to the day.
function toDayString(value?: string): string {
  return value ? value.slice(0, 10) : ''
}

export default function EditMissionDialog({ mission, open, onOpenChange }: EditMissionDialogProps) {
  const [title, setTitle] = useState(mission.title)
  const [description, setDescription] = useState(mission.description ?? '')
  const [startDate, setStartDate] = useState(toDayString(mission.startDate))
  const [endDate, setEndDate] = useState(toDayString(mission.endDate))
  const [startCalendarMonth, setStartCalendarMonth] = useState<Date>(() => new Date())
  const [endCalendarMonth, setEndCalendarMonth] = useState<Date>(() => new Date())
  const updateMission = useUpdateMission()

  // Re-sync the form to the mission whenever the dialog opens, so a cancelled
  // edit never leaks stale values into the next open.
  useEffect(() => {
    if (!open) return
    const start = toDayString(mission.startDate)
    const end = toDayString(mission.endDate)
    setTitle(mission.title)
    setDescription(mission.description ?? '')
    setStartDate(start)
    setEndDate(end)
    setStartCalendarMonth(start ? parseISO(start) : new Date())
    setEndCalendarMonth(end ? parseISO(end) : new Date())
  }, [open, mission])

  const startDateValue = useMemo(() => (startDate ? parseISO(startDate) : undefined), [startDate])
  const endDateValue = useMemo(() => (endDate ? parseISO(endDate) : undefined), [endDate])

  async function handleSave() {
    const trimmedTitle = title.trim()
    if (!trimmedTitle) {
      toast.error('Mission title is required')
      return
    }

    try {
      await updateMission.mutateAsync({
        missionId: mission.id,
        title: trimmedTitle,
        description: description.trim(),
        startDate: startDate || undefined,
        endDate: endDate || undefined,
      })
      toast.success('Mission updated')
      onOpenChange(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update mission')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="dashboard-dialog-content dashboard-mission-dialog max-h-[85vh] overflow-y-auto sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle className="dashboard-mission-dialog-title">Edit mission</DialogTitle>
          <DialogDescription className="dashboard-mission-dialog-copy">
            Update the mission's title, description, and dates.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4 py-2">
          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-mission-title" className="dashboard-mission-dialog-label">Title</Label>
            <Input
              id="edit-mission-title"
              placeholder="e.g. Ship v2.0 by Q2"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  handleSave()
                }
              }}
              autoFocus
              className="dashboard-mission-dialog-field"
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-mission-desc" className="dashboard-mission-dialog-label">Description <span className="text-muted-foreground font-normal">(optional)</span></Label>
            <Textarea
              id="edit-mission-desc"
              placeholder="Describe the mission's objective and success criteria..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
              className="dashboard-mission-dialog-field min-h-[118px] resize-none"
            />
          </div>
          <div className="flex flex-col gap-3 sm:flex-row">
            <div className="flex flex-col gap-2 flex-1">
              <Label htmlFor="edit-mission-start" className="dashboard-mission-dialog-label">Start date <span className="text-muted-foreground font-normal">(optional)</span></Label>
              <MissionDateField
                id="edit-mission-start"
                label="Start date"
                value={startDateValue}
                month={startCalendarMonth}
                onMonthChange={setStartCalendarMonth}
                onSelect={(date) => setStartDate(date ? format(date, 'yyyy-MM-dd') : '')}
              />
            </div>
            <div className="flex flex-col gap-2 flex-1">
              <Label htmlFor="edit-mission-end" className="dashboard-mission-dialog-label">End date <span className="text-muted-foreground font-normal">(optional)</span></Label>
              <MissionDateField
                id="edit-mission-end"
                label="End date"
                value={endDateValue}
                month={endCalendarMonth}
                onMonthChange={setEndCalendarMonth}
                onSelect={(date) => setEndDate(date ? format(date, 'yyyy-MM-dd') : '')}
              />
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="dashboard-action-button dashboard-action-button-neutral border-0">
            Cancel
          </Button>
          <Button
            variant="outline"
            onClick={handleSave}
            disabled={updateMission.isPending || !title.trim()}
            className="dashboard-action-button dashboard-action-button-accent border-0"
          >
            {updateMission.isPending ? 'Saving...' : 'Save changes'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
