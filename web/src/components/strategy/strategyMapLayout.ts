import {
  forceSimulation,
  forceLink,
  forceManyBody,
  forceCenter,
  forceCollide,
  forceY,
  forceX,
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
  spaceName?: string
}

interface SimLink extends SimulationLinkDatum<SimNode> {
  source: string
  target: string
}

export interface ClusterMetaEntry {
  color: string
  rank: number
  spaceName?: string
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
    const spaceName = mission.data?.spaceName as string | undefined
    const colour = CLUSTER_PALETTE[i % CLUSTER_PALETTE.length]

    const visited = new Set<string>([mission.id])
    const queue = [{ id: mission.id, rank: 0 }]
    meta.set(mission.id, { color: colour, rank: 0, spaceName })

    while (queue.length > 0) {
      const { id: current, rank } = queue.shift()!
      for (const edge of edges) {
        if (edge.source === current && !visited.has(edge.target)) {
          visited.add(edge.target)
          const childRank = rank + 1
          meta.set(edge.target, { color: colour, rank: childRank, spaceName })
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

  const uniqueSpaces = Array.from(
    new Set(Array.from(meta.values()).map(m => m.spaceName).filter(Boolean)),
  ) as string[]
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

  // Adaptive space-column gap. With 2-3 spaces keep the breathing room of the
  // original 1200px layout. As space count grows, shrink sub-linearly so the
  // graph doesn't blow out of the viewport. Floor at 600px so clusters never
  // collide. Scales from 1200 (2 spaces) to roughly 850 (4) to 600 (8+).
  const spaceGap = Math.max(
    600,
    1200 / Math.max(1, Math.sqrt(Math.max(2, uniqueSpaces.length) / 2)),
  )

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
      spaceName: m?.spaceName,
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
    // Space-column X-force. Pulls missions onto horizontal columns, one
    // per space, and lightly nudges KRs and leaves toward their parent
    // mission's column so clusters stay legible side-by-side.
    //
    // CRITICAL: this force is DISABLED when there's 0 or 1 space. With a
    // single space, every mission's target x resolves to 0, every KR's
    // target x resolves to 0, every leaf's target x resolves to 0 — so
    // the force effectively collapses the whole graph onto the y-axis
    // at x=0. Combined with the initial y=rank*200 positions (all KRs
    // start at y=200, all leaves at y=200 too), the simulation lands in
    // a local minimum where the cluster is a tall thin stack at x=0
    // instead of a readable radial arrangement around the mission.
    // Disabling the X-force for single-space graphs lets the charge,
    // link, and collide forces push nodes outward radially — which is
    // what actually produces a legible strategy map for the 1-space
    // case. Multi-space graphs still get proper columnar separation.
    .force(
      'x',
      forceX<SimNode>()
        .x(d => {
          if (d.tier !== 'mission' || !d.spaceName) return 0
          const idx = uniqueSpaces.indexOf(d.spaceName)
          return (idx - (uniqueSpaces.length - 1) / 2) * spaceGap
        })
        .strength(d => {
          if (uniqueSpaces.length < 2) return 0
          if (!d.spaceName) return 0
          if (d.tier === 'mission') return 0.25
          if (d.tier === 'kr') return 0.08
          return 0.08
        }),
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
