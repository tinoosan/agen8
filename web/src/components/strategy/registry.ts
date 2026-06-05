import type { NodeTypes, EdgeTypes } from '@xyflow/react'
import type { NodeTypeDescriptor, EdgeTypeDescriptor } from './types'
import { MissionNode } from './MissionNode'
import { KRNode } from './KRNode'
import { DecisionNode } from './DecisionNode'
import { TaskNode } from './TaskNode'
import { StatusEdge } from './StatusEdge'
import { ContextEdge } from './ContextEdge'
import { MissionPanel } from './MissionPanel'
import { KRPanel } from './KRPanel'
import { DecisionPanel } from './DecisionPanel'
import { TaskPanel } from './TaskPanel'

// ── Node type registry ────────────────────────────────────────────────────────
//
// To add a new node type to the knowledge graph (e.g. AgentNode):
//   1. Create AgentNode.tsx + AgentPanel.tsx
//   2. Add an entry here
//   3. Create useAgentNodes.ts and register it in useStrategyGraph.ts
//
// Zero changes required to StrategyMap.tsx or DetailPanel.tsx.

export const nodeTypeRegistry: Record<string, NodeTypeDescriptor> = {
  mission: {
    component: MissionNode,
    radius: 120,
    Panel: MissionPanel,
  },
  keyResult: {
    component: KRNode,
    radius: 100,
    Panel: KRPanel,
  },
  decision: {
    component: DecisionNode,
    radius: 70,
    Panel: DecisionPanel,
  },
  task: {
    component: TaskNode,
    radius: 70,
    Panel: TaskPanel,
  },
}

// ── Edge type registry ────────────────────────────────────────────────────────

export const edgeTypeRegistry: Record<string, EdgeTypeDescriptor> = {
  statusEdge: { component: StatusEdge },
  contextEdge: { component: ContextEdge },
}

// ── Derived maps for ReactFlow props ─────────────────────────────────────────
//
// These are module-level constants (stable references) — required by React Flow
// to prevent re-mounting custom node/edge components on every render.

export const reactFlowNodeTypes: NodeTypes = Object.fromEntries(
  Object.entries(nodeTypeRegistry).map(([k, v]) => [k, v.component]),
)

export const reactFlowEdgeTypes = Object.fromEntries(
  Object.entries(edgeTypeRegistry).map(([k, v]) => [k, v.component]),
) as unknown as EdgeTypes
