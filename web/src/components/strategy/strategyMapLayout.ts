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

/**
 * Pure force-directed layout + cluster-meta computation for the strategy
 * map. Extracted from `useStrategyGraph.ts` so the Web Worker
 * (`strategyMapLayout.worker.ts`) can import these functions without
 * dragging React or any `./registry` imports into the worker bundle.
 *
 * No React imports, no DOM APIs, no side effects — safe to run on any
 * thread.
 */

// ── Cluster colour palette ────────────────────────────────────────────────────
// Each mission cluster receives a distinct hue. KRs and leaves inherit their
// parent mission's colour. This is the primary visual identity of each cluster.
export const CLUSTER_PALETTE = [
  'var(--blue)',
  'var(--green)',
  'var(--amber)',
  'hsl(280, 55%, 65%)',   // purple
  'var(--red)',
  'hsl(175, 50%, 50%)',   // teal
  'hsl(330, 55%, 60%)',   // rose
  'hsl(200, 60%, 55%)',   // sky
]

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

// ── Cluster meta assignment ───────────────────────────────────────────────────
// Walks structural edges (BFS from each mission) to assign every node in a
// cluster the same colour, and its depth rank for strict Y-axis layout layering.
export function assignClusterMeta(
  nodes: Node[],
  edges: Edge[],
): Map<string, ClusterMetaEntry> {
  const meta = new Map<string, ClusterMetaEntry>()
  const missions = nodes.filter(n => n.type === 'mission')

  missions.forEach((mission, i) => {
    const colour = CLUSTER_PALETTE[i % CLUSTER_PALETTE.length]

    const visited = new Set<string>([mission.id])
    const queue = [{ id: mission.id, rank: 0 }]
    meta.set(mission.id, { color: colour, rank: 0 })

    while (queue.length > 0) {
      const { id: current, rank } = queue.shift()!
      for (const edge of edges) {
        if (edge.source === current && !visited.has(edge.target)) {
          visited.add(edge.target)
          const childRank = rank + 1
          meta.set(edge.target, { color: colour, rank: childRank })
          queue.push({ id: edge.target, rank: childRank })
        }
      }
    }
  })

  for (const n of nodes) {
    if (!meta.has(n.id)) {
      meta.set(n.id, { color: 'var(--border-strong)', rank: 0 })
    }
  }

  return meta
}

// ── Force-directed layout ─────────────────────────────────────────────────────
// Replaces dagre. Nodes attract via links and repel via charge, forming organic
// clusters around missions. The simulation runs to completion (no animation)
// and returns stable positions.
export function runForceLayout(
  nodes: Node[],
  edges: Edge[],
  meta: Map<string, ClusterMetaEntry>,
): Map<string, { x: number; y: number }> {
  if (nodes.length === 0) return new Map()

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

  // Run to completion (300 ticks is more than enough for convergence)
  const n = Math.ceil(Math.log(simulation.alphaMin()) / Math.log(1 - simulation.alphaDecay()))
  for (let i = 0; i < Math.max(n, 300); i++) simulation.tick()

  return new Map(simNodes.map(sn => [sn.id, { x: sn.x ?? 0, y: sn.y ?? 0 }]))
}
