import type { Snapshot, WorkNode } from "../lib/model";

export type ContextPayload = { content: { type: "text"; text: string; _meta?: Record<string, unknown> }[]; structuredContent: Record<string, unknown> };
export function selectedContext(snapshot: Snapshot, node: WorkNode): ContextPayload {
  const text = [`${snapshot.project.title} — ${node.title}`, `${node.kind === "task" ? "Work" : node.kind}: ${node.status} · version ${node.version}`, node.summary,
    node.blocker && `Blocked: ${node.blocker}`, node.stopReason && `Stopped: ${node.stopReason}`, node.outcome && `Result: ${node.outcome.slice(0, 500)}`].filter(Boolean).join("\n");
  return { content: [{ type: "text", text, _meta: { "openai/title": node.title } }], structuredContent: { project_id: node.projectId, node_id: node.id, version: node.version } };
}

// Host dismissal belongs to a selection, not a revision. Refreshes never restore it.
export class SelectionContext {
  private selection = "";
  private dismissed = "";
  private sent = "";
  private queue: Promise<void> = Promise.resolve();
  constructor(private update: (value: ContextPayload) => Promise<unknown>, private onError: (error: unknown) => void) {}
  dismiss() { if (this.selection) this.dismissed = this.selection; }
  select(snapshot: Snapshot | null, node: WorkNode | null, enabled: boolean) {
    const selection = snapshot && node ? `${snapshot.project.id}/${node.id}` : "";
    if (selection !== this.selection) this.dismissed = "";
    this.selection = selection;
    const payload = enabled && snapshot && node && this.selection !== this.dismissed ? selectedContext(snapshot, node) : { content: [], structuredContent: {} };
    const key = JSON.stringify(payload);
    if (key === this.sent || (!this.sent && !payload.content.length)) return;
    this.sent = key;
    this.queue = this.queue.then(async () => {
      // A later selection supersedes a queued update before it reaches the host.
      if (this.sent !== key) return;
      try { await this.update(payload); } catch (error) { if (this.sent === key) this.sent = ""; this.onError(error); }
    });
  }
  async settled() { await this.queue; }
}
