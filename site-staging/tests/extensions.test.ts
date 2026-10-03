import test from "node:test";
import assert from "node:assert/strict";
import { CallToolResultSchema, ListToolsResultSchema, ReadResourceResultSchema } from "@modelcontextprotocol/sdk/types.js";
import { fixture } from "./sqlite";
import { Work } from "../lib/work";
import { tools, mcpResponse } from "../lib/mcp";
import { extensionTools, extensionCall, readExtensionResource, appResource } from "../lib/extensions";
import { graphPath, graphUrl, parseGraphPath, recordUri, parseRecordUri } from "../lib/navigation";
import { SelectionContext, selectedContext, type ContextPayload } from "../mcp-app/context";
import type { WorkNode } from "../lib/model";
import { App } from "@modelcontextprotocol/ext-apps";
import { AppBridge } from "@modelcontextprotocol/ext-apps/app-bridge";
import { OpenAIExtensions } from "@openai/mcp-extensions/app";
import { InMemoryTransport } from "@modelcontextprotocol/sdk/inMemory.js";

test("extension metadata preserves five agent tools and exposes read-only graph entrypoints", async () => {
  assert.equal(tools.length, 5);
  assert(extensionTools.every(t => t._meta.ui.visibility.includes("app")));
  const graph = extensionTools.find(t => t.name === "open_graph")!;
  assert.equal(graph.title, "Work graph");
  assert.equal(graph.annotations.readOnlyHint, true);
  assert("openai/ui" in graph._meta);
  assert.deepEqual(graph._meta["openai/ui"]?.entrypoints, [{ type: "global" }, { type: "thread" }]);
  assert.equal(appResource._meta.ui.csp.connectDomains.length, 0);
  const request = (method: string) => new Request("https://example.com/mcp", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params: {} }) });
  const list = await (await mcpResponse(request("tools/list"))).json() as { result: unknown };
  assert.equal(ListToolsResultSchema.parse(list.result).tools.length, 9);
  const discovery = await (await mcpResponse(request("initialize"))).json() as { result: { capabilities: { extensions: Record<string, { readTool: string; updateTool: string }> } } };
  assert.equal(discovery.result.capabilities.extensions["openai/settings"].readTool, "settings_read");
  assert.equal(discovery.result.capabilities.extensions["openai/settings"].updateTool, "settings_update");
});

test("graph bootstrap, mention search and resource reads are bounded, private and read-only", async () => {
  const { db, sqlite } = fixture();
  try {
    const mine = new Work(db, "mine"), other = new Work(db, "other");
    const p = await mine.createProject({ title: "Current project" }), privateProject = await other.createProject({ title: "Secret project" });
    const node = await mine.createNode({ project_id: p.id, title: "Render work %_", summary: "Visible meaningful work", status: "working" });
    await other.createNode({ project_id: privateProject.id, title: "Secret work" });
    const before = sqlite.prepare("SELECT count(*) AS n FROM activity").get()!.n;
    const bootstrap = await extensionCall(mine, "open_graph", {}) as { projects: { id: string }[]; snapshot: { nodes: WorkNode[] } };
    assert.deepEqual(bootstrap.projects.map(p => p.id), [p.id]);
    assert.equal(bootstrap.snapshot.nodes[0].id, node.id);
    assert.equal((await extensionCall(mine, "search_mentions", { query: "Secret" }) as { items: unknown[] }).items.length, 0);
    const search = await extensionCall(mine, "search_mentions", { query: "%_" }) as { items: { uri: string }[] };
    assert.equal(search.items.length, 1); assert.equal(search.items[0].uri, recordUri(p.id, node.id));
    assert(CallToolResultSchema.safeParse({ content: [], structuredContent: search }).success);
    const resource = await readExtensionResource(mine, search.items[0].uri);
    assert(ReadResourceResultSchema.safeParse(resource).success);
    assert.equal(JSON.parse(resource.contents[0].text).id, node.id);
    await assert.rejects(readExtensionResource(other, search.items[0].uri), /not found/);
    await assert.rejects(extensionCall(mine, "open_graph", { project_id: privateProject.id }), /not found/);
    await assert.rejects(extensionCall(mine, "open_graph", { project_id: p.id, node_id: "missing" }), /not found/);
    const ui = await readExtensionResource(mine, appResource.uri);
    assert(ReadResourceResultSchema.safeParse(ui).success);
    assert(ui.contents[0].text.includes("ui/initialize"));
    assert(!ui.contents[0].text.includes("Secret project"));
    assert.equal(sqlite.prepare("SELECT count(*) AS n FROM activity").get()!.n, before);
    for (let i = 0; i < 25; i++) await mine.createNode({ project_id: p.id, title: `Many nodes ${i}` });
    assert.equal((await mine.mentions("Many nodes")).length, 20);
    await assert.rejects(extensionCall(mine, "search_mentions", { query: "a".repeat(201) }));
  } finally { sqlite.close(); }
});

test("context preference persists for one account without mutating work or history", async () => {
  const { db, sqlite } = fixture();
  try {
    const mine = new Work(db, "mine"), other = new Work(db, "other");
    assert.equal((await mine.preferences()).shareSelectedContext, true);
    assert.equal(sqlite.prepare("SELECT count(*) AS n FROM preferences").get()!.n, 0);
    const setting = await extensionCall(mine, "settings_update", { set: { shareSelectedContext: false } }) as { values: { shareSelectedContext: boolean } };
    assert.equal(setting.values.shareSelectedContext, false);
    assert.equal((await new Work(db, "mine").preferences()).shareSelectedContext, false);
    assert.equal((await other.preferences()).shareSelectedContext, true);
    await assert.rejects(extensionCall(mine, "settings_update", { set: {} }));
    await assert.rejects(extensionCall(mine, "settings_update", { set: { shareSelectedContext: "false" } }));
    await assert.rejects(extensionCall(mine, "settings_update", { set: { shareSelectedContext: true, owner: "other" } }));
    assert.equal((await mine.preferences()).shareSelectedContext, false);
    assert.equal(sqlite.prepare("SELECT count(*) AS n FROM activity").get()!.n, 0);
  } finally { sqlite.close(); }
});

test("deep links round-trip IDs while rejecting external, fragmented and ambiguous paths", () => {
  const path = graphPath("project a", "task/?b");
  assert.deepEqual(parseGraphPath(path), { projectId: "project a", nodeId: "task/?b" });
  assert.equal(new URL(graphUrl("project a", "task/?b")).searchParams.get("path"), path);
  assert.deepEqual(parseRecordUri(recordUri("project a", "task/?b")), { projectId: "project a", nodeId: "task/?b" });
  for (const path of ["https://evil.test", "//evil.test/", "/?project=x#node", "/?project=x&project=y", "/?node=x", "/anything", "/?unknown=x", "/?project=", "/\\evil.test"]) assert.equal(parseGraphPath(path), null, path);
  for (const uri of ["https://evil.test", "agen8://projects/x/extra", "agen8://projects/%ZZ"]) assert.equal(parseRecordUri(uri), null);
});

test("selected context is concise, follows changes and honours dismissal and disabled sharing", async () => {
  const { db, sqlite } = fixture();
  try {
    const work = new Work(db, "owner"), p = await work.createProject({ title: "Context" });
    const node = await work.createNode({ project_id: p.id, title: "Read graph", summary: "Short explanation", body: "Do not attach this long explanation." });
    const snapshot = await work.snapshot(p.id), sent: ContextPayload[] = [];
    const context = new SelectionContext(async value => { sent.push(value); }, error => { throw error; });
    context.select(snapshot, node, true); await context.settled();
    assert.equal(sent.length, 1); assert(!sent[0].content[0].text.includes(node.body));
    context.select({ ...snapshot }, { ...node }, true); await context.settled(); assert.equal(sent.length, 1);
    context.dismiss(); context.select(snapshot, { ...node, version: 2 }, true); await context.settled();
    assert.equal(sent.at(-1)!.content.length, 0);
    context.select(snapshot, { ...node, version: 3 }, true); await context.settled(); assert.equal(sent.length, 2);
    context.select(snapshot, { ...node, id: "another" }, true); await context.settled(); assert.equal(sent.at(-1)!.structuredContent.node_id, "another");
    context.select(snapshot, node, true); await context.settled(); assert.equal(sent.at(-1)!.structuredContent.node_id, node.id);
    context.select(snapshot, node, false); await context.settled(); assert.equal(sent.at(-1)!.content.length, 0);
    const payload = selectedContext(snapshot, { ...node, status: "blocked", blocker: "A dependency is missing", outcome: "x".repeat(1200) });
    assert(payload.content[0].text.includes("Blocked: A dependency is missing")); assert(payload.content[0].text.length < 1000);
  } finally { sqlite.close(); }
});

test("official app bridge initializes, forwards reads and delivers deep links and context dismissal", async () => {
  const { db, sqlite } = fixture();
  const app = new App({ name: "Agen8", version: "0.3.0" }, {}, { autoResize: false });
  const extensions = new OpenAIExtensions(app);
  const bridge = new AppBridge(null, { name: "Test host", version: "1" }, { serverTools: {}, experimental: { "openai/modelContext": {} }, updateModelContext: { text: {}, structuredContent: {} } }, { hostContext: { "openai/deepLink": { url: "/" } } });
  try {
    const work = new Work(db, "owner"), project = await work.createProject({ title: "Native app" });
    const node = await work.createNode({ project_id: project.id, title: "Read this work" });
    bridge.oncalltool = async params => ({ content: [], structuredContent: await extensionCall(work, params.name, params.arguments ?? {}) });
    let attached: unknown;
    bridge.onupdatemodelcontext = async params => { attached = params; return { _meta: { "openai/modelContext": { updateId: "update-1" } } }; };
    let received: unknown;
    const delivered = new Promise<void>(resolve => { app.ontoolresult = value => { received = value.structuredContent; resolve(); }; });
    const [host, view] = InMemoryTransport.createLinkedPair();
    await bridge.connect(host); await app.connect(view);
    assert.equal(extensions.deepLink.getCurrent()?.url, "/");
    await bridge.sendToolInput({ arguments: {} });
    const bootstrap = await extensionCall(work, "open_graph", {});
    await bridge.sendToolResult({ content: [], structuredContent: bootstrap });
    await delivered; assert.deepEqual(received, bootstrap);
    const read = await app.callServerTool({ name: "open_graph", arguments: { project_id: project.id, node_id: node.id } });
    assert.equal((read.structuredContent?.selection as { nodeId: string }).nodeId, node.id);
    const update = await extensions.modelContext!.update(selectedContext(await work.snapshot(project.id), node));
    assert.equal(update?.updateId, "update-1"); assert(attached);
    const changed = new Promise<void>(resolve => { app.onhostcontextchanged = () => resolve(); });
    bridge.setHostContext({ "openai/deepLink": { url: graphPath(project.id, node.id) }, "openai/modelContext": null });
    await changed;
    assert.equal(extensions.modelContext!.getCurrent(), null);
    assert.equal(extensions.deepLink.getCurrent()?.url, graphPath(project.id, node.id));
  } finally { await app.close(); await bridge.close(); sqlite.close(); }
});
