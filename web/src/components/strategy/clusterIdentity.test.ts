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
  it('prefers readable decision member label over raw source identity', () => {
    const node = makeNode({
      id: 'decision:1',
      type: 'decision',
      data: {
        decision: {
          sourceIdentity: 'cfo',
          sourceMemberLabel: 'Chief Financial Officer',
          id: 'decision-1',
        },
      },
    })
    expect(deriveNodeClusterIdentity(node)).toBe('Chief Financial Officer')
  })

  it('prefers readable task assignedToLabel over raw assignedTo id', () => {
    const node = makeNode({
      id: 'task:1',
      type: 'task',
      data: {
        task: {
          assignedToLabel: 'QA engineer',
          assignedTo: 'member:qa-engineer',
          id: 'task-1',
        },
      },
    })
    expect(deriveNodeClusterIdentity(node)).toBe('QA engineer')
  })

  it('uses node id as a final fallback', () => {
    const node = makeNode({
      id: 'unknown:123',
      type: 'unknown',
      data: {},
    })
    expect(deriveNodeClusterIdentity(node)).toBe('unknown:123')
  })
})
