"use client";
import { useMemo, useState } from "react";
import { ChevronRight, CircleAlert, CircleCheck, CircleDashed, CircleX, LoaderCircle } from "lucide-react";
import { statusLabel, type WorkNode } from "@/lib/model";

const shelves = [
  { key: "blocked", title: "Blocked", open: true },
  { key: "working", title: "Working", open: true },
  { key: "planned", title: "Planned", open: false },
  { key: "finished", title: "Finished", open: true },
] as const;
const stateIcons = { blocked: CircleAlert, working: LoaderCircle, planned: CircleDashed, done: CircleCheck, stopped: CircleX };
const shelfPageSize = 25;

export function WorkStateIcon({ state, size = 20 }: { state: WorkNode["status"]; size?: number }) {
  const Icon = stateIcons[state];
  return <Icon size={size} aria-hidden="true" />;
}

function WorkShelf({ shelf, nodes, selected, query, onSelect }: {
  shelf: typeof shelves[number]; nodes: WorkNode[]; selected: string | null;
  query: string; onSelect: (id: string) => void;
}) {
  const [expanded, setExpanded] = useState<boolean>(shelf.open);
  const [visibleCount, setVisibleCount] = useState(shelfPageSize);
  const selectedIndex = nodes.findIndex(n => n.id === selected);
  const containsSelection = selectedIndex >= 0;
  const hasMatches = nodes.length > 0;
  // Reveal a new selection/search while retaining manual folds across refreshes.
  const [reveal, setReveal] = useState({ selected, query, containsSelection, hasMatches });
  if (reveal.selected !== selected || reveal.query !== query || reveal.containsSelection !== containsSelection || reveal.hasMatches !== hasMatches) {
    setReveal({ selected, query, containsSelection, hasMatches });
    if (containsSelection || (query && hasMatches)) setExpanded(true);
  }
  const visible = nodes.slice(0, visibleCount);
  if (selectedIndex >= visibleCount) visible.push(nodes[selectedIndex]);
  return <details className="work-shelf" open={expanded} onToggle={e => setExpanded(e.currentTarget.open)}>
    <summary><span>{shelf.title} <span className="shelf-count">· {nodes.length}</span></span><ChevronRight size={14} aria-hidden="true" /></summary>
    {nodes.length ? <ul>{visible.map(n => {
      return <li key={n.id}><button data-static className={`work-row ${selected === n.id ? "selected" : ""}`} aria-current={selected === n.id ? "true" : undefined} onClick={() => onSelect(n.id)}>
        <span className={`work-row-icon work-state-${n.status}`}><WorkStateIcon state={n.status} /></span>
        <span className="work-row-content">
        <span className="work-row-heading"><span className="work-row-title">{n.title}</span><time dateTime={n.updatedAt} title={`Updated ${new Date(n.updatedAt).toLocaleString()}`}>{age(n.updatedAt)}</time></span>
        <span className="sr-only">{statusLabel(n.status)}</span>
        {n.status === "blocked" && n.blocker && <span className="work-row-blocker">{n.blocker}</span>}
        </span>
      </button></li>;
    })}</ul> : <p className="shelf-empty">{query ? "No matching work." : "No work in this state."}</p>}
    {nodes.length > visible.length && <button data-static className="shelf-more" onClick={() => setVisibleCount(visibleCount + shelfPageSize)}>Show more <span>· {nodes.length - visible.length} remaining</span></button>}
  </details>;
}

function age(value: string) {
  const minutes = Math.max(0, Math.floor((Date.now() - Date.parse(value)) / 60000));
  return minutes < 1 ? "now" : minutes < 60 ? `${minutes}m` : minutes < 1440 ? `${Math.floor(minutes / 60)}h` : `${Math.floor(minutes / 1440)}d`;
}

export default function WorkSidebar({ nodes, selected, query, onSelect }: {
  nodes: WorkNode[]; selected: string | null; query: string; onSelect: (id: string) => void;
}) {
  const groups = useMemo(() => {
    const grouped: Record<typeof shelves[number]["key"], WorkNode[]> = { blocked: [], working: [], planned: [], finished: [] };
    const search = query.toLowerCase();
    for (const n of nodes) {
      if (n.kind !== "task" || (search && !`${n.title} ${n.summary} ${n.body}`.toLowerCase().includes(search))) continue;
      grouped[n.status === "done" || n.status === "stopped" ? "finished" : n.status].push(n);
    }
    // Content updates should not shuffle the sidebar while someone is reading it.
    for (const group of Object.values(grouped)) group.sort((a, b) => a.title.localeCompare(b.title) || a.id.localeCompare(b.id));
    return grouped;
  }, [nodes, query]);
  return <nav className="work-shelves" aria-label="Work by state">{shelves.map(shelf => <WorkShelf key={shelf.key} shelf={shelf} nodes={groups[shelf.key]} selected={selected} query={query} onSelect={onSelect} />)}</nav>;
}
