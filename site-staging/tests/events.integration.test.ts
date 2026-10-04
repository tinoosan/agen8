import test from "node:test";
import assert from "node:assert/strict";
import { fixture } from "./d1";
import { Work } from "../lib/work";
import { createDispatchGrant, dispatchGrant, grantOwnerAccess } from "../lib/event-grants";
import { configuredEvents, eventAccessResponse } from "../lib/event-runtime";
import { pruneEvents } from "../lib/event-retention";

const key = Buffer.alloc(32, 7).toString("base64");
const accessRequest = (owner = "site-principal", body: unknown = { action: "grant" }, token = key) => new Request("https://agen8.test/api/events/access", {
  method: "POST", headers: { ...(owner ? { "oai-authenticated-user-id": owner } : {}), "x-agen8-dispatch-key": token }, body: JSON.stringify(body),
});

test("D1 migration preserves cursors; bounded retention protects queued, unseen and other-owner events", async () => {
  const at = Date.now(), old = new Date(at - 31 * 86_400_000).toISOString();
  const runtime = await fixture(async (db, file) => {
    if (!file.startsWith("0005")) return;
    for (const [sequence, id, owner] of [[5, "event_old", "site-principal"], [9, "event_private", "another-principal"]] as const) {
      await db.prepare("INSERT INTO mcp_event_outbox (rowid,id,owner_id,project_id,node_id,name,version,status,transition,summary,occurred_at) VALUES (?,?,?,'project_synthetic','node_synthetic','decision.created',1,'done','created','Synthetic',?)").bind(sequence, id, owner, old).run();
    }
  });
  try {
    assert.deepEqual((await runtime.db.prepare("SELECT sequence FROM mcp_event_outbox ORDER BY sequence").all()).results, [{ sequence: 5 }, { sequence: 9 }]);
    const insert = (id: string) => runtime.db.prepare("INSERT INTO mcp_event_outbox (id,owner_id,project_id,node_id,name,version,status,transition,summary,occurred_at) VALUES (?,'site-principal','project_synthetic','node_synthetic','decision.created',1,'done','created','Synthetic',?)").bind(id, old).run();
    await insert("event_pending"); await insert("event_unseen");
    await createDispatchGrant(runtime.db, accessRequest(), 60_000, () => at);
    await runtime.db.prepare("INSERT INTO mcp_subscriptions (id,owner_id,name,arguments,project_id,url,secret,secret_hash,verified_at,expires_at,start_sequence,active) VALUES ('sub_synthetic','site-principal','decision.created','{}','project_synthetic','https://example.com','encrypted-synthetic','synthetic',0,?,9,1)").bind(at + 60_000).run();
    await runtime.db.prepare("INSERT INTO mcp_deliveries (subscription_id,event_id,state,next_attempt_at) VALUES ('sub_synthetic','event_pending','pending',0)").run();
    await pruneEvents(runtime.db, "site-principal", () => at);
    assert.deepEqual((await runtime.db.prepare("SELECT id FROM mcp_event_outbox ORDER BY sequence").all()).results, [{ id: "event_private" }, { id: "event_pending" }, { id: "event_unseen" }]);
    await pruneEvents(runtime.db, "site-principal", () => at + 60_001);
    assert.equal((await runtime.db.prepare("SELECT secret FROM mcp_subscriptions").first<{ secret: string }>())!.secret, "");
    assert.equal((await runtime.db.prepare("SELECT state FROM mcp_deliveries").first<{ state: string }>())!.state, "cancelled");
    assert.deepEqual((await runtime.db.prepare("SELECT id FROM mcp_event_outbox").all()).results, [{ id: "event_private" }]);
    await runtime.db.prepare("DELETE FROM mcp_event_outbox").run();
    const db = await runtime.restart();
    await db.prepare("INSERT INTO mcp_event_outbox (id,owner_id,project_id,node_id,name,version,status,transition,summary,occurred_at) VALUES ('event_new','site-principal','project_synthetic','node_synthetic','decision.created',1,'done','created','Synthetic',?)").bind(new Date(at).toISOString()).run();
    assert((await db.prepare("SELECT sequence FROM mcp_event_outbox").first<{ sequence: number }>())!.sequence > 11);
  } finally { await runtime.close(); }
});

test("production Worker routes stay disabled until configured and preserve tool discovery", async () => {
  const runtime = await fixture();
  try {
    const response = await runtime.fetch("/mcp", { method: "POST", headers: { "content-type": "application/json", "mcp-protocol-version": "2026-07-28", "mcp-method": "server/discover" }, body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "server/discover", params: { _meta: { "io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": {} } } }) });
    const body = await response.json() as { result: { capabilities: Record<string, unknown> } };
    assert.equal(response.status, 200); assert.equal(body.result.capabilities.events, undefined); assert(body.result.capabilities.tools);
    for (const path of ["/api/events/access", "/api/events/dispatch"]) assert.equal((await runtime.fetch(path, { method: "POST", body: "{}" })).status, 503);
    assert.equal((await runtime.db.prepare("SELECT count(*) AS n FROM mcp_dispatch_grants").first<{ n: number }>())!.n, 0);
    assert.equal(await configuredEvents({}, runtime.db), undefined);
    await assert.rejects(configuredEvents({ AGEN8_EVENTS_ENABLED: "true" }, runtime.db), /configuration/);
    const configuration = { AGEN8_EVENTS_ENABLED: "true", AGEN8_EVENT_RELAY_URL: "https://example.com", AGEN8_EVENT_RELAY_TOKEN: key, AGEN8_EVENT_ENCRYPTION_KEY: key, AGEN8_EVENT_DISPATCH_KEY: key, AGEN8_EVENT_GRANT_ID: "grant_unconfigured" };
    const integration = await configuredEvents(configuration, runtime.db);
    assert(integration); assert.equal(await integration.host.ownerHasAccess("site-principal"), false);
    await assert.rejects(configuredEvents({ ...configuration, AGEN8_EVENT_RELAY_URL: "http://example.com" }, runtime.db), /HTTPS/);
  } finally { await runtime.close(); }
});

test("committed event IDs, versions and owner scope survive real D1 restart; failed batches roll back", async () => {
  const runtime = await fixture();
  try {
    const work = new Work(runtime.db, "site-principal"), p = await work.createProject({ title: "Synthetic integration" });
    const node = await work.createNode({ project_id: p.id, title: "Synthetic task" });
    await work.updateNode({ project_id: p.id, node_id: node.id, expected_version: 1, status: "done" });
    await work.createNode({ project_id: p.id, kind: "decision", title: "Synthetic choice" });
    const before = await runtime.db.prepare("SELECT * FROM mcp_event_outbox ORDER BY rowid").all();
    assert.equal(before.results.length, 2);
    for (const row of before.results) assert(await runtime.db.prepare("SELECT id FROM activity WHERE id=?").bind(row.id).first());
    const restarted = await runtime.restart();
    assert.deepEqual((await restarted.prepare("SELECT * FROM mcp_event_outbox ORDER BY rowid").all()).results, before.results);
    const afterRestart = new Work(restarted, "site-principal");
    await restarted.prepare("CREATE TRIGGER reject_outbox BEFORE INSERT ON mcp_event_outbox BEGIN SELECT RAISE(ABORT,'outbox failed'); END").run();
    await assert.rejects(afterRestart.updateNode({ project_id: p.id, node_id: node.id, expected_version: 2, status: "working" }), /outbox failed/);
    assert.equal((await afterRestart.node(p.id, node.id)).version, 2);
    assert.equal((await restarted.prepare("SELECT count(*) AS n FROM mcp_event_outbox").first<{ n: number }>())!.n, 2);
    await assert.rejects(new Work(restarted, "another-principal").project(p.id), /not found/);
  } finally { await runtime.close(); }
});

test("operational access requires Site identity and separate key; grants persist, revoke atomically and never self-renew", async () => {
  const runtime = await fixture(); const env = { AGEN8_EVENT_DISPATCH_KEY: key };
  try {
    assert.equal((await eventAccessResponse(accessRequest(""), env, runtime.db)).status, 401);
    assert.equal((await eventAccessResponse(accessRequest("site-principal", {}, "wrong"), env, runtime.db)).status, 401);
    assert.equal((await eventAccessResponse(accessRequest("site-principal", { action: "grant", owner: "another" }), env, runtime.db)).status, 400);
    const response = await eventAccessResponse(accessRequest(), env, runtime.db);
    assert.equal(response.status, 200);
    const grant = await response.json() as { id: string; ownerId: string; expiresAt: number };
    assert.equal(grant.ownerId, "site-principal");
    let db = await runtime.restart();
    assert.equal(await grantOwnerAccess(db, grant.id)("site-principal"), true);
    assert.equal(await grantOwnerAccess(db, grant.id)("account-metadata-id"), false);
    await db.prepare("INSERT INTO mcp_subscriptions (id,owner_id,name,arguments,project_id,url,secret,secret_hash,verified_at,expires_at,start_sequence,active) VALUES ('sub_synthetic','site-principal','decision.created','{}','project_synthetic','https://example.com','encrypted-synthetic','synthetic',0,?,0,1)").bind(Date.now() + 60_000).run();
    await db.prepare("INSERT INTO mcp_deliveries (subscription_id,event_id,state,next_attempt_at) VALUES ('sub_synthetic','event_synthetic','pending',0)").run();
    assert.equal((await eventAccessResponse(accessRequest("site-principal", { action: "revoke", id: grant.id }), env, db)).status, 200);
    db = await runtime.restart();
    assert.equal(await dispatchGrant(db, grant.id), null);
    assert.equal((await db.prepare("SELECT state FROM mcp_deliveries").first<{ state: string }>())!.state, "cancelled");
    assert.equal((await db.prepare("SELECT secret FROM mcp_subscriptions").first<{ secret: string }>())!.secret, "");
    const replacement = await createDispatchGrant(db, accessRequest());
    assert.notEqual(replacement.id, grant.id);
    assert.equal((await db.prepare("SELECT active FROM mcp_subscriptions").first<{ active: number }>())!.active, 0);
  } finally { await runtime.close(); }
});
