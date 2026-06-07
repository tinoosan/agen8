import { describe, expect, it, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import type { Node } from '@xyflow/react'
import { useStrategyMapFocusEffects } from './useStrategyMapFocusEffects'
import { useStrategyMapStore } from './strategyMapStore'

/* ── Fixtures ──────────────────────────────────────────────────────────────
 * These tests pin the archived-retry branch added so that deep-linking
 * (?focus= / command palette) to an archived mission or KR — which the map
 * hides by default — auto-enables the archived view exactly once, while a
 * deep link to a not-yet-rendered task/decision leaf never toggles archived.
 */
function missionNode(id: string): Node {
  return { id, type: 'mission', position: { x: 0, y: 0 }, data: {} }
}

type Props = Parameters<typeof useStrategyMapFocusEffects>[0]

function baseProps(overrides: Partial<Props>): Props {
  return {
    displayNodes: [],
    setCenter: vi.fn(),
    setFocusNodeId: vi.fn(),
    setSelectedNodeId: vi.fn(),
    displayMode: { missionKR: 'full', leaf: 'full' },
    leafPhase: 'full',
    isInteracting: false,
    isZooming: false,
    isDense: false,
    effectiveFocusNodeId: null,
    selectedNodeId: null,
    clusterNodeIds: null,
    directEdgeIds: null,
    clusterEdgeIds: null,
    activeFilter: null,
    showArchived: false,
    onRequestShowArchived: vi.fn(),
    ...overrides,
  }
}

describe('useStrategyMapFocusEffects — archived deep-link reveal', () => {
  beforeEach(() => {
    // Drive the pending target through the store rather than the URL so each
    // test starts from a known state; clear it after the previous test.
    useStrategyMapStore.setState({ pendingFocusNodeId: null })
    window.history.replaceState({}, '', '/')
  })

  it('enables the archived view once when a missing mission/KR target is pending', () => {
    const onRequestShowArchived = vi.fn()
    useStrategyMapStore.setState({ pendingFocusNodeId: 'mission-archived' })

    // Some live nodes are present (so the effect runs) but not the target.
    const { rerender } = renderHook(
      (p: Props) => useStrategyMapFocusEffects(p),
      { initialProps: baseProps({ displayNodes: [missionNode('mission-live')], onRequestShowArchived }) },
    )
    expect(onRequestShowArchived).toHaveBeenCalledTimes(1)

    // A subsequent graph change with the target still missing must NOT re-toggle.
    rerender(baseProps({ displayNodes: [missionNode('mission-live'), missionNode('mission-other')], onRequestShowArchived }))
    expect(onRequestShowArchived).toHaveBeenCalledTimes(1)
  })

  it('does not toggle archived for a not-yet-rendered task/decision leaf target', () => {
    const onRequestShowArchived = vi.fn()
    useStrategyMapStore.setState({ pendingFocusNodeId: 'task:task-pending' })

    renderHook(
      (p: Props) => useStrategyMapFocusEffects(p),
      { initialProps: baseProps({ displayNodes: [missionNode('mission-live')], onRequestShowArchived }) },
    )
    expect(onRequestShowArchived).not.toHaveBeenCalled()
  })

  it('does not toggle archived when archived is already shown', () => {
    const onRequestShowArchived = vi.fn()
    useStrategyMapStore.setState({ pendingFocusNodeId: 'mission-archived' })

    renderHook(
      (p: Props) => useStrategyMapFocusEffects(p),
      { initialProps: baseProps({ displayNodes: [missionNode('mission-live')], showArchived: true, onRequestShowArchived }) },
    )
    expect(onRequestShowArchived).not.toHaveBeenCalled()
  })

  it('focuses without toggling archived once the target node is present', () => {
    const onRequestShowArchived = vi.fn()
    const setFocusNodeId = vi.fn()
    const setSelectedNodeId = vi.fn()
    useStrategyMapStore.setState({ pendingFocusNodeId: 'mission-target' })

    renderHook(
      (p: Props) => useStrategyMapFocusEffects(p),
      {
        initialProps: baseProps({
          displayNodes: [missionNode('mission-target')],
          onRequestShowArchived,
          setFocusNodeId,
          setSelectedNodeId,
        }),
      },
    )
    expect(onRequestShowArchived).not.toHaveBeenCalled()
    // The pending target is consumed (cleared from the store) when found.
    expect(useStrategyMapStore.getState().pendingFocusNodeId).toBeNull()
  })
})
