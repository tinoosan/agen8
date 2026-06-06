import { describe, expect, it } from 'vitest'
import { formatDate, formatRelative } from './format'

const ago = (ms: number) => new Date(Date.now() - ms).toISOString()
const SEC = 1000
const MIN = 60 * SEC
const HOUR = 60 * MIN
const DAY = 24 * HOUR

describe('formatRelative', () => {
  it('returns the fallback for missing or unparseable input', () => {
    expect(formatRelative(undefined)).toBe('')
    expect(formatRelative('not-a-date')).toBe('')
    expect(formatRelative(undefined, { fallback: 'unknown' })).toBe('unknown')
    expect(formatRelative('not-a-date', { fallback: '—' })).toBe('—')
  })

  it('says "just now" for sub-minute times by default', () => {
    expect(formatRelative(new Date().toISOString())).toBe('just now')
    expect(formatRelative(ago(30 * SEC))).toBe('just now')
  })

  it('clamps future timestamps to "just now"', () => {
    expect(formatRelative(new Date(Date.now() + MIN).toISOString())).toBe('just now')
  })

  it('shows second granularity only when seconds:true', () => {
    expect(formatRelative(ago(30 * SEC), { seconds: true })).toBe('30s ago')
    expect(formatRelative(new Date().toISOString(), { seconds: true })).toBe('just now')
    // seconds option has no effect once past a minute
    expect(formatRelative(ago(5 * MIN), { seconds: true })).toBe('5m ago')
  })

  it('formats minutes, hours, and days', () => {
    expect(formatRelative(ago(5 * MIN))).toBe('5m ago')
    expect(formatRelative(ago(3 * HOUR))).toBe('3h ago')
    expect(formatRelative(ago(5 * DAY))).toBe('5d ago')
  })

  it('rolls old timestamps into months and years', () => {
    expect(formatRelative(ago(90 * DAY))).toBe('3mo ago')
    expect(formatRelative(ago(400 * DAY))).toBe('1y ago')
  })
})

describe('formatDate', () => {
  const iso = '2026-06-06T12:00:00.000Z'
  const date = new Date(iso)

  it('returns the fallback for missing or unparseable input', () => {
    expect(formatDate(undefined)).toBe('')
    expect(formatDate('not-a-date')).toBe('')
    expect(formatDate(undefined, { fallback: 'Unknown' })).toBe('Unknown')
    expect(formatDate('not-a-date', { fallback: 'Unknown' })).toBe('Unknown')
  })

  it('never leaks "Invalid Date" or the raw input', () => {
    expect(formatDate('Invalid Date')).toBe('')
    expect(formatDate('2026-13-99')).toBe('')
  })

  it('defaults to the medium long-form style', () => {
    expect(formatDate(iso)).toBe(
      date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' }),
    )
  })

  it('supports the numeric (locale-default) style', () => {
    expect(formatDate(iso, { style: 'numeric' })).toBe(date.toLocaleDateString())
  })
})
