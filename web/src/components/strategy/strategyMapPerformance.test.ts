import { describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import type { Edge, Node } from '@xyflow/react'
import {
  getNextDisplayMode,
  useLeafDisplayPhase,
  useDeferredStrategyGraph,
} from './strategyMapPerformance'

describe('getNextDisplayMode', () => {
  it('does not flap mission/KR mode when zoom stays inside the hysteresis window', () => {
    const previous = { missionKR: 'full' as const, leaf: 'dot' as const }
    expect(getNextDisplayMode(0.34, previous)).toBe(previous)
  })

  it('switches mission/KR mode only after crossing the lower exit threshold', () => {
    const previous = { missionKR: 'full' as const, leaf: 'full' as const }
    expect(getNextDisplayMode(0.29, previous)).toEqual({ missionKR: 'orbit', leaf: 'dot' })
  })

  it('keeps leaf nodes compact until zoom crosses the higher entry threshold', () => {
    const previous = { missionKR: 'full' as const, leaf: 'dot' as const }
    expect(getNextDisplayMode(0.7, previous)).toBe(previous)
    expect(getNextDisplayMode(0.8, previous)).toEqual({ missionKR: 'full', leaf: 'full' })
  })
})

describe('useDeferredStrategyGraph', () => {
  const baseNodes = [{ id: 'a', position: { x: 0, y: 0 }, data: {} }] as Node[]
  const baseEdges = [] as Edge[]
  const nextNodes = [{ id: 'b', position: { x: 10, y: 5 }, data: {} }] as Node[]
  const baseGraph = { nodes: baseNodes, edges: baseEdges, isLoading: false }
  const nextGraph = { nodes: nextNodes, edges: baseEdges, isLoading: false }

  it('holds incoming graph changes while interaction is active', () => {
    const { result, rerender } = renderHook(
      ({ graph, isInteracting }) =>
        useDeferredStrategyGraph(graph, isInteracting),
      {
        initialProps: { graph: baseGraph, isInteracting: false },
      },
    )

    expect(result.current.nodes).toBe(baseNodes)

    rerender({ graph: nextGraph, isInteracting: true })

    expect(result.current.nodes).toBe(baseNodes)
  })

  it('flushes the latest queued graph when interaction ends', () => {
    const { result, rerender } = renderHook(
      ({ graph, isInteracting }) =>
        useDeferredStrategyGraph(graph, isInteracting),
      {
        initialProps: { graph: baseGraph, isInteracting: false },
      },
    )

    rerender({ graph: nextGraph, isInteracting: true })
    rerender({ graph: nextGraph, isInteracting: false })

    expect(result.current.nodes).toBe(nextNodes)
  })
})

describe('useLeafDisplayPhase', () => {
  it('transitions dot -> toFull -> full with a single shared timer', () => {
    vi.useFakeTimers()
    try {
      const { result, rerender } = renderHook(
        ({ mode }) => useLeafDisplayPhase(mode),
        { initialProps: { mode: 'dot' as const } },
      )

      expect(result.current).toBe('dot')

      rerender({ mode: 'full' as const })
      expect(result.current).toBe('toFull')

      act(() => {
        vi.advanceTimersByTime(140)
      })
      expect(result.current).toBe('full')
    } finally {
      vi.useRealTimers()
    }
  })

  it('transitions full -> toDot -> dot', () => {
    vi.useFakeTimers()
    try {
      const { result, rerender } = renderHook(
        ({ mode }) => useLeafDisplayPhase(mode),
        { initialProps: { mode: 'full' as const } },
      )

      expect(result.current).toBe('full')

      rerender({ mode: 'dot' as const })
      expect(result.current).toBe('toDot')

      act(() => {
        vi.advanceTimersByTime(120)
      })
      expect(result.current).toBe('dot')
    } finally {
      vi.useRealTimers()
    }
  })
})
