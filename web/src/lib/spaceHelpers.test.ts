import { describe, expect, it } from 'vitest'
import { projectDisplayName } from './spaceHelpers'
import type { Project } from './types'

function makeProject(overrides: Partial<Project>): Project {
  return {
    id: 'project-abc123',
    locationId: 'loc-1',
    root: '/Users/me/code/my-app',
    status: 'open',
    ...overrides,
  }
}

describe('projectDisplayName', () => {
  it('prefers a trimmed title', () => {
    expect(projectDisplayName(makeProject({ title: '  Market Research  ' }))).toBe('Market Research')
  })

  it('falls back to the root folder name when there is no title', () => {
    expect(projectDisplayName(makeProject({ title: undefined, root: '/Users/me/code/my-app' }))).toBe('my-app')
  })

  it('ignores a trailing slash when deriving the folder name', () => {
    expect(projectDisplayName(makeProject({ title: undefined, root: '/Users/me/code/my-app/' }))).toBe('my-app')
  })

  it('falls back to the id only when there is neither title nor folder', () => {
    expect(projectDisplayName(makeProject({ title: undefined, root: '', id: 'project-xyz' }))).toBe('project-xyz')
  })
})
