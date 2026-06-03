import { describe, expect, it } from 'vitest'
import type { Node } from '@xyflow/react'
import { deriveNodeClusterIdentity } from './clusterIdentity'

function makeNode(partial: Partial<Node>): Node {
  return {
    id: partial.id ?? 'node-1',
    type: partial.type ?? 'task',
    data: partial.data ?? {},
    position: partial.position ?? { x: 0, y: 0 },
  } as Node
}

describe('deriveNodeClusterIdentity', () => {
  it('prefers decision spaceName over spaceId/source identity', () => {
    const node = makeNode({
      id: 'decision:1',
      type: 'decision',
      data: {
        decision: {
          spaceName: 'market-research',
          spaceId: 'space-abc',
          sourceIdentity: 'cfo',
        },
      },
    })
    expect(deriveNodeClusterIdentity(node)).toBe('market-research')
  })

  it('falls back to task spaceId when spaceName is absent', () => {
    const node = makeNode({
      id: 'task:1',
      type: 'task',
      data: {
        task: {
          spaceId: 'space-xyz',
          assignedRole: 'qa-engineer',
        },
      },
    })
    expect(deriveNodeClusterIdentity(node)).toBe('space-xyz')
  })

  it('uses node id as a final fallback', () => {
    const node = makeNode({
      id: 'plan:123',
      type: 'plan',
      data: { plan: {} },
    })
    expect(deriveNodeClusterIdentity(node)).toBe('plan:123')
  })
})
