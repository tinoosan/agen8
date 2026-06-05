import type { Node, Edge, NodeProps } from '@xyflow/react'

// ── Graph data types ──────────────────────────────────────────────────────────

export type GraphNode = Node
export type GraphEdge = Edge

// ── Registry descriptor interfaces ───────────────────────────────────────────
//
// OCP extension model: adding a new node type
// means creating a component pair and adding an entry to nodeTypeRegistry in
// registry.ts — zero changes to StrategyMap or DetailPanel.

export interface NodeTypeDescriptor {
  /** React Flow custom node component */
  component: React.ComponentType<NodeProps>
  /** Circle radius used by the force layout for collision detection and sizing */
  radius: number
  /** Detail panel rendered when this node type is selected */
  Panel: React.ComponentType<NodePanelProps>
}

export interface NodePanelProps {
  data: unknown
  projectId: string
  projectRoot?: string | null
  onClose: () => void
}

export interface EdgeTypeDescriptor {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  component: React.ComponentType<any>
}
