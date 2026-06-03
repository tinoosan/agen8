import { describe, expect, it } from 'vitest'
import { resolveUpdatedByActor } from './krActorLabels'

describe('resolveUpdatedByActor', () => {
  it('replaces soft-deleted space IDs in member actor strings with space names', () => {
    const resolved = resolveUpdatedByActor(
      'member:space-96cb45db-bd40-400d-9789-2300794182f8/head-analyst',
      ({ spaceId }) => (spaceId === 'space-96cb45db-bd40-400d-9789-2300794182f8' ? 'market-research' : ''),
    )
    expect(resolved).toBe('member:market-research/head-analyst')
  })

  it('keeps original actor string when no space name can be resolved', () => {
    const original = 'member:space-unknown/head-analyst'
    const resolved = resolveUpdatedByActor(original, () => '')
    expect(resolved).toBe(original)
  })
})
