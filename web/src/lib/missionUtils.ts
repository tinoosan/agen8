import type { KeyResultMeasurementType } from './types'

/**
 * Formats a key result's progress as a human-readable string.
 * Centralised here so every render site (MissionEditor, MissionSummary, etc.)
 * displays identical text for a given measurement type.
 *
 * Returns `null` for binary KRs (which should show Done/Not-done badges instead
 * of a numeric string).
 */
export function formatKRProgress(opts: {
  measurementType: KeyResultMeasurementType
  currentValue: number
  targetValue: number
  unit?: string
}): string | null {
  switch (opts.measurementType) {
    case 'binary':
      return opts.currentValue >= 1 ? 'Done' : 'Not done'
    case 'percentage':
      return `${opts.currentValue}% / ${opts.targetValue}%`
    case 'numeric':
    case 'currency':
    case 'count':
      return `${opts.currentValue} / ${opts.targetValue}${opts.unit ? ` ${opts.unit}` : ''}`
  }
}
