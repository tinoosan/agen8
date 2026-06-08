import type { KeyResultStatus } from '../../lib/types'

/* ── Key-result field metadata + display helpers ──────────────
   Shared by KRRow (edit form), KRDetailView (metadata chips), and
   KRProgressStrip (ghost inputs). Kept in one place so the option
   lists and their human labels can't drift apart.

   This module exports only data + pure helpers — no components. The one
   JSX-returning helper (directionIcon) lives in KRRow, its sole consumer,
   so react-refresh's only-export-components rule stays satisfied here. */

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
