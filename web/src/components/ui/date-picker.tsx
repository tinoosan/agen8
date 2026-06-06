import { useMemo, useState } from 'react'
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
import { CalendarDays, ChevronLeft, ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Popover, PopoverContent, PopoverTrigger } from './popover'

interface DatePickerProps {
  id?: string
  /** Selected date as a `yyyy-MM-dd` string, or '' when unset. */
  value: string
  onChange: (value: string) => void
  placeholder?: string
  /** Earliest selectable date (`yyyy-MM-dd`). */
  min?: string
  /** Latest selectable date (`yyyy-MM-dd`). */
  max?: string
  className?: string
}

const WEEKDAYS = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa']

function parse(value: string | undefined): Date | null {
  if (!value) return null
  const parsed = parseISO(value)
  return Number.isNaN(parsed.getTime()) ? null : parsed
}

export function DatePicker({ id, value, onChange, placeholder = 'Any date', min, max, className }: DatePickerProps) {
  const selected = parse(value)
  const minDate = parse(min)
  const maxDate = parse(max)
  const [open, setOpen] = useState(false)
  const [month, setMonth] = useState(() => startOfMonth(selected ?? new Date()))

  const days = useMemo(() => {
    const start = startOfWeek(startOfMonth(month))
    const end = endOfWeek(endOfMonth(month))
    return eachDayOfInterval({ start, end })
  }, [month])

  const isDisabled = (day: Date) => (minDate != null && day < minDate) || (maxDate != null && day > maxDate)

  const select = (day: Date) => {
    onChange(format(day, 'yyyy-MM-dd'))
    setOpen(false)
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        // Reopen on the month of the current selection so the picker is always
        // anchored to context rather than wherever the user last browsed.
        if (next) setMonth(startOfMonth(selected ?? new Date()))
        setOpen(next)
      }}
    >
      <PopoverTrigger asChild>
        <button
          id={id}
          type="button"
          className={cn(
            'flex h-9 items-center gap-2 rounded-[var(--r-md)] border border-transparent bg-[var(--bg-elevated)] px-3 text-sm text-[var(--text-1)] transition-colors hover:bg-[var(--bg-hover)] focus-visible:outline-none focus-visible:border-[var(--border-focus)] data-[state=open]:border-[var(--border-focus)] data-[state=open]:bg-[var(--bg-surface)]',
            className,
          )}
        >
          <CalendarDays size={14} className="shrink-0 text-[var(--text-3)]" />
          <span className={cn('truncate', !selected && 'text-[var(--text-3)]')}>
            {selected ? format(selected, 'MMM d, yyyy') : placeholder}
          </span>
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-auto border-0 p-3">
        <div className="mb-2 flex items-center justify-between">
          <button
            type="button"
            aria-label="Previous month"
            onClick={() => setMonth(addMonths(month, -1))}
            className="flex h-7 w-7 items-center justify-center rounded-[var(--r-sm)] text-[var(--text-2)] transition-colors hover:bg-[var(--bg-hover)] hover:text-[var(--text-1)]"
          >
            <ChevronLeft size={15} />
          </button>
          <div className="text-sm font-medium text-[var(--text-1)]">{format(month, 'MMMM yyyy')}</div>
          <button
            type="button"
            aria-label="Next month"
            onClick={() => setMonth(addMonths(month, 1))}
            className="flex h-7 w-7 items-center justify-center rounded-[var(--r-sm)] text-[var(--text-2)] transition-colors hover:bg-[var(--bg-hover)] hover:text-[var(--text-1)]"
          >
            <ChevronRight size={15} />
          </button>
        </div>

        <div className="mb-1 grid grid-cols-7 gap-0.5">
          {WEEKDAYS.map((day) => (
            <div key={day} className="text-center text-[0.625rem] font-medium uppercase tracking-[0.02em] text-[var(--text-3)]">
              {day}
            </div>
          ))}
        </div>

        <div className="grid grid-cols-7 gap-0.5">
          {days.map((day) => {
            const inMonth = isSameMonth(day, month)
            const isSelected = selected != null && isSameDay(day, selected)
            const disabled = isDisabled(day)
            return (
              <button
                key={format(day, 'yyyy-MM-dd')}
                type="button"
                disabled={disabled}
                onClick={() => select(day)}
                className={cn(
                  'flex h-8 w-8 items-center justify-center rounded-[var(--r-sm)] text-[0.8125rem] transition-colors',
                  inMonth ? 'text-[var(--text-1)]' : 'text-[var(--text-3)] opacity-60',
                  !isSelected && !disabled && 'hover:bg-[var(--bg-hover)]',
                  isToday(day) && !isSelected && 'ring-1 ring-inset ring-[var(--accent)]',
                  isSelected && 'bg-[var(--accent)] font-medium text-white hover:bg-[var(--accent)]',
                  disabled && 'cursor-not-allowed opacity-30 hover:bg-transparent',
                )}
              >
                {format(day, 'd')}
              </button>
            )
          })}
        </div>

        {value && (
          <div className="mt-2 flex justify-end border-t border-[var(--border)] pt-2">
            <button
              type="button"
              onClick={() => {
                onChange('')
                setOpen(false)
              }}
              className="text-[0.75rem] text-[var(--text-3)] transition-colors hover:text-[var(--text-1)]"
            >
              Clear
            </button>
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}
