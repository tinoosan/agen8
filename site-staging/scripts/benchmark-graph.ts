import { performance } from "node:perf_hooks";
import { topology, graphPositions, layoutInFrames } from "../components/graph/topology";
import type { WorkNode, Link } from "../lib/model";

// First project open, identical refresh, and frame fairness at the snapshot cap.
for (const count of [20, 100, 500, 1000]) {
  for (let sample = 0; sample < 3; sample++) {
    const nodes = Array.from({ length: count }, (_, i) => ({ id: `${count}-${sample}-n${i}`, kind: i % 40 === 0 ? "goal" : i % 7 === 0 ? "decision" : "task" })) as WorkNode[];
    const links = nodes.filter((_, i) => i % 40 !== 0).map((n, i) => ({ id: `${sample}-l${i}`, sourceId: n.id, targetId: `${count}-${sample}-n${Math.floor(Number(n.id.split("n")[1]) / 40) * 40}`, relation: "serves" })) as Link[];
    const graph = topology({ nodes, links });
    const started = performance.now(); graphPositions(graph); const coldMs = performance.now() - started;
    const cached = performance.now(); graphPositions(graph); const cachedMs = performance.now() - cached;
    // A fresh key is necessary to measure batching rather than the completed cache.
    const fresh = topology({ nodes: nodes.map(n => ({ ...n, id: `async-${n.id}` })), links: links.map(l => ({ ...l, sourceId: `async-${l.sourceId}`, targetId: `async-${l.targetId}` })) });
    let mark = performance.now(), maxBatchMs = 0, yields = 0;
    await layoutInFrames(fresh, async () => { maxBatchMs = Math.max(maxBatchMs, performance.now() - mark); yields++; await new Promise(resolve => setImmediate(resolve)); mark = performance.now(); }, new AbortController().signal);
    maxBatchMs = Math.max(maxBatchMs, performance.now() - mark);
    console.log(JSON.stringify({ nodes: count, sample, coldMs: +coldMs.toFixed(2), cachedMs: +cachedMs.toFixed(3), maxBatchMs: +maxBatchMs.toFixed(2), yields }));
  }
}
