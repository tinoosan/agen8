import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { useCreateMission } from '../../hooks/useMissions'
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
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { CalendarDays, ChevronLeft, ChevronRight } from 'lucide-react'
import {
  addMonths,
  eachDayOfInterval,
  endOfMonth,
  endOfWeek,
  format,
  isSameDay,
  isSameMonth,
  isToday,
  parseISO,
  startOfMonth,
  startOfWeek,
} from 'date-fns'

interface CreateMissionDialogProps {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export default function CreateMissionDialog({ projectId, open, onOpenChange }: CreateMissionDialogProps) {
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [startDate, setStartDate] = useState('')
  const [endDate, setEndDate] = useState('')
  const [startCalendarMonth, setStartCalendarMonth] = useState<Date>(() => new Date())
  const [endCalendarMonth, setEndCalendarMonth] = useState<Date>(() => new Date())
  const createMission = useCreateMission()

  const startDateValue = useMemo(() => (startDate ? parseISO(startDate) : undefined), [startDate])
  const endDateValue = useMemo(() => (endDate ? parseISO(endDate) : undefined), [endDate])

  function handleClose() {
    setTitle('')
    setDescription('')
    setStartDate('')
    setEndDate('')
    const today = new Date()
    setStartCalendarMonth(today)
    setEndCalendarMonth(today)
    onOpenChange(false)
  }

  async function handleCreate() {
    const trimmedTitle = title.trim()
    if (!trimmedTitle) {
      toast.error('Mission title is required')
      return
    }

    try {
      await createMission.mutateAsync({
        projectId,
        title: trimmedTitle,
        description: description.trim() || undefined,
        startDate: startDate || undefined,
        endDate: endDate || undefined,
      })
      toast.success('Mission created')
      handleClose()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create mission')
    }
  }

  function handleStartDateSelect(value?: string) {
    setStartDate(value ?? '')
    if (value) setStartCalendarMonth(parseISO(value))
  }

  function handleEndDateSelect(value?: string) {
    setEndDate(value ?? '')
    if (value) setEndCalendarMonth(parseISO(value))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="dashboard-dialog-content dashboard-mission-dialog sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle className="dashboard-mission-dialog-title">New Mission</DialogTitle>
          <DialogDescription className="dashboard-mission-dialog-copy">
            Define a mission objective for your project. You can add key results after creation.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4 py-2">
          <div className="flex flex-col gap-2">
            <Label htmlFor="mission-title" className="dashboard-mission-dialog-label">Title</Label>
            <Input
              id="mission-title"
              placeholder="e.g. Ship v2.0 by Q2"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  handleCreate()
                }
              }}
              autoFocus
              className="dashboard-mission-dialog-field"
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="mission-desc" className="dashboard-mission-dialog-label">Description <span className="text-muted-foreground font-normal">(optional)</span></Label>
            <Textarea
              id="mission-desc"
              placeholder="Describe the mission's objective and success criteria..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
              className="dashboard-mission-dialog-field min-h-[118px] resize-none"
            />
          </div>
          <div className="flex gap-3">
            <div className="flex flex-col gap-2 flex-1">
              <Label htmlFor="mission-start" className="dashboard-mission-dialog-label">Start date <span className="text-muted-foreground font-normal">(optional)</span></Label>
              <MissionDateField
                id="mission-start"
                label="Start date"
                value={startDateValue}
                month={startCalendarMonth}
                onMonthChange={setStartCalendarMonth}
                onSelect={(date) => handleStartDateSelect(date ? format(date, 'yyyy-MM-dd') : undefined)}
              />
            </div>
            <div className="flex flex-col gap-2 flex-1">
              <Label htmlFor="mission-end" className="dashboard-mission-dialog-label">End date <span className="text-muted-foreground font-normal">(optional)</span></Label>
              <MissionDateField
                id="mission-end"
                label="End date"
                value={endDateValue}
                month={endCalendarMonth}
                onMonthChange={setEndCalendarMonth}
                onSelect={(date) => handleEndDateSelect(date ? format(date, 'yyyy-MM-dd') : undefined)}
              />
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={handleClose} className="dashboard-action-button dashboard-action-button-neutral border-0">
            Cancel
          </Button>
          <Button
            variant="outline"
            onClick={handleCreate}
            disabled={createMission.isPending || !title.trim()}
            className="dashboard-action-button dashboard-action-button-accent border-0"
          >
            {createMission.isPending ? 'Creating...' : 'Create Mission'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function MissionDateField({
  id,
  label,
  value,
  month,
  onMonthChange,
  onSelect,
}: {
  id: string
  label: string
  value?: Date
  month: Date
  onMonthChange: (month: Date) => void
  onSelect: (date?: Date) => void
}) {
  const [open, setOpen] = useState(false)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          id={id}
          type="button"
          className="dashboard-date-field dashboard-mission-dialog-field dashboard-date-button"
          aria-label={label}
        >
          <span className={value ? 'dashboard-date-value' : 'dashboard-date-placeholder'}>
            {value ? format(value, 'dd/MM/yyyy') : 'dd/mm/yyyy'}
          </span>
          <span className="dashboard-date-icon-wrap">
            <CalendarDays size={15} className="dashboard-date-icon" />
          </span>
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        sideOffset={10}
        className="dashboard-date-popover w-[276px] p-0"
      >
        <MissionCalendar
          month={month}
          selected={value}
          onMonthChange={onMonthChange}
          onSelect={(date) => {
            onSelect(date)
            setOpen(false)
          }}
          onClear={() => {
            onSelect(undefined)
            setOpen(false)
          }}
        />
      </PopoverContent>
    </Popover>
  )
}

function MissionCalendar({
  month,
  selected,
  onMonthChange,
  onSelect,
  onClear,
}: {
  month: Date
  selected?: Date
  onMonthChange: (month: Date) => void
  onSelect: (date: Date) => void
  onClear: () => void
}) {
  const monthStart = startOfMonth(month)
  const calendarDays = eachDayOfInterval({
    start: startOfWeek(monthStart, { weekStartsOn: 1 }),
    end: endOfWeek(endOfMonth(month), { weekStartsOn: 1 }),
  })

  return (
    <div className="dashboard-calendar">
      <div className="dashboard-calendar-header">
        <div className="dashboard-calendar-title">{format(month, 'MMMM yyyy')}</div>
        <div className="dashboard-calendar-nav">
          <button
            type="button"
            className="dashboard-calendar-nav-button"
            aria-label="Previous month"
            onClick={() => onMonthChange(addMonths(month, -1))}
          >
            <ChevronLeft size={14} />
          </button>
          <button
            type="button"
            className="dashboard-calendar-nav-button"
            aria-label="Next month"
            onClick={() => onMonthChange(addMonths(month, 1))}
          >
            <ChevronRight size={14} />
          </button>
        </div>
      </div>

      <div className="dashboard-calendar-weekdays" aria-hidden>
        {['M', 'T', 'W', 'T', 'F', 'S', 'S'].map((day) => (
          <span key={day}>{day}</span>
        ))}
      </div>

      <div className="dashboard-calendar-grid">
        {calendarDays.map((day) => {
          const selectedDay = selected ? isSameDay(day, selected) : false
          const outsideMonth = !isSameMonth(day, month)
          const today = isToday(day)
          return (
            <button
              key={day.toISOString()}
              type="button"
              className="dashboard-calendar-day"
              data-selected={selectedDay ? 'true' : 'false'}
              data-outside={outsideMonth ? 'true' : 'false'}
              data-today={today ? 'true' : 'false'}
              onClick={() => onSelect(day)}
            >
              {format(day, 'd')}
            </button>
          )
        })}
      </div>

      <div className="dashboard-calendar-footer">
        <button type="button" className="dashboard-calendar-link" onClick={onClear}>
          Clear
        </button>
        <button
          type="button"
          className="dashboard-calendar-link"
          onClick={() => onSelect(new Date())}
        >
          Today
        </button>
      </div>
    </div>
  )
}
