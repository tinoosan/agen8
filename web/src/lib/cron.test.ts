// Unit tests for the client-side cron parser (F8).
// Covers parseCron, nextCronRun, describeCron, isValidCron, formatRelativeTime.

import { describe, it, expect } from 'vitest'
import { parseCron, nextCronRun, describeCron, isValidCron, formatRelativeTime } from './cron'

describe('parseCron', () => {
  it('parses a basic 5-field expression', () => {
    const result = parseCron('0 9 * * 1-5')
    expect(result).not.toBeNull()
    expect(result!.minute.values).toEqual(new Set([0]))
    expect(result!.hour.values).toEqual(new Set([9]))
    expect(result!.dom.values.size).toBe(31) // all days
    expect(result!.month.values.size).toBe(12) // all months
    expect(result!.dow.values).toEqual(new Set([1, 2, 3, 4, 5]))
  })

  it('parses wildcards', () => {
    const result = parseCron('* * * * *')
    expect(result).not.toBeNull()
    expect(result!.minute.values.size).toBe(60)
    expect(result!.hour.values.size).toBe(24)
  })

  it('parses step patterns', () => {
    const result = parseCron('*/15 * * * *')
    expect(result).not.toBeNull()
    expect(result!.minute.values).toEqual(new Set([0, 15, 30, 45]))
  })

  it('parses lists', () => {
    const result = parseCron('0 9,17 * * *')
    expect(result).not.toBeNull()
    expect(result!.hour.values).toEqual(new Set([9, 17]))
  })

  it('parses ranges with steps', () => {
    const result = parseCron('0 9 1-15/3 * *')
    expect(result).not.toBeNull()
    expect(result!.dom.values).toEqual(new Set([1, 4, 7, 10, 13]))
  })

  it('returns null for invalid expressions', () => {
    expect(parseCron('')).toBeNull()
    expect(parseCron('invalid')).toBeNull()
    expect(parseCron('0 9 *')).toBeNull() // too few fields
    expect(parseCron('0 9 * * * *')).toBeNull() // too many fields
    expect(parseCron('60 9 * * *')).toBeNull() // minute out of range
    expect(parseCron('0 25 * * *')).toBeNull() // hour out of range
  })

  it('handles @daily alias', () => {
    const result = parseCron('@daily')
    expect(result).not.toBeNull()
    expect(result!.minute.values).toEqual(new Set([0]))
    expect(result!.hour.values).toEqual(new Set([0]))
  })

  it('handles @hourly alias', () => {
    const result = parseCron('@hourly')
    expect(result).not.toBeNull()
    expect(result!.minute.values).toEqual(new Set([0]))
    expect(result!.hour.values.size).toBe(24)
  })

  it('handles @weekly alias', () => {
    const result = parseCron('@weekly')
    expect(result).not.toBeNull()
    expect(result!.dow.values).toEqual(new Set([0])) // Sunday
  })

  it('handles @monthly alias', () => {
    const result = parseCron('@monthly')
    expect(result).not.toBeNull()
    expect(result!.dom.values).toEqual(new Set([1]))
  })

  it('handles @yearly alias', () => {
    const result = parseCron('@yearly')
    expect(result).not.toBeNull()
    expect(result!.month.values).toEqual(new Set([1]))
    expect(result!.dom.values).toEqual(new Set([1]))
  })

  it('handles day-of-week names', () => {
    const result = parseCron('0 9 * * mon-fri')
    expect(result).not.toBeNull()
    expect(result!.dow.values).toEqual(new Set([1, 2, 3, 4, 5]))
  })

  it('handles month names', () => {
    const result = parseCron('0 0 1 jan,jun *')
    expect(result).not.toBeNull()
    expect(result!.month.values).toEqual(new Set([1, 6]))
  })
})

describe('nextCronRun', () => {
  it('computes next run for a simple cron', () => {
    // "0 9 * * *" = every day at 9am
    const after = new Date('2026-03-20T08:00:00Z')
    const next = nextCronRun('0 9 * * *', after)
    expect(next).not.toBeNull()
    expect(next!.getUTCHours()).toBe(9)
    expect(next!.getUTCMinutes()).toBe(0)
  })

  it('returns null for invalid expression', () => {
    expect(nextCronRun('invalid')).toBeNull()
  })

  it('advances past current minute', () => {
    const after = new Date('2026-03-20T09:00:00Z')
    const next = nextCronRun('0 9 * * *', after)
    expect(next).not.toBeNull()
    // Should be the next day since we're already at 09:00
    expect(next!.getUTCDate()).toBe(21)
  })

  it('handles step patterns', () => {
    const after = new Date('2026-03-20T08:00:00Z')
    const next = nextCronRun('*/15 * * * *', after)
    expect(next).not.toBeNull()
    // Next :15 minute mark after 08:00
    expect(next!.getUTCMinutes() % 15).toBe(0)
  })
})

describe('describeCron', () => {
  it('describes weekday morning schedule', () => {
    const desc = describeCron('0 9 * * 1-5')
    expect(desc).toContain('09:00')
    expect(desc.toLowerCase()).toContain('weekday')
  })

  it('describes @daily alias', () => {
    expect(describeCron('@daily')).toBe('Every day at midnight')
  })

  it('describes @hourly alias', () => {
    expect(describeCron('@hourly')).toBe('Every hour at :00')
  })

  it('describes step pattern', () => {
    const desc = describeCron('*/15 * * * *')
    expect(desc).toContain('15 minutes')
  })

  it('returns empty for invalid expression', () => {
    expect(describeCron('invalid')).toBe('')
  })

  it('describes specific month', () => {
    const desc = describeCron('0 0 1 6 *')
    expect(desc).toContain('June')
  })
})

describe('isValidCron', () => {
  it('returns true for valid expressions', () => {
    expect(isValidCron('0 9 * * 1-5')).toBe(true)
    expect(isValidCron('*/5 * * * *')).toBe(true)
    expect(isValidCron('@daily')).toBe(true)
  })

  it('returns false for invalid expressions', () => {
    expect(isValidCron('')).toBe(false)
    expect(isValidCron('bad')).toBe(false)
    expect(isValidCron('60 * * * *')).toBe(false)
  })
})

describe('formatRelativeTime', () => {
  it('formats minutes', () => {
    const now = new Date('2026-03-20T09:00:00Z')
    const target = new Date('2026-03-20T09:30:00Z')
    expect(formatRelativeTime(target, now)).toBe('in 30m')
  })

  it('formats hours and minutes', () => {
    const now = new Date('2026-03-20T09:00:00Z')
    const target = new Date('2026-03-20T11:30:00Z')
    expect(formatRelativeTime(target, now)).toBe('in 2h 30m')
  })

  it('formats exact hours', () => {
    const now = new Date('2026-03-20T09:00:00Z')
    const target = new Date('2026-03-20T12:00:00Z')
    expect(formatRelativeTime(target, now)).toBe('in 3h')
  })

  it('formats day and time for >24h', () => {
    const now = new Date('2026-03-20T09:00:00Z')
    const target = new Date('2026-03-22T10:30:00Z')
    const result = formatRelativeTime(target, now)
    // Should contain day name and time
    expect(result).toMatch(/\w{3}\s/)
  })

  it('returns past for negative diff', () => {
    const now = new Date('2026-03-20T09:00:00Z')
    const target = new Date('2026-03-20T08:00:00Z')
    expect(formatRelativeTime(target, now)).toBe('past')
  })
})
