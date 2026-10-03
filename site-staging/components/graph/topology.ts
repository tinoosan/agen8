import type { Node, Edge } from "@xyflow/react";
import type { Snapshot } from "../../lib/model";
import { runForceLayout, createForceLayout, type ClusterMetaEntry } from "./layout";

export function topology(snapshot: Pick<Snapshot, "nodes" | "links">) {
  const nodes: Node[] = [...snapshot.nodes].sort((a, b) => a.id.localeCompare(b.id)).map(n => ({ id: n.id, type: n.kind === "goal" ? "mission" : n.kind === "key_result" ? "keyResult" : n.kind, position: { x: 0, y: 0 }, data: {} }));
  const ids = new Set(nodes.map(n => n.id));
  const edges: Edge[] = snapshot.links.filter(l => ids.has(l.sourceId) && ids.has(l.targetId)).map(l => ({ id: l.id, source: l.sourceId, target: l.targetId, data: { relation: l.relation } })).sort((a, b) => a.id.localeCompare(b.id));
  const key = JSON.stringify([nodes.map(n => [n.id, n.type]), edges.map(e => [e.id, e.source, e.target, e.data?.relation])]);
  // Records point from work to the goal it serves. Only layout reverses this.
  const layoutEdges = edges.map(e => e.data?.relation === "serves" || e.data?.relation === "child_of" ? { ...e, source: e.target, target: e.source } : e);
  const children = new Map<string, string[]>();
  for (const e of layoutEdges) if (["serves", "child_of"].includes(String(e.data?.relation))) children.set(e.source, [...children.get(e.source) ?? [], e.target]);
  const meta = new Map<string, ClusterMetaEntry>();
  const queue = nodes.filter(n => n.type === "mission").map(n => ({ id: n.id, rank: 0 }));
  for (let i = 0; i < queue.length; i++) {
    const n = queue[i];
    if (meta.has(n.id)) continue;
    meta.set(n.id, { color: "var(--accent)", rank: n.rank });
    for (const id of children.get(n.id) ?? []) if (!meta.has(id)) queue.push({ id, rank: n.rank + 1 });
  }
  for (const n of nodes) if (!meta.has(n.id)) meta.set(n.id, { color: "var(--blue)", rank: 0 });
  return { nodes, edges, layoutEdges, meta, key };
}
const cache = new Map<string, Map<string, { x: number; y: number }>>();
export function graphPositions(graph: ReturnType<typeof topology>) {
  const hit = cache.get(graph.key);
  if (hit) { cache.delete(graph.key); cache.set(graph.key, hit); return hit; }
  const positions = runForceLayout(graph.nodes, graph.layoutEdges, graph.meta);
  remember(graph.key, positions);
  return positions;
}
export function focusedNodes(id: string | null, edges: Edge[]) {
  if (!id) return null;
  const neighbors = new Map<string, string[]>();
  for (const edge of edges) {
    neighbors.set(edge.source, [...neighbors.get(edge.source) ?? [], edge.target]);
    neighbors.set(edge.target, [...neighbors.get(edge.target) ?? [], edge.source]);
  }
  const seen = new Set([id]), queue = [id];
  for (let i = 0; i < queue.length; i++) for (const next of neighbors.get(queue[i]) ?? []) if (!seen.has(next)) { seen.add(next); queue.push(next); }
  return seen;
}

// Module-owned LRU: IDs, kinds and relationships invalidate it; content does not.
// Consumers treat completed positions as immutable and keep dragged positions separately.
function remember(key: string, positions: Map<string, { x: number; y: number }>) {
  cache.set(key, positions);
  if (cache.size > 8) cache.delete(cache.keys().next().value!);
}
export const cachedPositions = (key: string) => cache.get(key);
export async function layoutInFrames(graph: ReturnType<typeof topology>, yieldFrame: () => Promise<void>, signal: AbortSignal) {
  const hit = cache.get(graph.key);
  if (hit) { cache.delete(graph.key); cache.set(graph.key, hit); return hit; }
  const layout = createForceLayout(graph.nodes, graph.layoutEdges, graph.meta);
  let done = false;
  while (!done) {
    if (signal.aborted) return null;
    const start = performance.now();
    do { done = layout.tick(); } while (!done && performance.now() - start < 8);
    if (!done) await yieldFrame();
  }
  if (signal.aborted) return null;
  const positions = layout.positions(); remember(graph.key, positions); return positions;
}
