import { afterEach, describe, expect, it } from 'vitest'
import {
  deriveGettingStarted,
  readGettingStartedDismissed,
  writeGettingStartedDismissed,
} from './gettingStarted'

describe('deriveGettingStarted', () => {
  it('fresh project: only the implicit steps are unticked', () => {
    const state = deriveGettingStarted({ memberCount: 0, missionCount: 0, taskCount: 0 })
    expect(state.done).toEqual({ connect: false, skill: false, agent: false, work: false })
    expect(state.complete).toBe(false)
  })

  it('first member registration ticks connect, skill, and agent together', () => {
    const state = deriveGettingStarted({ memberCount: 1, missionCount: 0, taskCount: 0 })
    expect(state.done).toEqual({ connect: true, skill: true, agent: true, work: false })
    expect(state.complete).toBe(false)
  })

  it('a mission ticks the work step', () => {
    const state = deriveGettingStarted({ memberCount: 1, missionCount: 1, taskCount: 0 })
    expect(state.done.work).toBe(true)
    expect(state.complete).toBe(true)
  })

  it('a task alone also ticks the work step', () => {
    const state = deriveGettingStarted({ memberCount: 1, missionCount: 0, taskCount: 3 })
    expect(state.done.work).toBe(true)
    expect(state.complete).toBe(true)
  })

  it('work without a member never completes (member is the payoff signal)', () => {
    const state = deriveGettingStarted({ memberCount: 0, missionCount: 2, taskCount: 5 })
    expect(state.complete).toBe(false)
    expect(state.done.agent).toBe(false)
  })
})

describe('getting-started dismissal storage', () => {
  afterEach(() => localStorage.clear())

  it('defaults to not dismissed', () => {
    expect(readGettingStartedDismissed('project-a')).toBe(false)
  })

  it('persists per project', () => {
    writeGettingStartedDismissed('project-a')
    expect(readGettingStartedDismissed('project-a')).toBe(true)
    expect(readGettingStartedDismissed('project-b')).toBe(false)
  })
})
