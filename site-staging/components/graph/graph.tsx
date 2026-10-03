"use client";
import { memo, useCallback, useEffect, useMemo, useState } from "react";
import { Background, Controls, Handle, Position, ReactFlow, ReactFlowProvider, useReactFlow, useOnViewportChange, MarkerType, type Node, type NodeProps, type Edge, type NodeChange } from "@xyflow/react";
import { Diamond, Target, Network } from "lucide-react";
import "@xyflow/react/dist/style.css";
import { kindLabel, statusLabel, type Snapshot, type WorkNode } from "../../lib/model";
import { focusedNodes, cachedPositions, layoutInFrames, topology } from "./topology";
import "./graph.css";

type Data = { work: WorkNode; dimmed: boolean; compact: boolean };
const tones = { planned: "var(--muted)", working: "var(--blue)", blocked: "var(--gold)", done: "var(--green)", stopped: "var(--red)" };
function Ring({ state }: { state: WorkNode["status"] }) {
  const progress = { planned: 0, working: .45, blocked: .3, done: 1, stopped: 0 }[state], circumference = 2 * Math.PI * 7;
  return <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true"><circle cx="9" cy="9" r="7" fill="none" stroke="currentColor" strokeWidth="2" opacity=".3" />{progress > 0 && <circle cx="9" cy="9" r="7" fill="none" stroke="currentColor" strokeWidth="2" strokeDasharray={circumference} strokeDashoffset={circumference * (1 - progress)} strokeLinecap="round" transform="rotate(-90 9 9)" />}{state === "done" && <polyline points="6.5,9.5 8.5,11.5 12,7.5" fill="none" stroke="currentColor" strokeWidth="1.5" />}{state === "blocked" && <><path d="M9 6v4" stroke="currentColor" strokeWidth="1.5" /><circle cx="9" cy="12.5" r=".8" fill="currentColor" /></>}{state === "planned" && <path d="M7 6.5v5M11 6.5v5" stroke="currentColor" strokeWidth="1.5" />}{state === "stopped" && <path d="m6.5 6.5 5 5m0-5-5 5" stroke="currentColor" strokeWidth="1.5" />}</svg>;
}
const GraphNode = memo(function GraphNode({ data, selected }: NodeProps<Node<Data>>) {
  const n = data.work, goal = n.kind === "goal", legacy = n.kind === "key_result", decision = n.kind === "decision";
  const compact = data.compact && !selected;
  return <div className={`shared-node kind-${n.kind} ${selected ? "focused" : ""} ${data.dimmed ? "dimmed" : ""} ${compact ? "macro" : ""}`} style={{ color: decision ? "var(--green)" : goal ? "var(--accent)" : tones[n.status] }} title={`${n.title} · ${decision ? "Decision" : statusLabel(n.status)}`}>
    <Handle type="target" position={Position.Top} /><Handle type="source" position={Position.Bottom} />
    {compact ? <span className={`macro-glyph ${decision ? "diamond" : ""}`} /> : <>{decision ? <Diamond size={16} fill={selected ? "currentColor" : "none"} /> : goal || legacy ? <Target size={18} /> : <Ring state={n.status} />}<strong>{n.title}</strong><span className="node-state">{decision ? "Decision" : `${goal || legacy ? `${kindLabel(n.kind)} · ` : ""}${statusLabel(n.status)}`}</span></>}
  </div>;
});
const nodeTypes = { record: GraphNode };
function Canvas({ snapshot, selected, onSelect, query, kind, state }: { snapshot: Snapshot; selected: string | null; onSelect: (id: string | null) => void; query: string; kind: string; state: string }) {
  const graph = useMemo(() => topology(snapshot), [snapshot]);
  const [layout, setLayout] = useState<{ key: string; positions: Map<string, { x: number; y: number }> } | null>(null);
  const positions = layout?.key === graph.key ? layout.positions : cachedPositions(graph.key);
  useEffect(() => {
    const controller = new AbortController();
    void layoutInFrames(graph, () => new Promise(resolve => requestAnimationFrame(() => resolve())), controller.signal).then(value => { if (value && !controller.signal.aborted) setLayout({ key: graph.key, positions: value }); });
    return () => controller.abort();
  }, [graph]);
  const [moved, setMoved] = useState<Record<string, { x: number; y: number }>>({});
  const [dimensions, setDimensions] = useState<Record<string, { width: number; height: number }>>({});
  const [compact, setCompact] = useState(false);
  useOnViewportChange({ onChange: viewport => setCompact(viewport.zoom < .38) });
  const flow = useReactFlow();
  const focus = useMemo(() => focusedNodes(selected, graph.edges), [selected, graph.edges]);
  const visible = snapshot.nodes.filter(n => (!kind || n.kind === kind) && (!state || n.status === state) && (!query || `${n.title} ${n.summary} ${n.body}`.toLowerCase().includes(query.toLowerCase())));
  const ids = new Set(visible.map(n => n.id));
  const nodes: Node<Data>[] = visible.map(n => ({ id: n.id, type: "record", position: moved[n.id] ?? positions?.get(n.id) ?? { x: 0, y: 0 }, selected: n.id === selected, measured: dimensions[n.id], ariaLabel: `${kindLabel(n.kind)}: ${n.title}, ${statusLabel(n.status)}`, data: { work: n, dimmed: !!focus && !focus.has(n.id), compact } }));
  const edges: Edge[] = graph.edges.filter(e => ids.has(e.source) && ids.has(e.target)).map(e => ({ ...e, type: "default", markerEnd: { type: MarkerType.ArrowClosed, color: e.data?.relation === "blocked_by" ? "#e8c57b" : "#68718b" }, label: String(e.data?.relation).replaceAll("_", " "), style: { stroke: e.data?.relation === "blocked_by" ? "var(--gold)" : "#68718b", strokeWidth: selected === e.source || selected === e.target ? 2 : 1, opacity: focus && (!focus.has(e.source) || !focus.has(e.target)) ? .15 : .65 }, labelStyle: { fill: "#a1a8ba", fontSize: 11 }, labelBgStyle: { fill: "#10141d" }, labelBgPadding: [5, 3] }));
  const move = useCallback((changes: NodeChange[]) => {
    for (const change of changes) if (change.type === "select" && change.selected) onSelect(change.id);
    setDimensions(previous => {
      let next = previous;
      for (const change of changes) if (change.type === "dimensions" && change.dimensions && (previous[change.id]?.width !== change.dimensions.width || previous[change.id]?.height !== change.dimensions.height)) { if (next === previous) next = { ...previous }; next[change.id] = change.dimensions; }
      return next;
    });
    setMoved(previous => {
      let next = previous;
      for (const change of changes) if (change.type === "position" && change.position) { if (next === previous) next = { ...previous }; next[change.id] = change.position; }
      return next;
    });
  }, [onSelect]);
  useEffect(() => {
    if (!positions) return;
    const frame = requestAnimationFrame(() => { void flow.fitView({ maxZoom: 1, padding: .2, duration: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? 0 : 150 }); });
    return () => cancelAnimationFrame(frame);
  }, [query, kind, state, positions, flow]);
  useEffect(() => {
    if (!selected) return;
    const n = flow.getNode(selected);
    if (n) void flow.setCenter(n.position.x + (n.measured?.width ?? 190) / 2, n.position.y + (n.measured?.height ?? 60) / 2, { zoom: Math.max(.85, flow.getZoom()), duration: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? 0 : 300 });
  }, [selected, flow]);
  return <section className="graph-section shared-graph" aria-label="Shared work graph"><div className="section-heading"><div><Network size={16} /><h2>Work graph</h2><span>{visible.length} {visible.length === 1 ? "item" : "items"}</span></div>{selected && <button data-static className="clear-focus" onClick={() => onSelect(null)}>Clear focus</button>}</div><div className="graph-canvas" onKeyDown={e => { if (e.key === "Escape") onSelect(null); }}>
    {nodes.length && !positions ? <div className="canvas-empty" role="status">Arranging graph…</div> : nodes.length ? <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} onNodesChange={move} onNodeClick={(_, n) => onSelect(n.id)} onPaneClick={() => onSelect(null)} fitView fitViewOptions={{ maxZoom: 1, padding: .25 }} minZoom={.08} maxZoom={2} nodesDraggable nodesConnectable={false} edgesFocusable={false} onlyRenderVisibleElements proOptions={{ hideAttribution: true }}><Background color="#31394a" gap={24} /><Controls showInteractive={false} /></ReactFlow> : <div className="canvas-empty"><Network size={32} /><h3>{snapshot.nodes.length ? "No matching work" : "No work recorded yet"}</h3><p>{snapshot.nodes.length ? "Try another search or filter." : "Work and decisions will appear here as they are recorded through Agen8."}</p></div>}
  </div><div className="graph-legend"><span className="legend-goal">Goal</span><span className="legend-task">Work</span><span className="legend-decision">Decision</span><span>Click for context. Drag to move; scroll to zoom.</span></div></section>;
}
export default function Graph(props: Parameters<typeof Canvas>[0]) { return <ReactFlowProvider><Canvas {...props} /></ReactFlowProvider>; }
