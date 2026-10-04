"use client";
/* eslint-disable @next/next/no-img-element -- the shared Site/MCP icon supports data URLs and requires no image optimizer */
import { useCallback, useEffect, useRef, useState } from "react";
import { parseGraphPath, type GraphSelection } from "@/lib/navigation";
import { Search, X, ArrowUpRight, ArrowLeft, ArrowRight, RefreshCw, Network, LoaderCircle, UserRound } from "lucide-react";
import { kindLabel, statusLabel, statuses, type Project, type Snapshot, type WorkNode, type HistoryEntry, type HistoryPage } from "@/lib/model";
import Graph from "./graph/graph";
import WorkSidebar, { WorkStateIcon } from "./work-sidebar";

async function read<T>(url: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(url, { signal, cache: "no-store" });
  const data = await response.json() as T & { error?: string };
  if (!response.ok) throw new Error(data.error || "Could not load your work.");
  return data;
}
export type WorkspaceReader = typeof read;
function age(value: string) {
  const minutes = Math.max(0, Math.floor((Date.now() - Date.parse(value)) / 60000));
  return minutes < 1 ? "just now" : minutes < 60 ? `${minutes}m ago` : minutes < 1440 ? `${Math.floor(minutes / 60)}h ago` : `${Math.floor(minutes / 1440)}d ago`;
}
function timestamp(value: string) { return new Date(value).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" }); }
function safeUrl(value: string) { try { const u = new URL(value); return ["https:", "http:"].includes(u.protocol) ? u.href : null; } catch { return null; } }
function Evidence({ items }: { items: string[] }) { return <>{items.map((item, i) => <p key={i}>{safeUrl(item) ? <a href={safeUrl(item)!} target="_blank" rel="noreferrer">{item}<ArrowUpRight size={14} /></a> : <code>{item}</code>}</p>)}</>; }
const historyFields = ["title", "summary", "body", "status", "blocker", "stopReason", "outcome", "artifacts"] as const;
const fieldLabel = { title: "Title", summary: "Explanation", body: "Context", status: "State", blocker: "Blocker", stopReason: "Stopped because", outcome: "Result and checks", artifacts: "Evidence" };
function HistoryRecord({ entry, nodes }: { entry: HistoryEntry; nodes: WorkNode[] }) {
  const { before, after, link, originalStatus } = entry.details;
  return <details className="history-entry"><summary>{entry.summary}<time dateTime={entry.createdAt}>{timestamp(entry.createdAt)}</time></summary><div className="history-fields">
    {originalStatus && <p>Earlier state: {originalStatus}</p>}
    {after && historyFields.filter(field => !before || JSON.stringify(before[field]) !== JSON.stringify(after[field])).map(field => <div key={field}><h4>{fieldLabel[field]}</h4>{field === "artifacts" ? <><Evidence items={after.artifacts} />{before?.artifacts.length ? <details><summary>Previously</summary><Evidence items={before.artifacts} /></details> : null}</> : <><p>{String(after[field]) || "Cleared"}</p>{before && before[field] && <details><summary>Previously</summary><p>{String(before[field])}</p></details>}</>}</div>)}
    {link && <p>{nodes.find(n => n.id === link.sourceId)?.title ?? link.sourceId} → {nodes.find(n => n.id === link.targetId)?.title ?? link.targetId}<br />{link.relation.replaceAll("_", " ")}{link.rationale && `: ${link.rationale}`}</p>}
    {!after && !link && !originalStatus && <p>Recorded {timestamp(entry.createdAt)}.</p>}
  </div></details>;
}
function Detail({ node, snapshot, onClose, onSelect, reader }: { node: WorkNode; snapshot: Snapshot; onClose: () => void; onSelect: (id: string) => void; reader: WorkspaceReader }) {
  const [history, setHistory] = useState<HistoryPage | null>(null), [expanded, setExpanded] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState("");
  const request = useRef<AbortController | null>(null);
  const close = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const previous = document.activeElement; close.current?.focus();
    return () => { request.current?.abort(); if (previous instanceof HTMLElement && previous.isConnected) previous.focus(); };
  }, []);
  const loadHistory = useCallback(async (cursor?: string) => {
    request.current?.abort(); const controller = new AbortController(); request.current = controller;
    setBusy(true); setError("");
    try {
      const page = await reader<HistoryPage>(`/api/work?project=${encodeURIComponent(node.projectId)}&node=${encodeURIComponent(node.id)}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`, controller.signal);
      setHistory(current => cursor && current ? { entries: [...current.entries, ...page.entries], next_cursor: page.next_cursor } : page);
    } catch (e) { if (!controller.signal.aborted) setError((e as Error).message); } finally { if (!controller.signal.aborted) setBusy(false); }
  }, [node.id, node.projectId, reader]);
  // Fresh current records also refresh an already-open history, including new links.
  const latest = snapshot.activity.find(a => a.nodeId === node.id)?.id;
  useEffect(() => { if (expanded) { const controller = new AbortController(); void Promise.resolve().then(() => { if (!controller.signal.aborted) return loadHistory(); }); return () => controller.abort(); } }, [expanded, node.version, latest, loadHistory]);
  const connections = snapshot.links.filter(l => l.sourceId === node.id || l.targetId === node.id);
  return <aside className="detail-panel" aria-label="Node details" onKeyDown={e => { if (e.key === "Escape") onClose(); }}><div className="detail-heading"><span>{kindLabel(node.kind)}</span><button ref={close} className="icon-button" onClick={onClose} aria-label="Close details"><X size={20} /></button></div><h2>{node.title}</h2><div className="detail-meta"><span className={`detail-state status-${node.status}`}><WorkStateIcon state={node.status} />{node.kind === "decision" ? "Recorded" : statusLabel(node.status)}</span><time dateTime={node.updatedAt} title={timestamp(node.updatedAt)}>Updated {age(node.updatedAt)}</time></div>
    {node.summary && <section className="detail-section"><h3>Current explanation</h3><p className="detail-copy">{node.summary}</p></section>}
    {node.body && <section className="detail-section"><h3>{node.kind === "decision" ? "Reasoning" : "Context"}</h3><p className="detail-copy">{node.body}</p></section>}
    {node.blocker && <section className="detail-section"><h3>Blocker</h3><p className="detail-copy blocked-label">{node.blocker}</p></section>}
    {node.stopReason && <section className="detail-section"><h3>Stopped because</h3><p className="detail-copy">{node.stopReason}</p></section>}
    {node.outcome && <section className="detail-section"><h3>Result and checks</h3><p className="detail-copy">{node.outcome}</p></section>}
    {node.artifacts.length > 0 && <section className="detail-section"><h3>Evidence</h3><Evidence items={node.artifacts} /></section>}
    <section className="detail-section"><h3>Connections</h3>{connections.length ? connections.map(l => { const otherId = l.sourceId === node.id ? l.targetId : l.sourceId; const other = snapshot.nodes.find(n => n.id === otherId); return <div key={l.id}><button data-static className="connection-row" onClick={() => onSelect(otherId)}><span className="connection-direction" aria-hidden="true">{l.sourceId === node.id ? <ArrowRight size={22} /> : <ArrowLeft size={22} />}</span><span className="connection-copy"><span>{l.sourceId === node.id ? l.relation.replaceAll("_", " ") : `Incoming ${l.relation.replaceAll("_", " ")}`}</span><strong>{other?.title ?? otherId}</strong></span></button>{l.rationale && <p className="muted">{l.rationale}</p>}</div>; }) : <p className="muted">No connections.</p>}</section>
    <section className="detail-section"><details open={expanded} onToggle={e => setExpanded(e.currentTarget.open)}><summary>Earlier changes</summary>{history?.entries.map(entry => <HistoryRecord key={entry.id} entry={entry} nodes={snapshot.nodes} />)}{history && !history.entries.length && !busy && !error && <p className="muted">No earlier changes recorded.</p>}{busy && <p className="muted" role="status">Loading history…</p>}{error && <p className="history-error" role="alert"><span>{error}</span><button onClick={() => void loadHistory()}>Retry</button></p>}{history?.next_cursor && <button className="history-more" disabled={busy} onClick={() => void loadHistory(history.next_cursor!)}>More history</button>}</details></section><footer className="record-meta"><p>{kindLabel(node.kind)} node · v{node.version}</p><time dateTime={node.updatedAt}>Updated {timestamp(node.updatedAt)}</time><details><summary>Record ID</summary><p className="record-id">{node.id}</p></details></footer>
  </aside>;
}
export default function Workspace({ displayName, reader = read, navigation, onSelection, iconSrc = "/favicon.svg", brandHref = "/" }: {
  displayName: string; reader?: WorkspaceReader; navigation?: GraphSelection;
  onSelection?: (snapshot: Snapshot | null, node: WorkNode | null) => void;
  iconSrc?: string; brandHref?: string;
}) {
  const [projects, setProjects] = useState<Project[]>([]), [projectId, setProjectId] = useState(""), [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [selected, setSelected] = useState<string | null>(null), [query, setQuery] = useState(""), [kind, setKind] = useState(""), [state, setState] = useState("");
  const [error, setError] = useState(""), [loading, setLoading] = useState(true), [refreshing, setRefreshing] = useState(false);
  const generation = useRef(0);
  const activatedNavigation = useRef<string | null>(null);
  const loadProjects = useCallback(async () => {
    return reader<{ projects: Project[] }>("/api/work").then(value => {
      setProjects(value.projects); setProjectId(current => value.projects.some(p => p.id === current) ? current : value.projects[0]?.id ?? ""); return value;
    });
  }, [reader]);
  useEffect(() => { let live = true; void loadProjects().then(value => { if (live && !value.projects.length) setLoading(false); }).catch(e => { if (live) { setError(e.message); setLoading(false); } }); return () => { live = false; }; }, [loadProjects]);
  const refresh = useCallback(async (manual = false) => {
    if (!projectId) return;
    const token = generation.current;
    if (manual) setRefreshing(true);
    try { const value = await reader<Snapshot>(`/api/work?project=${encodeURIComponent(projectId)}`); if (token === generation.current) { setSnapshot(value); setError(""); } }
    catch (e) { if (token === generation.current) setError((e as Error).message); }
    finally { if (token === generation.current) { setLoading(false); setRefreshing(false); } }
  }, [projectId, reader]);
  useEffect(() => {
    generation.current++;
    if (projectId) void Promise.resolve().then(() => refresh());
    const interval = setInterval(() => { if (document.visibilityState === "visible") { void refresh(); void loadProjects().catch(() => {}); } }, 10000);
    return () => { clearInterval(interval); };
  }, [projectId, refresh, loadProjects]);
  const node = snapshot?.nodes.find(n => n.id === selected);
  useEffect(() => {
    let live = true;
    void Promise.resolve().then(() => {
    if (!live) return;
    const target = navigation ?? parseGraphPath(window.location.pathname + window.location.search);
    const key = JSON.stringify(target);
    if (!target || activatedNavigation.current === key) return;
    if (!target.projectId) { activatedNavigation.current = key; setSelected(null); return; }
    if (!projects.length) return;
    if (!projects.some(p => p.id === target.projectId)) { activatedNavigation.current = key; setError("The linked project is unavailable in this account."); return; }
    if (projectId !== target.projectId) { generation.current++; setProjectId(target.projectId); setSnapshot(null); setSelected(null); setLoading(true); return; }
    if (snapshot?.project.id === target.projectId) {
      activatedNavigation.current = key;
      if (target.nodeId) {
        if (snapshot.nodes.some(n => n.id === target.nodeId)) setSelected(target.nodeId);
        else setError("The linked node is unavailable in this graph view.");
      } else setSelected(null);
    }
    });
    return () => { live = false; };
  // Activate a link once; refreshing a snapshot must not reopen dismissed details.
  }, [navigation, projects, projectId, snapshot]);
  useEffect(() => { onSelection?.(snapshot, node ?? null); }, [snapshot, node, onSelection]);
  const selectNode = (id: string | null) => { setSelected(id); };
  const focusWork = (id: string) => {
    setQuery(""); setKind(""); setState(""); setSelected(id);
  };
  return <div className={`workspace shared-workspace ${node ? "with-detail" : ""}`}>
    <header className="workspace-header">
      <a className="brand" href={brandHref}><img src={iconSrc} alt="" width={28} height={28} /><strong>agen8</strong></a>
      <label className="project-label"><span className="sr-only">Project</span><select aria-label="Project" value={projectId} title={snapshot?.project.objective || undefined} onChange={e => { generation.current++; setProjectId(e.target.value); setSnapshot(null); setSelected(null); setQuery(""); setKind(""); setState(""); setLoading(true); }}>
        <option value="" disabled>Choose a project</option>{projects.map(p => <option value={p.id} key={p.id}>{p.title}{p.status === "archived" ? " · archived" : ""}</option>)}
      </select></label>
      <div className="header-actions">
        {snapshot && <div className="graph-filters">
          <label>Type<select aria-label="Node type" value={kind} onChange={e => setKind(e.target.value)}><option value="">All types</option>{["task", "goal", "decision", "key_result", "note"].filter(k => ["task", "goal", "decision"].includes(k) || snapshot.nodes.some(n => n.kind === k)).map(k => <option key={k} value={k}>{kindLabel(k)}</option>)}</select></label>
          <label>State<select aria-label="Work state" value={state} onChange={e => setState(e.target.value)}><option value="">All states</option>{statuses.map(s => <option key={s} value={s}>{statusLabel(s)}</option>)}</select></label>
        </div>}
        <button className="icon-button" aria-label="Refresh work" aria-busy={refreshing} disabled={refreshing} onClick={() => { void loadProjects().catch(e => setError(e.message)); void refresh(true); }}><span className={`state-icon ${refreshing ? "is-busy" : ""}`} style={{ width: 18, height: 18 }} aria-hidden="true"><span className="state-icon-default"><RefreshCw size={18} /></span><span className="state-icon-busy"><LoaderCircle size={18} className="spinning" /></span></span></button>
      </div>
    </header>
    <aside className="sidebar">
      <label className="search sidebar-search"><Search size={18} aria-hidden="true" /><input aria-label="Search work or decisions" placeholder="Search work or decisions…" value={query} onChange={e => setQuery(e.target.value)} /></label>
      {snapshot && <WorkSidebar key={snapshot.project.id} nodes={snapshot.nodes} selected={selected} query={query} onSelect={focusWork} />}
      <div className="sidebar-bottom"><UserRound size={20} aria-hidden="true" /><div><span className="signed-in">{displayName}</span><span className="account-caption">Signed in</span></div></div>
    </aside>
    <main className="main-work" aria-label={snapshot?.project.title || "Your workspace"}>
      {error && <div className="error-banner" role="alert"><span>{error}</span><button onClick={() => { void loadProjects().catch(e => setError(e.message)); void refresh(true); }}>Retry</button></div>}
      {loading ? <div className="empty-state" role="status"><LoaderCircle className="spinning" />Loading work…</div> : snapshot ? <>
        {snapshot.hasMore && <p className="muted graph-limit">Showing 1,000 of {snapshot.totalNodes} records, with working and blocked items first.</p>}
        <Graph key={snapshot.project.id} snapshot={snapshot} selected={selected} onSelect={selectNode} query={query} kind={kind} state={state} />
      </> : !error && <div className="empty-state"><Network size={40} /><h2>No projects yet</h2><p>Ask an agent to use Agen8 to organise your project. Its work and decisions will appear here.</p></div>}
    </main>
    {node && snapshot && <Detail key={node.id} node={node} snapshot={snapshot} reader={reader} onClose={() => selectNode(null)} onSelect={id => { setQuery(""); setKind(""); setState(""); selectNode(id); }} />}
  </div>;
}
