import { describe, expect, it } from 'vitest'
import { resolveEdgeStrokeColor } from './edgeColors'

describe('resolveEdgeStrokeColor', () => {
  it('uses focused target color for incoming direct edges', () => {
    const color = resolveEdgeStrokeColor({
      isDirect: true,
      isAmbient: false,
      focusNodeId: 'target-node',
      source: 'source-node',
      target: 'target-node',
      sourceColor: 'var(--amber)',
      targetColor: 'var(--blue)',
    })
    expect(color).toBe('var(--blue)')
  })

  it('uses source color for outgoing direct edges', () => {
    const color = resolveEdgeStrokeColor({
      isDirect: true,
      isAmbient: false,
      focusNodeId: 'source-node',
      source: 'source-node',
      target: 'target-node',
      sourceColor: 'var(--green)',
      targetColor: 'var(--blue)',
    })
    expect(color).toBe('var(--green)')
  })

  it('falls back to neutral when edge is outside focus neighbourhood', () => {
    const color = resolveEdgeStrokeColor({
      isDirect: false,
      isAmbient: false,
      focusNodeId: 'node-a',
      source: 'node-b',
      target: 'node-c',
      sourceColor: 'var(--green)',
      targetColor: 'var(--blue)',
    })
    expect(color).toBe('var(--text-3)')
  })
})
