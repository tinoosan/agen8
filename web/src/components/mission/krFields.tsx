import { TrendingUp, TrendingDown } from 'lucide-react'
import type { KeyResultStatus, KeyResultDirection } from '../../lib/types'

/* ── Key-result field metadata + display helpers ──────────────
   Shared by KRRow (edit form), KRDetailView (metadata chips), and
   KRProgressStrip (ghost inputs). Kept in one place so the option
   lists and their human labels can't drift apart. */

export const MEASUREMENT_TYPES = [
  { value: 'percentage', label: 'Percentage' },
  { value: 'count',      label: 'Count' },
  { value: 'numeric',    label: 'Numeric' },
  { value: 'currency',   label: 'Currency' },
  { value: 'binary',     label: 'Binary' },
] as const

export const DIRECTIONS = [
  { value: 'increase', label: 'Increase' },
  { value: 'decrease', label: 'Decrease' },
] as const

// Ghost input — blends with surrounding text, only keyboard reveals it
export const ghostInput = 'bg-transparent border-none outline-none p-0 m-0 font-[inherit] text-[inherit] w-full'

export function krStatusBadge(status: KeyResultStatus): { variant: 'success' | 'warning' | 'info' | 'secondary' | 'danger'; label: string } {
  switch (status) {
    case 'on_track':  return { variant: 'success',   label: 'On Track' }
    case 'at_risk':   return { variant: 'warning',   label: 'At Risk' }
    case 'completed': return { variant: 'info',      label: 'Completed' }
    case 'dropped':   return { variant: 'danger',    label: 'Dropped' }
    default:          return { variant: 'secondary', label: 'Open' }
  }
}

export function directionIcon(dir: KeyResultDirection) {
  switch (dir) {
    case 'increase': return <TrendingUp  size={10} className="text-[var(--green)]" />
    case 'decrease': return <TrendingDown size={10} className="text-[var(--red)]" />
  }
}

export function measurementLabel(type: string): string {
  return MEASUREMENT_TYPES.find(t => t.value === type)?.label ?? type
}

export function directionLabel(dir: string): string {
  return DIRECTIONS.find(d => d.value === dir)?.label ?? dir
}

export function shortAgent(raw: string): string {
  const slashPart = raw.split('/').pop() ?? raw
  const label = slashPart.split(':').pop() ?? slashPart
  return label.replace(/[_-]/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}
