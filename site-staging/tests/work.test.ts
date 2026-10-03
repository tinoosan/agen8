import test from "node:test";
import assert from "node:assert/strict";
import { fixture } from "./sqlite";
import { Work } from "../lib/work";
import { brief, type WorkNode, type HistoryPage } from "../lib/model";
import { withMcpProtocol } from "../lib/mcp-protocol";
import { tools, instructions } from "../lib/mcp";

test("work completes directly, stops with a reason, and reopens with results preserved", async () => {
  const { db, sqlite } = fixture();
  try {
    const work = new Work(db, "owner"), project = await work.createProject({ title: "Shared work" });
    let node = await work.dispatch("work", { action: "create", project_id: project.id, title: "Port graph", body: "Keep colours" }) as WorkNode;
    const update = (fields: Record<string, unknown>) => work.dispatch("work", { action: "update", project_id: project.id, node_id: node.id, expected_version: node.version, ...fields }) as Promise<WorkNode>;
    assert.equal(node.status, "planned");
    node = await update({ status: "working", summary: "Adapting the original graph." });
    await assert.rejects(update({ status: "blocked" }), /blocking/i);
    node = await update({ status: "blocked", blocker: "Missing hosted snapshot" });
    node = await update({ status: "done", outcome: "Ported; typecheck and interaction checks passed.", artifacts: ["https://example.com/checks"] });
    assert.equal(node.blocker, ""); assert.equal(node.status, "done");
    await assert.rejects(update({ status: "stopped" }), /reason/i);
    node = await update({ status: "stopped", stop_reason: "Reconsidering focus behaviour." });
    node = await update({ status: "working", outcome: "" });
    assert.equal(node.stopReason, "");
    const history = await work.dispatch("work", { action: "history", project_id: project.id, node_id: node.id }) as HistoryPage;
    assert(history.entries.some(e => e.details.after?.status === "done" && e.details.after.outcome.includes("Ported")));
    assert(history.entries.some(e => e.details.after?.stopReason.includes("Reconsidering")));
    assert.equal((await brief(await work.snapshot(project.id))).working.length, 1);
    assert.equal(sqlite.prepare("SELECT count(*) AS n FROM agents").get()!.n, 0);
    assert(!("agentId" in node)); assert(!("agents" in await work.snapshot(project.id)));
  } finally { sqlite.close(); }
});

test("concurrent stale writes, no-ops, and rejected writes emit no false history", async () => {
  const { db, sqlite } = fixture();
  try {
    const work = new Work(db, "owner"), p = await work.createProject({ title: "Conflict" });
    const n = await work.createNode({ project_id: p.id, title: "Shared node" });
    const results = await Promise.allSettled(["First", "Second"].map(summary => work.updateNode({ project_id: p.id, node_id: n.id, expected_version: 1, summary })));
    assert.equal(results.filter(r => r.status === "fulfilled").length, 1);
    let history = await work.history(p.id, n.id); assert.equal(history.entries.length, 2);
    const current = await work.node(p.id, n.id);
    await assert.rejects(work.updateNode({ project_id: p.id, node_id: n.id, expected_version: 1, status: "done" }), /reconcile/);
    assert.equal((await work.updateNode({ project_id: p.id, node_id: n.id, expected_version: current.version, summary: current.summary })).version, current.version);
    history = await work.history(p.id, n.id); assert.equal(history.entries.length, 2);
    sqlite.exec("CREATE TRIGGER reject_history BEFORE INSERT ON activity WHEN NEW.event='task.updated' BEGIN SELECT RAISE(ABORT,'history failed'); END");
    await assert.rejects(work.updateNode({ project_id: p.id, node_id: n.id, expected_version: current.version, status: "done" }), /history failed/);
    assert.deepEqual(await work.node(p.id, n.id), current);
    assert.equal((await work.history(p.id, n.id)).entries.length, 2);
  } finally { sqlite.close(); }
});

test("account isolation applies to reads, updates, history and relationships", async () => {
  const { db, sqlite } = fixture();
  try {
    const a = new Work(db, "a"), b = new Work(db, "b");
    const p = await a.createProject({ title: "A" }), q = await a.createProject({ title: "B" });
    const left = await a.createNode({ project_id: p.id, title: "Left" }), right = await a.createNode({ project_id: q.id, title: "Right" });
    await assert.rejects(b.snapshot(p.id), /Project not found/);
    await assert.rejects(b.node(p.id, left.id), /Project not found/);
    await assert.rejects(b.history(p.id, left.id), /Project not found/);
    await assert.rejects(b.updateNode({ project_id: p.id, node_id: left.id, expected_version: 1, title: "Wrong account" }), /Project not found/);
    await assert.rejects(b.link({ project_id: p.id, source_id: left.id, target_id: right.id, relation: "blocked_by" }), /Project not found/);
    assert.equal((await b.projects()).length, 0);
    await assert.rejects(a.link({ project_id: p.id, source_id: left.id, target_id: right.id, relation: "blocked_by" }), /Work item not found/);
    assert.throws(() => new Work(db, ""), /Sign in/);
  } finally { sqlite.close(); }
});

test("relationships and their endpoint histories are atomic and idempotent", async () => {
  const { db, sqlite } = fixture();
  try {
    const work = new Work(db, "owner"), p = await work.createProject({ title: "Relationships" });
    const goal = await work.dispatch("work", { action: "create", project_id: p.id, kind: "goal", title: "Shared understanding" }) as WorkNode;
    const task = await work.createNode({ project_id: p.id, parent_id: goal.id, title: "Port graph" });
    const decision = await work.dispatch("decision", { action: "log", project_id: p.id, title: "Keep graph", body: "Relationships give context" }) as WorkNode;
    const args = { project_id: p.id, source_id: task.id, target_id: decision.id, relation: "informed_by", rationale: "Preserve the interaction" };
    const link = await work.link(args);
    assert.equal((await work.link(args)).id, link.id);
    assert.equal((await work.history(p.id, decision.id)).entries.length, 2);
    await assert.rejects(work.link({ ...args, rationale: "Different" }), /different rationale/);
    assert.equal((await work.history(p.id, decision.id)).entries.length, 2);
    await assert.rejects(work.dispatch("work", { action: "update", project_id: p.id, node_id: decision.id, expected_version: 1, body: "Wrong tool" }), /decision tool/);
    const revised = await work.dispatch("decision", { action: "update", project_id: p.id, node_id: decision.id, expected_version: 1, body: "Compact shapes give context" }) as WorkNode;
    assert.equal(revised.version, 2);
    assert.equal((await work.history(p.id, decision.id)).entries[0].details.before?.body, decision.body);
    assert.deepEqual(await work.unlink({ project_id: p.id, link_id: link.id }), { removed: true });
    assert.deepEqual(await work.unlink({ project_id: p.id, link_id: link.id }), { removed: false });
    assert.equal((await work.history(p.id, decision.id)).entries.filter(e => e.event === "link.removed").length, 1);
    sqlite.exec("CREATE TRIGGER reject_link_history BEFORE INSERT ON activity WHEN NEW.event='link.created' BEGIN SELECT RAISE(ABORT,'link history failed'); END");
    await assert.rejects(work.link(args), /link history failed/);
    assert.equal((await work.snapshot(p.id)).links.length, 1);
  } finally { sqlite.close(); }
});

test("history pagination preserves ordering without duplicates at equal timestamps", async () => {
  const { db, sqlite } = fixture();
  try {
    const work = new Work(db, "owner"), p = await work.createProject({ title: "History" });
    let n = await work.createNode({ project_id: p.id, title: "Changes" });
    for (let i = 0; i < 25; i++) n = await work.updateNode({ project_id: p.id, node_id: n.id, expected_version: n.version, summary: `Meaningful revision ${i}` });
    sqlite.prepare("UPDATE activity SET created_at='2026-10-01T00:00:00Z'").run();
    const page1 = await work.history(p.id, n.id), page2 = await work.history(p.id, n.id, page1.next_cursor!);
    assert.equal(page1.entries.length, 20); assert.equal(page2.entries.length, 6); assert.equal(page2.next_cursor, null);
    const ids = [...page1.entries, ...page2.entries].map(e => e.id); assert.equal(new Set(ids).size, 26);
    assert.equal(page1.entries[0].details.after?.version, 26);
  } finally { sqlite.close(); }
});

test("migration retains legacy text, evidence, links, timestamps, agents and original states", async () => {
  const oldStates = ["pending", "active", "in_review", "succeeded", "failed", "canceled"], mapped = ["planned", "working", "working", "done", "stopped", "stopped"];
  const at = "2026-09-29T12:00:00Z";
  const { db, sqlite } = fixture((sql, file) => {
    if (!file.startsWith("0001")) return;
    sql.prepare("INSERT INTO projects(id,owner_id,title,created_at,updated_at) VALUES ('original','owner','Original',?,?)").run(at, at);
    sql.prepare("INSERT INTO agents(id,project_id,session_ref,name,last_seen_at) VALUES ('old-agent','original','native','Earlier identity',?)").run(at);
    for (const [i, state] of oldStates.entries()) sql.prepare("INSERT INTO nodes(id,project_id,kind,title,body,status,agent_id,outcome,artifacts,created_at,updated_at) VALUES (?,'original',?,'Original title','Original text',?,'old-agent','Existing result','[\"artifact:original\"]',?,?)").run(state, i === 0 ? "key_result" : i === 1 ? "note" : "task", state, at, at);
    sql.prepare("INSERT INTO links(id,project_id,source_id,target_id,relation,rationale,created_at) VALUES ('old-link','original','pending','active','serves','Original rationale',?)").run(at);
  });
  try {
    const work = new Work(db, "owner"), snapshot = await work.snapshot("original");
    for (const [i, id] of oldStates.entries()) {
      const n = await work.node("original", id); assert.equal(n.status, mapped[i]); assert.equal(n.body, "Original text"); assert.equal(n.outcome, "Existing result"); assert.deepEqual(n.artifacts, ["artifact:original"]); assert.equal(n.createdAt, at); assert.equal(n.updatedAt, at); assert.equal(n.version, 2);
      const event = (await work.history("original", id)).entries[0]; assert.equal(event.details.originalStatus, id); assert.equal(event.details.before?.status, id); assert.equal(event.details.after?.status, mapped[i]);
      if (n.status === "stopped") assert(n.stopReason);
    }
    assert.equal(snapshot.links[0].id, "old-link"); assert.equal(snapshot.links[0].rationale, "Original rationale");
    assert.equal(sqlite.prepare("SELECT count(*) AS n FROM agents").get()!.n, 1);
    assert.equal(snapshot.nodes.find(n => n.id === "active")!.kind, "note");
    await assert.rejects(work.dispatch("work", { action: "create", project_id: "original", kind: "note", title: "Unsupported" }), /tasks or optional goals/);
  } finally { sqlite.close(); }
});

test("exposed tools and instructions are the five shared graph tools", () => {
  assert.deepEqual(tools.map(t => t.name), ["project", "work", "decision", "graph_query", "get_context"]);
  assert(instructions.includes("expected_version")); assert(instructions.includes("reuse existing work"));
  for (const t of tools) { assert(!("agent_id" in t.inputSchema.properties)); assert(!("actor_agent_id" in t.inputSchema.properties)); }
});
test("modern MCP validates request mirrors and returns modern response envelopes", async () => {
  const meta = { "io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": {} };
  const body = { jsonrpc: "2.0", id: 1, method: "server/discover", params: { _meta: meta } };
  const valid = new Request("https://agen8.example/mcp", { method: "POST", headers: { "mcp-protocol-version": "2026-07-28", "mcp-method": "server/discover" }, body: JSON.stringify(body) });
  const response = await withMcpProtocol(valid, async () => Response.json({ jsonrpc: "2.0", id: 1, result: { capabilities: { tools: {} } } }));
  const payload = await response.json() as { result: { resultType: string; _meta: Record<string, { name: string }> } };
  assert.equal(payload.result.resultType, "complete"); assert.equal(payload.result._meta["io.modelcontextprotocol/serverInfo"].name, "agen8-dev");
  const invalid = new Request("https://agen8.example/mcp", { method: "POST", headers: { "mcp-protocol-version": "2026-07-28", "mcp-method": "tools/call" }, body: JSON.stringify(body) });
  assert.equal((await withMcpProtocol(invalid, async () => { throw new Error("Must not dispatch"); })).status, 400);
});

test("Sites-dispatched MCP accepts omitted method/name mirrors and rejects contradictory metadata", async () => {
  const meta = { "io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": {} };
  const body = { jsonrpc: "2.0", id: 1, method: "tools/call", params: { name: "project", arguments: { action: "list" }, _meta: meta } };
  const headers = { "mcp-protocol-version": "2026-07-28", "x-dispatched-app": "site---test", "oai-authenticated-user-id": "owner" };
  const request = (extra: Record<string, string> = {}, payload: unknown = body) => new Request("https://agen8.example/mcp", { method: "POST", headers: { ...headers, ...extra }, body: JSON.stringify(payload) });
  let dispatched = false;
  const response = await withMcpProtocol(request(), async () => { dispatched = true; return Response.json({ jsonrpc: "2.0", id: 1, result: { content: [] } }); });
  assert.equal(response.status, 200); assert.equal(dispatched, true);
  const reject = async () => { throw new Error("Invalid request must not dispatch"); };
  assert.equal((await withMcpProtocol(request({ "mcp-method": "tools/list" }), reject)).status, 400);
  assert.equal((await withMcpProtocol(request({ "mcp-name": "task" }), reject)).status, 400);
  assert.equal((await withMcpProtocol(request({}, { ...body, params: { name: "project" } }), reject)).status, 400);
  const direct = new Request("https://agen8.example/mcp", { method: "POST", headers: { "mcp-protocol-version": "2026-07-28" }, body: JSON.stringify(body) });
  assert.equal((await withMcpProtocol(direct, reject)).status, 400);
});
