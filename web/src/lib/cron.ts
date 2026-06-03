// Client-side cron parser.
// Supports standard 5-field expressions: minute hour dom month dow
// and common aliases: @hourly, @daily, @weekly, @monthly, @yearly.

// Parsed representation of a single cron field (e.g. "1-5", "*/15", "1,3,5")
interface CronField {
  values: Set<number> // expanded set of matching values within [min, max]
}

export interface CronExpression {
  minute: CronField
  hour: CronField
  dom: CronField    // day of month
  month: CronField
  dow: CronField    // day of week (0=Sun, 6=Sat)
  raw: string       // original expression string
}

// Alias expansions to 5-field expressions
const ALIASES: Record<string, string> = {
  '@yearly':  '0 0 1 1 *',
  '@annually': '0 0 1 1 *',
  '@monthly': '0 0 1 * *',
  '@weekly':  '0 0 * * 0',
  '@daily':   '0 0 * * *',
  '@midnight': '0 0 * * *',
  '@hourly':  '0 * * * *',
}

// Field bounds: [min, max]
const FIELD_BOUNDS: [number, number][] = [
  [0, 59],  // minute
  [0, 23],  // hour
  [1, 31],  // dom
  [1, 12],  // month
  [0, 6],   // dow (0=Sun)
]

const DOW_NAMES: Record<string, number> = {
  sun: 0, mon: 1, tue: 2, wed: 3, thu: 4, fri: 5, sat: 6,
}

const MONTH_NAMES: Record<string, number> = {
  jan: 1, feb: 2, mar: 3, apr: 4, may: 5, jun: 6,
  jul: 7, aug: 8, sep: 9, oct: 10, nov: 11, dec: 12,
}

// Parse a single cron field token into a set of matching values
function parseField(token: string, min: number, max: number, names?: Record<string, number>): CronField | null {
  const values = new Set<number>()

  for (const part of token.split(',')) {
    const trimmed = part.trim()
    if (!trimmed) return null

    // Replace named values (e.g. MON, JAN)
    let resolved = trimmed.toLowerCase()
    if (names) {
      for (const [name, val] of Object.entries(names)) {
        resolved = resolved.replace(new RegExp(`\\b${name}\\b`, 'gi'), String(val))
      }
    }

    // Handle step: */2, 1-5/2, or just a number with step
    const stepMatch = resolved.match(/^(.+)\/(\d+)$/)
    const step = stepMatch ? parseInt(stepMatch[2], 10) : 1
    const range = stepMatch ? stepMatch[1] : resolved

    if (step < 1) return null

    if (range === '*') {
      for (let i = min; i <= max; i += step) values.add(i)
    } else if (range.includes('-')) {
      const [startStr, endStr] = range.split('-')
      const start = parseInt(startStr, 10)
      const end = parseInt(endStr, 10)
      if (isNaN(start) || isNaN(end) || start < min || end > max || start > end) return null
      for (let i = start; i <= end; i += step) values.add(i)
    } else {
      const val = parseInt(range, 10)
      if (isNaN(val) || val < min || val > max) return null
      values.add(val)
    }
  }

  return values.size > 0 ? { values } : null
}

// Parse a cron expression string into a CronExpression, or null if invalid
export function parseCron(expr: string): CronExpression | null {
  const trimmed = expr.trim()

  // Check for aliases first
  const aliased = ALIASES[trimmed.toLowerCase()]
  const effective = aliased ?? trimmed

  const parts = effective.split(/\s+/)
  if (parts.length !== 5) return null

  const namesByField: (Record<string, number> | undefined)[] = [
    undefined,    // minute: no names
    undefined,    // hour: no names
    undefined,    // dom: no names
    MONTH_NAMES,  // month
    DOW_NAMES,    // dow
  ]

  const fields: CronField[] = []
  for (let i = 0; i < 5; i++) {
    const field = parseField(parts[i], FIELD_BOUNDS[i][0], FIELD_BOUNDS[i][1], namesByField[i])
    if (!field) return null
    fields.push(field)
  }

  return {
    minute: fields[0],
    hour: fields[1],
    dom: fields[2],
    month: fields[3],
    dow: fields[4],
    raw: trimmed,
  }
}

// Compute the next time a cron expression matches after the given date.
// Returns null if the expression is invalid. Searches up to 2 years ahead.
export function nextCronRun(expr: string, after?: Date): Date | null {
  const cron = typeof expr === 'string' ? parseCron(expr) : null
  if (!cron) return null

  const start = after ? new Date(after) : new Date()
  // Advance to next minute boundary
  const candidate = new Date(start)
  candidate.setSeconds(0, 0)
  candidate.setMinutes(candidate.getMinutes() + 1)

  const limit = new Date(start)
  limit.setFullYear(limit.getFullYear() + 2)

  while (candidate < limit) {
    const month = candidate.getMonth() + 1  // JS months are 0-indexed
    const dom = candidate.getDate()
    const dow = candidate.getDay()
    const hour = candidate.getHours()
    const minute = candidate.getMinutes()

    if (
      cron.month.values.has(month) &&
      cron.dom.values.has(dom) &&
      cron.dow.values.has(dow) &&
      cron.hour.values.has(hour) &&
      cron.minute.values.has(minute)
    ) {
      return candidate
    }

    // Advance: skip by the smallest meaningful increment
    candidate.setMinutes(candidate.getMinutes() + 1)
  }

  return null
}

// Day-of-week names for human-readable descriptions
const DOW_LABELS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const MONTH_LABELS = ['', 'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December']

// Return a human-readable description of a cron expression.
// Returns empty string for invalid expressions.
export function describeCron(expr: string): string {
  const trimmed = expr.trim().toLowerCase()

  // Handle aliases with friendly names
  if (trimmed === '@hourly') return 'Every hour at :00'
  if (trimmed === '@daily' || trimmed === '@midnight') return 'Every day at midnight'
  if (trimmed === '@weekly') return 'Every Sunday at midnight'
  if (trimmed === '@monthly') return 'First day of every month at midnight'
  if (trimmed === '@yearly' || trimmed === '@annually') return 'January 1st at midnight'

  const cron = parseCron(expr)
  if (!cron) return ''

  const parts: string[] = []

  // Time portion
  const minutes = sorted(cron.minute.values)
  const hours = sorted(cron.hour.values)

  if (hours.length <= 3 && minutes.length <= 2) {
    // Enumerate specific times: "At 09:00, 17:00"
    const times = hours.flatMap(h =>
      minutes.map(m => `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`)
    )
    parts.push(`At ${times.join(', ')}`)
  } else {
    // Describe with intervals
    if (minutes.length === 60) {
      parts.push('Every minute')
    } else if (isStepPattern(minutes, 0)) {
      parts.push(`Every ${stepSize(minutes, 0)} minutes`)
    } else {
      parts.push(`At minute ${describeValues(minutes)}`)
    }

    if (hours.length < 24) {
      if (isStepPattern(hours, 0)) {
        parts.push(`every ${stepSize(hours, 0)} hours`)
      } else {
        parts.push(`past hour ${describeValues(hours)}`)
      }
    }
  }

  // Day-of-week constraint
  const dows = sorted(cron.dow.values)
  if (dows.length < 7) {
    if (dows.length === 5 && dows[0] === 1 && dows[4] === 5) {
      parts.push('on weekdays')
    } else if (dows.length === 2 && dows[0] === 0 && dows[1] === 6) {
      parts.push('on weekends')
    } else {
      parts.push(`on ${dows.map(d => DOW_LABELS[d]).join(', ')}`)
    }
  }

  // Day-of-month constraint
  const doms = sorted(cron.dom.values)
  if (doms.length < 31) {
    parts.push(`on day ${describeValues(doms)} of the month`)
  }

  // Month constraint
  const months = sorted(cron.month.values)
  if (months.length < 12) {
    parts.push(`in ${months.map(m => MONTH_LABELS[m]).join(', ')}`)
  }

  return parts.join(' ')
}

// Helpers for describeCron

function sorted(set: Set<number>): number[] {
  return Array.from(set).sort((a, b) => a - b)
}

function describeValues(values: number[]): string {
  if (values.length <= 5) return values.join(', ')
  return `${values[0]}-${values[values.length - 1]}`
}

function isStepPattern(values: number[], min: number): boolean {
  if (values.length < 2) return false
  const step = values[1] - values[0]
  if (step < 2) return false
  return values.every((v, i) => v === min + i * step)
}

function stepSize(values: number[], min: number): number {
  if (values.length < 2) return 1
  return values[1] - (values[0] === min ? min : values[0])
}

// Validate that a string is a valid cron expression (5-field or alias)
export function isValidCron(expr: string): boolean {
  return parseCron(expr) !== null
}

// Format a relative time from now to a future date, e.g. "in 2h 15m" or "Mon 9:00am"
export function formatRelativeTime(target: Date, now?: Date): string {
  const ref = now ?? new Date()
  const diffMs = target.getTime() - ref.getTime()
  if (diffMs < 0) return 'past'

  const diffMin = Math.floor(diffMs / 60_000)
  if (diffMin < 60) return `in ${diffMin}m`

  const diffHr = Math.floor(diffMin / 60)
  const remMin = diffMin % 60
  if (diffHr < 24) return remMin > 0 ? `in ${diffHr}h ${remMin}m` : `in ${diffHr}h`

  // More than 24h away — show day and time
  const dayName = DOW_LABELS[target.getDay()].slice(0, 3)
  const hour = target.getHours()
  const minute = target.getMinutes()
  const ampm = hour >= 12 ? 'pm' : 'am'
  const h12 = hour % 12 || 12
  const timeStr = minute > 0
    ? `${h12}:${String(minute).padStart(2, '0')}${ampm}`
    : `${h12}${ampm}`
  return `${dayName} ${timeStr}`
}
