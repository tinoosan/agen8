import {
  forceSimulation,
  forceLink,
  forceManyBody,
  forceCenter,
  forceCollide,
  forceY,
  type SimulationNodeDatum,
  type SimulationLinkDatum,
} from 'd3-force'
import type { Node, Edge } from '@xyflow/react'
import { NODE_RADIUS, DEFAULT_RADIUS } from './nodeRadii'

// Original force layout, adapted to the hosted graph. Runs only when topology changes.

// ── Force simulation types ────────────────────────────────────────────────────

interface SimNode extends SimulationNodeDatum {
  id: string
  radius: number
  tier: 'mission' | 'kr' | 'leaf'
  rank: number
}

interface SimLink extends SimulationLinkDatum<SimNode> {
  source: string
  target: string
}

export interface ClusterMetaEntry {
  color: string
  rank: number
}

// ── Force-directed layout ─────────────────────────────────────────────────────
// Replaces dagre. Nodes attract via links and repel via charge, forming organic
// clusters around missions. The simulation runs to completion (no animation)
// and returns stable positions.
export function createForceLayout(
  nodes: Node[],
  edges: Edge[],
  meta: Map<string, ClusterMetaEntry>,
) {

  const nodeIds = new Set(nodes.map(node => node.id))

  // Compute the set of leaves attached DIRECTLY to a mission (not via a KR).
  // These need to be treated as first-class children of the mission in the
  // layout — otherwise their light leaf charge (-150) gets overwhelmed by the
  // mission's -2200 repulsion and the KR orbit's cumulative push, and they
  // drift far into empty space with no counter-force to bring them back.
  const missionIds = new Set(nodes.filter(n => n.type === 'mission').map(n => n.id))
  const nodeTypeById = new Map<string, string | undefined>(nodes.map(n => [n.id, n.type]))
  const directMissionLeafIds = new Set<string>()
  for (const edge of edges) {
    if (missionIds.has(edge.source)) {
      const targetType = nodeTypeById.get(edge.target)
      if (targetType && targetType !== 'mission' && targetType !== 'keyResult') {
        directMissionLeafIds.add(edge.target)
      }
    }
  }

  const simNodes: SimNode[] = nodes.map((n, i) => {
    const radius = NODE_RADIUS[n.type ?? ''] ?? DEFAULT_RADIUS
    const tier: SimNode['tier'] =
      n.type === 'mission' ? 'mission' : n.type === 'keyResult' ? 'kr' : 'leaf'
    const m = meta.get(n.id)
    const rank = m?.rank ?? 0
    // Initial position: missions start at (0, 0) and let the y-force hold
    // them there. Everything else gets a deterministic angular initial
    // position based on its index in the node list, so same-rank siblings
    // don't all stack at the same (0, y=rank*200) starting point. Without
    // this, a single-mission cluster starts as one tight vertical column
    // and the simulation has nothing asymmetric to break the tie on —
    // charge + collide can resolve overlaps but only find a local minimum
    // that's still a near-vertical stack. The prime-coefficient hash on
    // the index gives good spread without requiring cluster-aware grouping.
    let initialX: number | undefined
    let initialY = rank * 200
    if (tier !== 'mission') {
      const theta = (i * 2.3998) % (Math.PI * 2) // golden-angle spread
      const radialOffset = 240 + (i % 3) * 40
      initialX = Math.cos(theta) * radialOffset
      initialY = Math.sin(theta) * radialOffset + rank * 60
    }
    return {
      id: n.id,
      radius,
      tier,
      rank,
      x: initialX,
      y: initialY,
    }
  })

  const simLinks: SimLink[] = edges
    .filter(e => nodeIds.has(e.source) && nodeIds.has(e.target))
    .map(e => ({ source: e.source, target: e.target }))

  const simulation = forceSimulation<SimNode>(simNodes)
    .force(
      'link',
      forceLink<SimNode, SimLink>(simLinks)
        .id(d => d.id)
        .distance(d => {
          const src = d.source as unknown as SimNode
          const tgt = d.target as unknown as SimNode
          if (src.tier === 'mission' || tgt.tier === 'mission') return 240
          return 130
        })
        .strength(1.0),
    )
    .force(
      'charge',
      forceManyBody<SimNode>().strength(d => {
        if (d.tier === 'mission') return -1500
        if (d.tier === 'kr') return -600
        if (directMissionLeafIds.has(d.id)) return -600
        return -300
      }),
    )
    .force(
      'y',
      forceY<SimNode>()
        .y(0)
        .strength(d => (d.tier === 'mission' ? 0.05 : 0)),
    )
    .force('center', forceCenter(0, 0).strength(0.01))
    .force(
      'collide',
      forceCollide<SimNode>()
        .radius(d => d.radius + 30)
        .strength(1.0),
    )
    .stop()

  const total = Math.max(300, Math.ceil(Math.log(simulation.alphaMin()) / Math.log(1 - simulation.alphaDecay())))
  let ticks = 0
  return {
    tick() { if (ticks < total) { simulation.tick(); ticks++ }; return ticks === total },
    positions() { return new Map(simNodes.map(sn => [sn.id, { x: sn.x ?? 0, y: sn.y ?? 0 }])) },
  }
}

export function runForceLayout(nodes: Node[], edges: Edge[], meta: Map<string, ClusterMetaEntry>) {
  const layout = createForceLayout(nodes, edges, meta)
  while (!layout.tick()) { /* Complete the same simulation synchronously for tests and benchmarks. */ }
  return layout.positions()
}
