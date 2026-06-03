import { describe, expect, it } from 'vitest'
import { assignableSpaces, keyResultSpaceOwnerLabel, keyResultSpaceOwnerLabelFromKR, spaceSummaryLabel } from './spaceOwnerLabels'

describe('spaceOwnerLabels', () => {
  it('uses the space name as the label', () => {
    expect(spaceSummaryLabel({
      spaceId: 'space-1',
      spaceName: 'ASML thesis',
    })).toBe('ASML thesis')
  })

  it('falls back to the default space label when no space name is available', () => {
    expect(spaceSummaryLabel({
      spaceId: 'space-2',
    })).toBe('Space')
  })

  it('resolves key result owners through the project space catalog', () => {
    expect(keyResultSpaceOwnerLabel('space-2', [
      { spaceId: 'space-1', spaceName: 'Research' },
      { spaceId: 'space-2', spaceName: 'Network scanner' },
    ])).toBe('Network scanner')
  })

  it('falls back to the KR owner snapshot when the space is no longer in the active catalog', () => {
    expect(keyResultSpaceOwnerLabelFromKR({
      spaceId: 'space-deleted',
      ownerSpaceName: 'tnxp-thesis',
    }, [])).toBe('tnxp-thesis')
  })

  it('excludes archived deleted inactive and duplicate spaces from assignable options', () => {
    expect(assignableSpaces([
      { spaceId: 'space-active', spaceName: 'Space', projectRoot: '/repo', status: 'active' },
      { spaceId: 'space-duplicate', spaceName: ' space ', projectRoot: '/repo', status: 'active' },
      { spaceId: 'space-archived', spaceName: 'Old', projectRoot: '/repo', status: 'archived' },
      { spaceId: 'space-deleted', spaceName: 'Deleted', projectRoot: '/repo', status: 'deleted' },
      { spaceId: 'space-inactive', spaceName: 'Inactive', projectRoot: '/repo', status: 'inactive' },
      { spaceId: 'space-deleting', spaceName: 'Deleting', projectRoot: '/repo', status: 'active', lifecyclePhase: 'deleting' },
    ]).map(space => space.spaceId)).toEqual(['space-active'])
  })
})
