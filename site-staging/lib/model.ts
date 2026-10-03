export const creatableKinds = ["task", "goal", "decision"] as const;
export const kinds = [...creatableKinds, "key_result", "note"] as const;
export const statuses = ["planned", "working", "blocked", "done", "stopped"] as const;
export const relations = ["blocked_by", "serves", "informed_by", "produced", "relates_to", "supersedes", "child_of", "spawned", "resolved_by", "completed_by", "made_during"] as const;
export type Kind = typeof kinds[number];
export type Status = typeof statuses[number];
export type Project = { id: string; title: string; objective: string; status: string; createdAt: string; updatedAt: string };
export type WorkNode = { id: string; projectId: string; kind: Kind; title: string; summary: string; body: string; status: Status; blocker: string; stopReason: string; outcome: string; artifacts: string[]; version: number; createdAt: string; updatedAt: string };
export type Link = { id: string; projectId: string; sourceId: string; targetId: string; relation: string; rationale: string; createdAt: string };
export type Activity = { id: string; projectId: string; nodeId: string | null; event: string; summary: string; createdAt: string };
export type HistoryEntry = Activity & { details: { before?: WorkNode; after?: WorkNode; link?: Link; originalStatus?: string; [key: string]: unknown } };
export type HistoryPage = { entries: HistoryEntry[]; next_cursor: string | null };
export type Snapshot = { project: Project; nodes: WorkNode[]; links: Link[]; activity: Activity[]; totalNodes: number; hasMore: boolean };
export const statusLabel = (status: string) => ({ planned: "Planned", working: "Working", blocked: "Blocked", done: "Done", stopped: "Stopped" }[status] ?? status);
export const kindLabel = (kind: string) => ({ task: "Work", goal: "Goal", key_result: "Outcome", decision: "Decision", note: "Note" }[kind] ?? kind);
export function compactNode(n: WorkNode) {
  return { id: n.id, kind: n.kind, title: n.title, status: n.status, summary: n.summary, blocker: n.blocker, stopReason: n.stopReason, outcome: n.outcome, version: n.version, updatedAt: n.updatedAt };
}
export function brief(snapshot: Snapshot, since?: string) {
  const work = snapshot.nodes.filter(n => n.kind === "task" || n.kind === "goal");
  return {
    project: { id: snapshot.project.id, title: snapshot.project.title, objective: snapshot.project.objective },
    working: work.filter(n => n.status === "working").map(compactNode), blocked: work.filter(n => n.status === "blocked").map(compactNode),
    planned: work.filter(n => n.status === "planned").map(compactNode), done: work.filter(n => n.status === "done").slice(0, 15).map(compactNode),
    stopped: work.filter(n => n.status === "stopped").slice(0, 15).map(compactNode), decisions: snapshot.nodes.filter(n => n.kind === "decision").slice(0, 15).map(compactNode),
    links: snapshot.links, recent: snapshot.activity.filter(a => !since || a.createdAt > since).slice(0, 15), totalNodes: snapshot.totalNodes, hasMore: snapshot.hasMore,
  };
}
