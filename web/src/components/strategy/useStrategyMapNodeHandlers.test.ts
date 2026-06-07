import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import type { Node, Edge } from '@xyflow/react'
import { useStrategyMapNodeHandlers } from './useStrategyMapNodeHandlers'

// These tests pin down the KR3 contract: traversal (a tap on a node, an edge
// click) moves the focus *cursor* and recenters, but only carries the detail
// panel along when the panel is already open (sticky-follow). Opening the panel
// is a deliberate gesture — tapping the node that is already focused.

const nodeA: Node = { id: 'a', position: { x: 10, y: 20 }, data: {} }
const nodeB: Node = { id: 'b', position: { x: 100, y: 200 }, data: {} }

function setup(overrides: Partial<Parameters<typeof useStrategyMapNodeHandlers>[0]> = {}) {
  const setCenter = vi.fn()
  const getZoom = vi.fn(() => 1)
  const fitView = vi.fn()
  const markInteraction = vi.fn()
  const setFocusNodeId = vi.fn()
  const setSelectedNodeId = vi.fn()
  const nodeById = new Map<string, Node>([
    [nodeA.id, nodeA],
    [nodeB.id, nodeB],
  ])

  const props = {
    effectiveFocusNodeId: null as string | null,
    selectedNodeId: null as string | null,
    nodeById,
    setCenter,
    getZoom,
    fitView,
    markInteraction,
    setFocusNodeId,
    setSelectedNodeId,
    ...overrides,
  }

  const { result } = renderHook(() => useStrategyMapNodeHandlers(props))
  return {
    handlers: result.current,
    setCenter,
    getZoom,
    fitView,
    markInteraction,
    setFocusNodeId,
    setSelectedNodeId,
  }
}

const mouseEvt = {} as React.MouseEvent

describe('useStrategyMapNodeHandlers', () => {
  beforeEach(() => vi.clearAllMocks())

  describe('handleNodeClick', () => {
    it('moves the focus cursor only when tapping an unfocused node with the panel closed', () => {
      const { handlers, setFocusNodeId, setSelectedNodeId } = setup({
        effectiveFocusNodeId: 'a',
        selectedNodeId: null,
      })

      handlers.handleNodeClick(mouseEvt, nodeB)

      expect(setFocusNodeId).toHaveBeenCalledWith('b')
      // Panel stays closed: traversal must not open the detail panel.
      expect(setSelectedNodeId).not.toHaveBeenCalled()
    })

    it('opens the panel when tapping the node that is already focused', () => {
      const { handlers, setFocusNodeId, setSelectedNodeId } = setup({
        effectiveFocusNodeId: 'a',
        selectedNodeId: null,
      })

      handlers.handleNodeClick(mouseEvt, nodeA)

      expect(setFocusNodeId).toHaveBeenCalledWith('a')
      // Second tap on the focused node is the deliberate open-details gesture.
      expect(setSelectedNodeId).toHaveBeenCalledWith('a')
    })

    it('sticky-follows the panel to a new node when the panel is already open', () => {
      const { handlers, setFocusNodeId, setSelectedNodeId } = setup({
        effectiveFocusNodeId: 'a',
        selectedNodeId: 'a',
      })

      handlers.handleNodeClick(mouseEvt, nodeB)

      expect(setFocusNodeId).toHaveBeenCalledWith('b')
      expect(setSelectedNodeId).toHaveBeenCalledWith('b')
    })
  })

  describe('handleEdgeClick', () => {
    const edge: Edge = { id: 'a-b', source: 'a', target: 'b' }

    it('hops the cursor along the edge without opening the panel, centered with no offset', () => {
      const { handlers, setFocusNodeId, setSelectedNodeId, setCenter } = setup({
        effectiveFocusNodeId: 'a',
        selectedNodeId: null,
      })

      handlers.handleEdgeClick(mouseEvt, edge)

      // From the focused source 'a', the edge leads to target 'b'.
      expect(setFocusNodeId).toHaveBeenCalledWith('b')
      expect(setSelectedNodeId).not.toHaveBeenCalled()
      // Panel closed → node lands dead-center, no +110 slide-over offset.
      expect(setCenter).toHaveBeenCalledWith(100, 200, expect.objectContaining({ duration: 400 }))
    })

    it('carries the panel along and offsets for the slide-over when the panel is open', () => {
      const { handlers, setFocusNodeId, setSelectedNodeId, setCenter } = setup({
        effectiveFocusNodeId: 'a',
        selectedNodeId: 'a',
      })

      handlers.handleEdgeClick(mouseEvt, edge)

      expect(setFocusNodeId).toHaveBeenCalledWith('b')
      expect(setSelectedNodeId).toHaveBeenCalledWith('b')
      // Panel open → +110 makes room for the panel slide-over.
      expect(setCenter).toHaveBeenCalledWith(210, 200, expect.objectContaining({ duration: 400 }))
    })
  })

  describe('handlePaneClick', () => {
    it('clears both the panel and the focus cursor', () => {
      const { handlers, setFocusNodeId, setSelectedNodeId } = setup({
        effectiveFocusNodeId: 'a',
        selectedNodeId: 'a',
      })

      handlers.handlePaneClick()

      expect(setSelectedNodeId).toHaveBeenCalledWith(null)
      expect(setFocusNodeId).toHaveBeenCalledWith(null)
    })
  })
})
