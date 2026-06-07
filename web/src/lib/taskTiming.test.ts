import { describe, it, expect } from 'vitest'
import {
  pickupLatencyMs,
  inProgressDurationMs,
  isOverThreshold,
  formatCoarseDuration,
} from './taskTiming'
import type { Task } from './types'

// A fixed wall-clock anchor so spans are deterministic regardless of run time.
const T0 = Date.parse('2026-05-10T12:00:00.000Z')
const at = (ms: number) => new Date(T0 + ms).toISOString()
const MIN = 60_000
const HOUR = 60 * MIN
const DAY = 24 * HOUR

// Minimal task factory — only the fields the timing helpers read.
function task(partial: Partial<Task>): Task {
  return { id: 'task-1', description: 'do the thing', status: 'pending', ...partial }
}

describe('pickupLatencyMs', () => {
  it('returns null when never claimed', () => {
    expect(pickupLatencyMs(task({ createdAt: at(0) }))).toBeNull()
  })
  it('returns null when creation time unknown', () => {
    expect(pickupLatencyMs(task({ startedAt: at(MIN) }))).toBeNull()
  })
  it('returns startedAt - createdAt', () => {
    expect(pickupLatencyMs(task({ createdAt: at(0), startedAt: at(3 * MIN) }))).toBe(3 * MIN)
  })
  it('clamps clock skew to zero', () => {
    expect(pickupLatencyMs(task({ createdAt: at(5 * MIN), startedAt: at(MIN) }))).toBe(0)
  })
})

describe('inProgressDurationMs', () => {
  const now = T0 + HOUR
  it('returns null when never started', () => {
    expect(inProgressDurationMs(task({ createdAt: at(0) }), now)).toBeNull()
  })
  it('uses now while in flight', () => {
    expect(inProgressDurationMs(task({ startedAt: at(10 * MIN) }), now)).toBe(50 * MIN)
  })
  it('uses completedAt when finished', () => {
    expect(
      inProgressDurationMs(task({ startedAt: at(10 * MIN), completedAt: at(25 * MIN) }), now),
    ).toBe(15 * MIN)
  })
  it('clamps clock skew to zero', () => {
    expect(
      inProgressDurationMs(task({ startedAt: at(30 * MIN), completedAt: at(10 * MIN) }), now),
    ).toBe(0)
  })
})

describe('isOverThreshold', () => {
  const now = T0 + HOUR
  it('flags an in-flight task past the threshold', () => {
    expect(isOverThreshold(task({ status: 'active', startedAt: at(0) }), 30 * MIN, now)).toBe(true)
  })
  it('does not flag a task under the threshold', () => {
    expect(isOverThreshold(task({ status: 'active', startedAt: at(0) }), 90 * MIN, now)).toBe(false)
  })
  it('never flags a terminal task', () => {
    const t = task({ status: 'succeeded', startedAt: at(0), completedAt: at(90 * MIN) })
    expect(isOverThreshold(t, MIN, now)).toBe(false)
  })
  it('never flags an unstarted task', () => {
    expect(isOverThreshold(task({ status: 'pending', createdAt: at(0) }), MIN, now)).toBe(false)
  })
  it('treats a non-positive threshold as disabled', () => {
    expect(isOverThreshold(task({ status: 'active', startedAt: at(0) }), 0, now)).toBe(false)
  })
})

describe('formatCoarseDuration', () => {
  it('passes null through', () => {
    expect(formatCoarseDuration(null)).toBeNull()
  })
  it('shows <1m under a minute', () => {
    expect(formatCoarseDuration(30_000)).toBe('<1m')
  })
  it('shows minutes under an hour', () => {
    expect(formatCoarseDuration(45 * MIN)).toBe('45m')
  })
  it('shows hours and minutes', () => {
    expect(formatCoarseDuration(2 * HOUR + 15 * MIN)).toBe('2h 15m')
  })
  it('drops minutes on a whole hour', () => {
    expect(formatCoarseDuration(3 * HOUR)).toBe('3h')
  })
  it('shows days and hours past a day', () => {
    expect(formatCoarseDuration(DAY + 4 * HOUR)).toBe('1d 4h')
  })
  it('drops hours on a whole day', () => {
    expect(formatCoarseDuration(2 * DAY)).toBe('2d')
  })
})
