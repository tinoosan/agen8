import { z } from "zod";
import { brief, compactNode, type Project, type WorkNode, type Link, type Activity, type Snapshot, type HistoryPage } from "./model";
import { WorkError, idSchema, nodeInput, nodeUpdate, linkInput, validateState } from "./validation";

type Row = Record<string, unknown>;
type Args = Record<string, unknown>;
const now = () => new Date().toISOString();
const newId = (prefix: string) => `${prefix}_${crypto.randomUUID()}`;
const str = (value: unknown) => String(value ?? "");
const projectRow = (r: Row): Project => ({ id: str(r.id), title: str(r.title), objective: str(r.objective), status: str(r.status), createdAt: str(r.created_at), updatedAt: str(r.updated_at) });
const linkRow = (r: Row): Link => ({ id: str(r.id), projectId: str(r.project_id), sourceId: str(r.source_id), targetId: str(r.target_id), relation: str(r.relation), rationale: str(r.rationale), createdAt: str(r.created_at) });
const activityRow = (r: Row): Activity => ({ id: str(r.id), projectId: str(r.project_id), nodeId: r.node_id ? str(r.node_id) : null, event: str(r.event), summary: r.agent_id || str(r.summary).startsWith("You:") ? str(r.summary).replace(/^[^:]+:\s*/, "") : str(r.summary), createdAt: str(r.created_at) });
function nodeRow(r: Row): WorkNode {
  return { id: str(r.id), projectId: str(r.project_id), kind: str(r.kind) as WorkNode["kind"], title: str(r.title), summary: str(r.summary), body: str(r.body), status: str(r.status) as WorkNode["status"], blocker: str(r.blocker), stopReason: str(r.stop_reason), outcome: str(r.outcome), artifacts: JSON.parse(str(r.artifacts) || "[]"), version: Number(r.version), createdAt: str(r.created_at), updatedAt: str(r.updated_at) };
}

// All durable mutations and their history commit in the same D1 batch.
// MCP writes use the connected account; there is no agent identity or ownership.
export class Work {
  constructor(private db: D1Database, private owner: string) {
    if (!owner) throw new WorkError("Sign in to access your work.", 401);
  }
  private sql(query: string, ...values: unknown[]) { return this.db.prepare(query).bind(...values); }
  async projects() {
    const rows = await this.sql("SELECT * FROM projects WHERE owner_id=? ORDER BY updated_at DESC", this.owner).all<Row>();
    return rows.results.map(projectRow);
  }
  async preferences() {
    const row = await this.sql("SELECT share_selected_context FROM preferences WHERE owner_id=?", this.owner).first<Row>();
    return { shareSelectedContext: row ? !!row.share_selected_context : true };
  }
  async updatePreferences(args: Args) {
    const input = z.object({ set: z.object({ shareSelectedContext: z.boolean() }).strict() }).strict().parse(args);
    await this.sql("INSERT INTO preferences (owner_id,share_selected_context) VALUES (?,?) ON CONFLICT(owner_id) DO UPDATE SET share_selected_context=excluded.share_selected_context", this.owner, Number(input.set.shareSelectedContext)).run();
    return this.preferences();
  }
  async mentions(query: string) {
    const value = z.string().max(200).parse(query).trim();
    // Literal typeahead text, bounded on the database side and scoped before searching.
    const escaped = `%${value.replace(/[\\%_]/g, c => `\\${c}`)}%`;
    const result = await this.sql(`SELECT * FROM (
      SELECT p.id AS project_id,NULL AS node_id,p.title,p.objective AS summary,'project' AS kind,p.updated_at
      FROM projects p WHERE p.owner_id=? AND (p.title LIKE ? ESCAPE '\\' OR p.objective LIKE ? ESCAPE '\\')
      UNION ALL
      SELECT n.project_id,n.id AS node_id,n.title,n.summary,n.kind,n.updated_at
      FROM nodes n JOIN projects p ON p.id=n.project_id WHERE p.owner_id=? AND (n.title LIKE ? ESCAPE '\\' OR n.summary LIKE ? ESCAPE '\\')
    ) ORDER BY updated_at DESC,project_id,node_id LIMIT 20`, this.owner, escaped, escaped, this.owner, escaped, escaped).all<Row>();
    return result.results.map(r => ({ projectId: str(r.project_id), nodeId: r.node_id ? str(r.node_id) : undefined, title: str(r.title), summary: str(r.summary), kind: str(r.kind) }));
  }
  async project(id: string) {
    const row = await this.sql("SELECT * FROM projects WHERE id=? AND owner_id=?", idSchema.parse(id), this.owner).first<Row>();
    if (!row) throw new WorkError("Project not found.", 404);
    return projectRow(row);
  }
  private async editableProject(id: string) {
    const project = await this.project(id);
    if (project.status !== "open") throw new WorkError("Reopen this project before changing its graph.");
    return project;
  }
  async createProject(args: Args) {
    const input = z.object({ title: z.string().trim().min(1).max(180), objective: z.string().trim().max(1200).default("") }).parse(args);
    const id = newId("project"), at = now();
    await this.db.batch([
      this.sql("INSERT INTO projects (id,owner_id,title,objective,status,created_at,updated_at) VALUES (?,?,?,?,'open',?,?)", id, this.owner, input.title, input.objective, at, at),
      this.sql("INSERT INTO activity (id,project_id,event,summary,created_at,details) VALUES (?,?,'project.created',?,?,?)", newId("event"), id, `Created ${input.title}`, at, JSON.stringify(input)),
    ]);
    return this.project(id);
  }
  async updateProject(args: Args) {
    const input = z.object({ project_id: idSchema, title: z.string().trim().min(1).max(180).optional(), objective: z.string().trim().max(1200).optional(), status: z.enum(["open", "archived"]).optional() }).parse(args);
    const current = await this.project(input.project_id), at = now();
    const after = { ...current, title: input.title ?? current.title, objective: input.objective ?? current.objective, status: input.status ?? current.status, updatedAt: at };
    await this.db.batch([
      this.sql("UPDATE projects SET title=?,objective=?,status=?,updated_at=? WHERE id=? AND owner_id=?", after.title, after.objective, after.status, at, current.id, this.owner),
      this.sql("INSERT INTO activity (id,project_id,event,summary,created_at,details) VALUES (?,?,'project.updated',?,?,?)", newId("event"), current.id, `Updated ${after.title}`, at, JSON.stringify({ before: current, after })),
    ]);
    return this.project(current.id);
  }
  async node(projectId: string, id: string) {
    await this.project(projectId);
    const row = await this.sql("SELECT * FROM nodes WHERE id=? AND project_id=?", idSchema.parse(id), projectId).first<Row>();
    if (!row) throw new WorkError("Work item not found.", 404);
    return nodeRow(row);
  }
  async createNode(args: Args) {
    const input = nodeInput.parse(args), project = await this.editableProject(input.project_id);
    if (input.parent_id) {
      const parent = await this.node(project.id, input.parent_id);
      if (!["goal", "key_result"].includes(parent.kind)) throw new WorkError("Use graph_query to connect work; parent_id must refer to a goal.");
    }
    const id = newId(input.kind), at = now();
    const node: WorkNode = { id, projectId: project.id, kind: input.kind, title: input.title, summary: input.summary, body: input.body, status: input.kind === "decision" ? "done" : input.status, blocker: input.blocker, stopReason: input.stop_reason, outcome: input.outcome, artifacts: input.artifacts, version: 1, createdAt: at, updatedAt: at };
    validateState(node);
    const queries = [
      this.sql("INSERT INTO nodes (id,project_id,kind,title,summary,body,status,blocker,stop_reason,outcome,artifacts,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)", id, project.id, node.kind, node.title, node.summary, node.body, node.status, node.blocker, node.stopReason, node.outcome, JSON.stringify(node.artifacts), at, at),
      this.sql("INSERT INTO activity (id,project_id,node_id,event,summary,created_at,details) VALUES (?,?,?,?,?,?,?)", newId("event"), project.id, id, `${node.kind}.created`, `Added ${node.title}`, at, JSON.stringify({ after: node })),
      this.sql("UPDATE projects SET updated_at=? WHERE id=?", at, project.id),
    ];
    if (node.kind === "decision") queries.push(this.sql(`INSERT INTO mcp_event_outbox
      (id,owner_id,project_id,node_id,name,version,status,transition,summary,occurred_at)
      SELECT a.id,p.owner_id,a.project_id,a.node_id,'decision.created',1,'done','created',?,a.created_at
      FROM activity a JOIN projects p ON p.id=a.project_id WHERE a.node_id=? AND a.event='decision.created'`, node.summary || node.title, id));
    if (input.parent_id) {
      const link: Link = { id: newId("link"), projectId: project.id, sourceId: id, targetId: input.parent_id, relation: "serves", rationale: "", createdAt: at };
      queries.push(this.sql("INSERT INTO links (id,project_id,source_id,target_id,relation,created_at) VALUES (?,?,?,?,?,?)", link.id, project.id, id, input.parent_id, link.relation, at));
      for (const nodeId of [id, input.parent_id]) queries.push(this.sql("INSERT INTO activity (id,project_id,node_id,event,summary,created_at,details) VALUES (?,?,?,'link.created',?,?,?)", newId("event"), project.id, nodeId, "Connected work to its goal", at, JSON.stringify({ link })));
    }
    await this.db.batch(queries);
    return this.node(project.id, id);
  }
  async updateNode(args: Args) {
    const input = nodeUpdate.parse(args), project = await this.editableProject(input.project_id);
    const current = await this.node(project.id, input.node_id);
    if (current.version !== input.expected_version) throw new WorkError("This item changed. Read it again and reconcile your update.", 409);
    const status = current.kind === "decision" ? "done" : input.status ?? current.status;
    const next: WorkNode = { ...current, title: input.title ?? current.title, summary: input.summary ?? current.summary, body: input.body ?? current.body, status, blocker: status === "blocked" ? input.blocker ?? current.blocker : "", stopReason: status === "stopped" ? input.stop_reason ?? current.stopReason : "", outcome: input.outcome ?? current.outcome, artifacts: input.artifacts ?? current.artifacts };
    validateState(next);
    const fields = ["title", "summary", "body", "status", "blocker", "stopReason", "outcome", "artifacts"] as const;
    const changed = fields.filter(field => JSON.stringify(next[field]) !== JSON.stringify(current[field]));
    if (!changed.length) return current;
    next.version = current.version + 1; next.updatedAt = now();
    const eventId = newId("event"), at = next.updatedAt;
    const transition = ["done", "stopped"].includes(current.status) && ["planned", "working", "blocked"].includes(next.status) ? "reopened" : next.status;
    const event = current.kind !== "decision" && changed.includes("status") && ["blocked", "done", "stopped", "reopened"].includes(transition)
      ? [this.sql(`INSERT INTO mcp_event_outbox
        (id,owner_id,project_id,node_id,name,version,status,previous_status,transition,summary,occurred_at)
        SELECT a.id,p.owner_id,a.project_id,a.node_id,'work.status_changed',?,?,?,?,?,a.created_at
        FROM activity a JOIN projects p ON p.id=a.project_id WHERE a.id=?`,
        next.version, next.status, current.status, transition, next.summary || next.title, eventId)] : [];
    const results = await this.db.batch([
      this.sql("UPDATE nodes SET title=?,summary=?,body=?,status=?,blocker=?,stop_reason=?,outcome=?,artifacts=?,version=?,mutation_id=?,updated_at=? WHERE id=? AND project_id=? AND version=?", next.title, next.summary, next.body, next.status, next.blocker, next.stopReason, next.outcome, JSON.stringify(next.artifacts), next.version, eventId, at, current.id, project.id, input.expected_version),
      // The marker prevents a losing concurrent update from emitting false history.
      this.sql("INSERT INTO activity (id,project_id,node_id,event,summary,created_at,details) SELECT ?,project_id,id,?,?,?,? FROM nodes WHERE id=? AND mutation_id=?", eventId, `${current.kind}.updated`, `${next.title}: ${changed.includes("status") ? `${current.status} → ${next.status}` : `updated ${changed.join(", ")}`}`, at, JSON.stringify({ before: current, after: next }), current.id, eventId),
      this.sql("UPDATE projects SET updated_at=? WHERE id=? AND EXISTS(SELECT 1 FROM nodes WHERE id=? AND mutation_id=?)", at, project.id, current.id, eventId),
      ...event,
    ]);
    if (!results[0].meta.changes) throw new WorkError("This item changed. Read it again and reconcile your update.", 409);
    return this.node(project.id, current.id);
  }
  async link(args: Args) {
    const input = linkInput.parse(args);
    await this.editableProject(input.project_id);
    if (input.source_id === input.target_id) throw new WorkError("An item cannot link to itself.");
    await Promise.all([this.node(input.project_id, input.source_id), this.node(input.project_id, input.target_id)]);
    const link: Link = { id: newId("link"), projectId: input.project_id, sourceId: input.source_id, targetId: input.target_id, relation: input.relation, rationale: input.rationale, createdAt: now() };
    const queries = [this.sql("INSERT INTO links (id,project_id,source_id,target_id,relation,rationale,created_at) VALUES (?,?,?,?,?,?,?) ON CONFLICT(project_id,source_id,target_id,relation) DO NOTHING", link.id, link.projectId, link.sourceId, link.targetId, link.relation, link.rationale, link.createdAt)];
    for (const nodeId of [link.sourceId, link.targetId]) queries.push(this.sql("INSERT INTO activity (id,project_id,node_id,event,summary,created_at,details) SELECT ?,project_id,?,'link.created',?,?,? FROM links WHERE id=?", newId("event"), nodeId, `Connected ${link.relation.replaceAll("_", " ")}`, link.createdAt, JSON.stringify({ link }), link.id));
    queries.push(this.sql("UPDATE projects SET updated_at=? WHERE id=? AND EXISTS(SELECT 1 FROM links WHERE id=?)", link.createdAt, link.projectId, link.id));
    await this.db.batch(queries);
    const row = await this.sql("SELECT * FROM links WHERE project_id=? AND source_id=? AND target_id=? AND relation=?", link.projectId, link.sourceId, link.targetId, link.relation).first<Row>();
    if (!row) throw new WorkError("This relationship changed. Read the graph again.", 409);
    const saved = linkRow(row);
    if (saved.rationale !== link.rationale) throw new WorkError("This link already has a different rationale. Read it, then unlink and replace it if needed.", 409);
    return saved;
  }
  async unlink(args: Args) {
    const input = z.object({ project_id: idSchema, link_id: idSchema }).parse(args);
    await this.editableProject(input.project_id);
    const row = await this.sql("SELECT * FROM links WHERE id=? AND project_id=?", input.link_id, input.project_id).first<Row>();
    if (!row) return { removed: false };
    const link = linkRow(row), at = now(), eventIds = [newId("event"), newId("event")];
    const results = await this.db.batch([
      ...[link.sourceId, link.targetId].map((nodeId, i) => this.sql("INSERT INTO activity (id,project_id,node_id,event,summary,created_at,details) SELECT ?,project_id,?,'link.removed',?,?,? FROM links WHERE id=? AND project_id=?", eventIds[i], nodeId, `Removed ${link.relation.replaceAll("_", " ")}`, at, JSON.stringify({ link }), link.id, link.projectId)),
      this.sql("DELETE FROM links WHERE id=? AND project_id=?", link.id, link.projectId),
      this.sql("UPDATE projects SET updated_at=? WHERE id=? AND EXISTS(SELECT 1 FROM activity WHERE id=?)", at, link.projectId, eventIds[0]),
    ]);
    return { removed: !!results[2].meta.changes };
  }
  async history(projectId: string, nodeId: string, cursor?: string): Promise<HistoryPage> {
    await this.node(projectId, nodeId);
    const before = cursor === undefined ? Number.MAX_SAFE_INTEGER : z.coerce.number().int().positive().max(Number.MAX_SAFE_INTEGER).parse(cursor);
    // SQLite rowid supplies stable insertion order even for same-millisecond events.
    const result = await this.sql("SELECT rowid AS sequence,* FROM activity WHERE project_id=? AND node_id=? AND rowid<? ORDER BY rowid DESC LIMIT 21", projectId, nodeId, before).all<Row>();
    const page = result.results.slice(0, 20);
    return { entries: page.map(r => ({ ...activityRow(r), details: JSON.parse(str(r.details) || "{}") })), next_cursor: result.results.length > 20 ? str(page.at(-1)!.sequence) : null };
  }
  async snapshot(projectId: string): Promise<Snapshot> {
    const project = await this.project(projectId);
    const [nodeResult, linkResult, activityResult, count] = await this.db.batch<Row>([
      this.sql("SELECT * FROM nodes WHERE project_id=? ORDER BY CASE WHEN status IN ('working','blocked') THEN 0 ELSE 1 END,updated_at DESC,id LIMIT 1000", projectId),
      this.sql("SELECT * FROM links WHERE project_id=?", projectId),
      this.sql("SELECT id,project_id,node_id,agent_id,event,summary,created_at FROM activity WHERE project_id=? ORDER BY rowid DESC LIMIT 100", projectId),
      this.sql("SELECT count(*) AS total FROM nodes WHERE project_id=?", projectId),
    ]);
    const nodes = nodeResult.results.map(nodeRow), ids = new Set(nodes.map(n => n.id)), totalNodes = Number(count.results[0].total);
    return { project, nodes, links: linkResult.results.map(linkRow).filter(l => ids.has(l.sourceId) && ids.has(l.targetId)), activity: activityResult.results.map(activityRow), totalNodes, hasMore: totalNodes > nodes.length };
  }
  async dispatch(tool: string, args: Args) {
    const action = str(args.action), projectId = args.project_id ? idSchema.parse(args.project_id) : "";
    if (tool === "project") {
      if (action === "list") return { projects: await this.projects() };
      if (action === "create") return this.createProject(args);
      if (action === "get") return this.project(projectId);
      if (action === "update") return this.updateProject(args);
    }
    if (tool === "get_context") return brief(await this.snapshot(projectId), typeof args.since === "string" ? args.since : undefined);
    if (tool === "graph_query") {
      if (action === "link") return this.link(args);
      if (action === "unlink") return this.unlink(args);
      if (!action || action === "query") {
        if (args.node_id) return { node: await this.node(projectId, str(args.node_id)) };
        const snapshot = await this.snapshot(projectId), query = str(args.query).toLowerCase();
        const matching = snapshot.nodes.filter(n => !query || `${n.title} ${n.summary}`.toLowerCase().includes(query));
        const ids = new Set(matching.map(n => n.id));
        return { nodes: matching.map(compactNode), links: snapshot.links.filter(l => ids.has(l.sourceId) || ids.has(l.targetId)), totalNodes: snapshot.totalNodes, hasMore: snapshot.hasMore };
      }
    }
    if (tool === "work" || tool === "decision") {
      if (action === (tool === "decision" ? "log" : "create")) {
        if (tool === "work" && args.kind !== undefined && !["task", "goal"].includes(str(args.kind))) throw new WorkError("Work creates tasks or optional goals. Use decision to record a choice.");
        return this.createNode({ ...args, kind: tool === "decision" ? "decision" : args.kind ?? "task" });
      }
      if (["get", "update", "history"].includes(action)) {
        const current = await this.node(projectId, str(args.node_id));
        if ((tool === "decision") !== (current.kind === "decision")) throw new WorkError(`Use the ${current.kind === "decision" ? "decision" : "work"} tool for this record.`);
        if (action === "get") return current;
        if (action === "history") return this.history(projectId, current.id, typeof args.cursor === "string" ? args.cursor : undefined);
        return this.updateNode(args);
      }
      if (action === "list") {
        const snapshot = await this.snapshot(projectId);
        return { nodes: snapshot.nodes.filter(n => (tool === "decision" ? n.kind === "decision" : n.kind !== "decision") && (!args.status || n.status === args.status) && (!args.kind || n.kind === args.kind)).map(compactNode), hasMore: snapshot.hasMore };
      }
    }
    throw new WorkError("Unknown coordination action.");
  }
}
