import test from "node:test";
import assert from "node:assert/strict";
import { topology, graphPositions, focusedNodes } from "../components/graph/topology";
import type { WorkNode, Link } from "../lib/model";
const node = (id: string, kind: WorkNode["kind"] = "task"): WorkNode => ({ id, kind, title: id, projectId: "p", summary: "", body: "", status: "done", blocker: "", stopReason: "", outcome: "", artifacts: [], version: 1, createdAt: "", updatedAt: "" });
const link = (id: string, sourceId: string, targetId: string, relation = "serves"): Link => ({ id, sourceId, targetId, relation, projectId: "p", rationale: "", createdAt: "" });
test("standalone records, shared nodes under multiple goals, dependencies and cycles have finite deterministic positions", () => {
  const nodes = [node("goal1", "goal"), node("goal2", "goal"), node("shared"), node("standalone"), node("choice", "decision")];
  const links = [link("a", "shared", "goal1"), link("b", "shared", "goal2"), link("c", "choice", "shared", "informed_by"), link("d", "goal1", "shared", "child_of")];
  const graph = topology({ nodes, links }), positions = graphPositions(graph);
  assert.equal(positions.size, 5); assert.equal(graph.nodes.length, 5);
  assert.deepEqual(graph.layoutEdges.find(e => e.id === "a")?.source, "goal1"); assert.equal(graph.edges.find(e => e.id === "a")?.source, "shared");
  for (const p of positions.values()) { assert(Number.isFinite(p.x)); assert(Number.isFinite(p.y)); }
  assert.deepEqual(graphPositions(topology({ nodes: [...nodes].reverse(), links: [...links].reverse() })), positions);
  assert.deepEqual(focusedNodes("standalone", graph.edges), new Set(["standalone"])); assert.equal(focusedNodes("shared", graph.edges)?.size, 4);
});
test("content, completion and refresh ordering reuse layout; topology changes invalidate it", () => {
  const nodes = [node("work"), node("decision", "decision")], links = [link("edge", "work", "decision", "informed_by")];
  const original = topology({ nodes, links }), positions = graphPositions(original);
  const changed = topology({ nodes: nodes.map(n => ({ ...n, title: "New title", status: "working", version: 2 })), links: links.map(l => ({ ...l, rationale: "New explanation" })) });
  assert.equal(original.key, changed.key); assert.equal(positions, graphPositions(changed));
  const extra = topology({ nodes: [...nodes, node("new")], links }); assert.notEqual(extra.key, original.key); assert.equal(graphPositions(extra).size, 3);
});
test("frame batching preserves force results and cancels obsolete layouts", async () => {
  const { layoutInFrames } = await import("../components/graph/topology");
  const nodes = Array.from({ length: 100 }, (_, i) => node(`batched${i}`, i === 0 ? "goal" : "task"));
  const graph = topology({ nodes, links: nodes.slice(1).map(n => link(n.id, n.id, nodes[0].id)) });
  const cancelled = new AbortController(); cancelled.abort();
  assert.equal(await layoutInFrames(graph, async () => {}, cancelled.signal), null);
  let yields = 0;
  const batched = await layoutInFrames(graph, async () => { yields++; }, new AbortController().signal);
  assert(yields > 0);
  const { runForceLayout } = await import("../components/graph/layout");
  assert.deepEqual(batched, runForceLayout(graph.nodes, graph.layoutEdges, graph.meta));
});
