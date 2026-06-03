import { describe, expect, it } from 'vitest'
import { getProjectSpaceQueryKeysToInvalidate } from './useProjectSpaces'

describe('getProjectSpaceQueryKeysToInvalidate', () => {
  it('invalidates the global space list and the affected project list when a project id is known', () => {
    expect(getProjectSpaceQueryKeysToInvalidate('project-alpha')).toEqual([
      ['project.space.list', ''],
      ['project.space.list', 'project-alpha'],
    ])
  })

  it('only invalidates the global list when the project id is absent', () => {
    expect(getProjectSpaceQueryKeysToInvalidate(undefined)).toEqual([
      ['project.space.list', ''],
    ])
    expect(getProjectSpaceQueryKeysToInvalidate('')).toEqual([
      ['project.space.list', ''],
    ])
  })
})
