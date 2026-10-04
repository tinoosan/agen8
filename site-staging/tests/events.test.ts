import test from "node:test";
import assert from "node:assert/strict";
import { createHmac } from "node:crypto";
import { fixture } from "./sqlite";
import { Work } from "../lib/work";
import { callbackUrl, dispatchEvents, EventError, eventDefinitions, McpEvents, subscriptionId, webhookHeaders, type EventHost } from "../lib/mcp-events";
import { mcpResponse } from "../lib/mcp";

const secret = `whsec_${Buffer.alloc(32, 7).toString("base64")}`;
const replacement = `whsec_${Buffer.alloc(32, 9).toString("base64")}`;
const destination = "https://receiver.example.com/callback";
const params = (project: string, extra: Record<string, unknown> = {}) => ({ name: "work.status_changed", arguments: { project_id: project }, delivery: { mode: "webhook", url: destination, secret }, ...extra });
function stopParams(input: ReturnType<typeof params>) {
  return { name: input.name, arguments: input.arguments, delivery: { mode: "webhook", url: input.delivery.url } };
}
async function setup() {
  const { db, sqlite } = fixture();
  const work = new Work(db, "owner"), project = await work.createProject({ title: "Synthetic fixture" });
  let time = Date.now(), allowed = true, available = true, status = 200;
  const requests: { url: string; init: RequestInit; body: Record<string, unknown> }[] = [];
  const host: EventHost = {
    encryptionKey: await crypto.subtle.generateKey({ name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"]),
    assertDispatchReady: async () => { if (!available) throw new Error("No dispatcher"); },
    ownerHasAccess: async () => allowed,
    webhookFetch: async (url, init) => {
      const body = JSON.parse(String(init.body)); requests.push({ url, init, body });
      assert.equal(init.redirect, "error"); assert(init.signal);
      return body.type === "verification" ? Response.json({ challenge: body.challenge }) : new Response(null, { status });
    },
  };
  const clock = () => time;
  const events = new McpEvents(db, "owner", host, clock);
  const run = () => dispatchEvents(db, host, "owner", clock);
  const deliveries = () => requests.filter(r => r.body.eventId);
  const node = () => work.createNode({ project_id: project.id, title: "Synthetic work", summary: "Synthetic summary", body: "Sensitive body excluded", artifacts: ["https://example.com/private"] });
  return { db, sqlite, work, project, host, events, requests, deliveries, run, node, clock,
    advance: (ms: number) => { time += ms; }, deny: () => { allowed = false; }, unavailable: () => { available = false; }, status: (value: number) => { status = value; } };
}
function verifySignature(request: { init: RequestInit }, key = secret) {
  const headers = new Headers(request.init.headers);
  const expected = createHmac("sha256", Buffer.from(key.slice(6), "base64"))
    .update(`${headers.get("webhook-id")}.${headers.get("webhook-timestamp")}.${request.init.body}`).digest("base64");
  assert(headers.get("webhook-signature")!.split(" ").includes(`v1,${expected}`));
}

test("outbox includes committed transitions and decision creation with history IDs, versions and minimal data", async () => {
  const s = await setup();
  try {
    let n = await s.node();
    const update = async (fields: Record<string, unknown>) => { n = await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: n.version, ...fields }); };
    await update({ status: "working" }); await update({ status: "blocked", blocker: "Synthetic blocker" });
    await update({ status: "done" }); await update({ status: "working" });
    await update({ status: "stopped", stop_reason: "Synthetic reason" }); await update({ status: "blocked", blocker: "Again" });
    await update({ summary: "Changed text" });
    const decision = await s.work.createNode({ project_id: s.project.id, kind: "decision", title: "Synthetic choice", body: "Full rationale" });
    await s.work.updateNode({ project_id: s.project.id, node_id: decision.id, expected_version: 1, body: "Revised rationale" });
    await s.work.link({ project_id: s.project.id, source_id: n.id, target_id: decision.id, relation: "informed_by" });
    const rows = s.sqlite.prepare("SELECT * FROM mcp_event_outbox ORDER BY rowid").all();
    assert.deepEqual(rows.map(r => r.transition), ["blocked", "done", "reopened", "stopped", "reopened", "created"]);
    assert.deepEqual(rows.map(r => r.version), [3, 4, 5, 6, 7, 1]);
    assert.equal(rows[5].name, "decision.created");
    for (const row of rows) assert(s.sqlite.prepare("SELECT 1 FROM activity WHERE id=?").get(row.id!));
    assert(!JSON.stringify(rows).includes("Sensitive body")); assert(!JSON.stringify(rows).includes("Full rationale"));
  } finally { s.sqlite.close(); }
});

test("no-op, stale, losing concurrent and rolled-back writes produce no event", async () => {
  const s = await setup();
  try {
    const n = await s.node();
    const updates = await Promise.allSettled(["done", "blocked"].map(status => s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: 1, status, blocker: "Fixture" })));
    assert.equal(updates.filter(r => r.status === "fulfilled").length, 1);
    const current = await s.work.node(s.project.id, n.id);
    await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: current.version, status: current.status });
    await assert.rejects(s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: 1, status: "done" }));
    assert.equal(s.sqlite.prepare("SELECT count(*) AS n FROM mcp_event_outbox").get()!.n, 1);
    s.sqlite.exec("CREATE TRIGGER reject_event BEFORE INSERT ON mcp_event_outbox BEGIN SELECT RAISE(ABORT,'outbox failed'); END");
    await assert.rejects(s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: current.version, status: "stopped", stop_reason: "Fixture" }), /outbox failed/);
    assert.deepEqual(await s.work.node(s.project.id, n.id), current);
    await assert.rejects(s.work.createNode({ project_id: s.project.id, kind: "decision", title: "Rollback" }), /outbox failed/);
    assert.equal(s.sqlite.prepare("SELECT count(*) AS n FROM nodes WHERE kind='decision'").get()!.n, 0);
  } finally { s.sqlite.close(); }
});

test("signed single-use challenge, encrypted persistence, canonical identity, cache and restart refresh", async () => {
  const s = await setup();
  try {
    const n = await s.node(), input = params(s.project.id, { arguments: { node_id: n.id, project_id: s.project.id, status: "done" }, ttlMs: 120_000 });
    const first = await s.events.subscribe(input); verifySignature(s.requests[0]);
    assert.equal(s.requests[0].body.type, "verification"); assert.equal(s.requests[0].body.data, undefined);
    const stored = s.sqlite.prepare("SELECT * FROM mcp_subscriptions").get()!;
    assert(!String(stored.secret).includes(secret)); assert.equal(Number(stored.expires_at) - s.clock(), 120_000);
    s.advance(10_000);
    const refreshed = await new McpEvents(s.db, "owner", s.host, s.clock).subscribe({ ...input, arguments: { status: "done", project_id: s.project.id, node_id: n.id } });
    assert.equal(first.id, refreshed.id); assert.equal(s.requests.length, 1);
    assert(new Date(refreshed.refreshBefore).getTime() > new Date(first.refreshBefore).getTime());
    assert.equal(s.sqlite.prepare("SELECT count(*) AS n FROM mcp_subscriptions").get()!.n, 1);
    assert.notEqual(first.id, await subscriptionId("another", input.name, input.arguments, destination));
  } finally { s.sqlite.close(); }
});

test("tenant/project/node/status filters prevent unrelated delivery, and payload supports authoritative rereads", async () => {
  const s = await setup();
  try {
    const n = await s.node(), another = await s.node();
    await s.events.subscribe(params(s.project.id, { arguments: { project_id: s.project.id, node_id: n.id, status: "done" } }));
    await s.events.subscribe(params(s.project.id, { name: "decision.created" }));
    await s.work.updateNode({ project_id: s.project.id, node_id: another.id, expected_version: 1, status: "done" });
    const blocked = await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: 1, status: "blocked", blocker: "Fixture" });
    const done = await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: blocked.version, status: "done" });
    const p = await s.work.createProject({ title: "Other project" }), other = new Work(s.db, "other");
    await s.work.createNode({ project_id: p.id, kind: "decision", title: "Excluded project" });
    const q = await other.createProject({ title: "Private tenant" });
    await other.createNode({ project_id: q.id, kind: "decision", title: "Excluded tenant" });
    const decision = await s.work.createNode({ project_id: s.project.id, kind: "decision", title: "Included choice" });
    await s.run();
    assert.equal(s.deliveries().length, 2);
    const work = s.deliveries().find(r => r.body.name === "work.status_changed")!;
    const data = work.body.data as Record<string, unknown>;
    assert.equal(data.node_id, done.id); assert.equal(data.version, done.version); assert.equal(data.previous_status, "blocked");
    assert.equal(data.summary, "Synthetic summary"); assert.equal(data.body, undefined); assert.equal(data.artifacts, undefined);
    assert(String(data.url).includes(encodeURIComponent(n.id))); verifySignature(work);
    assert.equal((s.deliveries().find(r => r.body.name === "decision.created")!.body.data as Record<string, unknown>).node_id, decision.id);
    await s.run(); assert.equal(s.deliveries().length, 2);
  } finally { s.sqlite.close(); }
});

test("subscription authorization rejects other tenants and mismatched node kinds before callbacks", async () => {
  const s = await setup();
  try {
    const other = new McpEvents(s.db, "other", s.host, s.clock), n = await s.node();
    await assert.rejects(other.subscribe(params(s.project.id)), /not found/);
    await assert.rejects(s.events.subscribe(params(s.project.id, { name: "decision.created", arguments: { project_id: s.project.id, node_id: n.id } })), /kind/);
    await assert.rejects(s.events.subscribe(params(s.project.id, { name: "decision.created", arguments: { project_id: s.project.id, status: "done" } })), /status filter/);
    await assert.rejects(s.events.subscribe(params(s.project.id, { arguments: { project_id: s.project.id, owner_id: "other" } })));
    assert.equal(s.requests.length, 0);
    const original = await s.events.subscribe(params(s.project.id));
    await other.unsubscribe(stopParams(params(s.project.id)));
    assert.equal(s.sqlite.prepare("SELECT active FROM mcp_subscriptions WHERE id=?").get(original.id)!.active, 1);
  } finally { s.sqlite.close(); }
});

test("callback URL syntax and secrets fail before any outbound request; transport handles DNS/pinning rejection", async () => {
  const s = await setup();
  try {
    for (const url of ["http://receiver.example.com", "https://127.0.0.1", "https://2130706433", "https://[::1]", "https://[::ffff:127.0.0.1]", "https://localhost", "https://foo.local", "https://foo.internal", "https://x:y@receiver.example.com", "https://receiver.example.com:444", "https://receiver.example.com/#x", "https://receiver.example.com./"]) assert.throws(() => callbackUrl(url));
    for (const value of ["secret", "whsec_a", `whsec_${Buffer.alloc(23).toString("base64")}`, `whsec_${Buffer.alloc(65).toString("base64")}`]) await assert.rejects(s.events.subscribe(params(s.project.id, { delivery: { mode: "webhook", url: destination, secret: value } })));
    assert.equal(s.requests.length, 0);
    s.host.webhookFetch = async () => { throw new Error("Private DNS address rejected by test transport"); };
    await assert.rejects(s.events.subscribe(params(s.project.id)), (e: unknown) => e instanceof EventError && e.reason === "challenge_failed");
    assert.equal(s.sqlite.prepare("SELECT count(*) AS n FROM mcp_subscriptions").get()!.n, 0);
  } finally { s.sqlite.close(); }
});

test("incorrect, reused, oversized, timeout and redirect challenge responses never activate subscriptions", async () => {
  const s = await setup();
  try {
    const responses = [
      async () => Response.json({ challenge: "wrong" }),
      async () => new Response("x".repeat(5000)),
      async () => new Response(null, { status: 302, headers: { location: "https://evil.example.com" } }),
      async () => { throw new DOMException("timeout", "TimeoutError"); },
    ];
    for (const respond of responses) {
      s.host.webhookFetch = respond;
      await assert.rejects(s.events.subscribe(params(s.project.id)), (e: unknown) => e instanceof EventError && e.code === -32015);
    }
    let previous = "";
    s.host.webhookFetch = async (_url, init) => {
      const challenge = JSON.parse(String(init.body)).challenge;
      const response = Response.json({ challenge: previous || challenge }); previous = challenge; return response;
    };
    await s.events.subscribe(params(s.project.id)); s.advance(301_000);
    await assert.rejects(s.events.subscribe(params(s.project.id)), /verification failed/);
    assert.equal(s.sqlite.prepare("SELECT count(*) AS n FROM mcp_subscriptions").get()!.n, 1);
  } finally { s.sqlite.close(); }
});

test("persistent retries use bounded backoff, stable IDs/body and fresh independent signatures after restart", async () => {
  const s = await setup();
  try {
    await s.events.subscribe(params(s.project.id)); const n = await s.node();
    await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: 1, status: "done" });
    s.status(503); await s.run(); await s.run(); assert.equal(s.deliveries().length, 1);
    s.advance(1000); await dispatchEvents(s.db, s.host, "owner", s.clock);
    assert.equal(s.deliveries().length, 2);
    assert.equal(s.deliveries()[0].init.body, s.deliveries()[1].init.body);
    assert.notEqual(new Headers(s.deliveries()[0].init.headers).get("webhook-signature"), new Headers(s.deliveries()[1].init.headers).get("webhook-signature"));
    for (const delivery of s.deliveries()) verifySignature(delivery);
    s.status(200); s.advance(2000); await s.run(); await s.run(); assert.equal(s.deliveries().length, 3);
    assert.equal(s.sqlite.prepare("SELECT state,attempts FROM mcp_deliveries").get()!.state, "delivered");
    assert.equal(new Set(s.deliveries().map(r => r.body.eventId)).size, 1);
  } finally { s.sqlite.close(); }
});

test("network errors/429 retry at most six times; 410/413/redirect/400 are terminal", async () => {
  for (const status of [429, 410, 413, 302, 400]) {
    const s = await setup();
    try {
      await s.events.subscribe(params(s.project.id)); const n = await s.node();
      await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: 1, status: "done" });
      s.status(status);
      for (let i = 0; i < 8; i++) { await s.run(); s.advance(60_000); }
      assert.equal(s.deliveries().length, status === 429 ? 6 : 1);
      assert.equal(s.sqlite.prepare("SELECT state FROM mcp_deliveries").get()!.state, "failed");
      if (status === 410) assert.equal(s.sqlite.prepare("SELECT active FROM mcp_subscriptions").get()!.active, 0);
    } finally { s.sqlite.close(); }
  }
});

test("leases prevent concurrent dispatch; lost acknowledgments recover with the same event ID", async () => {
  const s = await setup();
  try {
    await s.events.subscribe(params(s.project.id)); const n = await s.node();
    await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: 1, status: "done" });
    await Promise.all([s.run(), s.run()]); assert.equal(s.deliveries().length, 1);
    s.sqlite.exec("UPDATE mcp_deliveries SET state='sending',claim='crashed',lease_until=9999999999999");
    await s.run(); assert.equal(s.deliveries().length, 1);
    s.sqlite.exec("UPDATE mcp_deliveries SET lease_until=0"); await s.run();
    assert.equal(s.deliveries().length, 2); assert.equal(s.deliveries()[0].body.eventId, s.deliveries()[1].body.eventId);
  } finally { s.sqlite.close(); }
});

test("expiration, finite null TTL, refresh and unsubscribe exclude old events and cancel pending retries", async () => {
  const s = await setup();
  try {
    const input = params(s.project.id, { ttlMs: 60_000 });
    await s.events.subscribe(input); let n = await s.node();
    n = await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: n.version, status: "done" });
    s.status(503); await s.run(); s.advance(60_000); await s.run(); assert.equal(s.deliveries().length, 1);
    assert.equal(s.sqlite.prepare("SELECT secret FROM mcp_subscriptions").get()!.secret, "");
    const renewed = await s.events.subscribe({ ...input, ttlMs: null });
    assert.equal(new Date(renewed.refreshBefore).getTime() - s.clock(), 86_400_000);
    await s.run(); assert.equal(s.deliveries().length, 1);
    await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: n.version, status: "working" });
    await s.run(); assert.equal(s.deliveries().length, 2);
    await s.events.unsubscribe(stopParams(input)); await s.events.unsubscribe(stopParams(input));
    s.advance(60_000); await s.run(); assert.equal(s.deliveries().length, 2);
    assert.equal(s.sqlite.prepare("SELECT secret,active FROM mcp_subscriptions").get()!.active, 0);
    assert.equal(s.sqlite.prepare("SELECT secret FROM mcp_subscriptions").get()!.secret, "");
  } finally { s.sqlite.close(); }
});

test("replacement signing secret gets verified and dual signatures end after rotation window", async () => {
  const s = await setup();
  try {
    const input = params(s.project.id); await s.events.subscribe(input);
    const rotated = await s.events.subscribe({ ...input, delivery: { ...input.delivery, secret: replacement } });
    verifySignature(s.requests[1], replacement);
    // An identical refresh preserves the old key for the remaining rotation window.
    await s.events.subscribe({ ...input, delivery: { ...input.delivery, secret: replacement } });
    const n = await s.node();
    const done = await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: 1, status: "done" });
    await s.run(); verifySignature(s.deliveries()[0]); verifySignature(s.deliveries()[0], replacement);
    s.advance(301_000);
    await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: done.version, status: "working" });
    await s.run(); verifySignature(s.deliveries()[1], replacement);
    assert.equal(new Headers(s.deliveries()[1].init.headers).get("webhook-signature")!.split(" ").length, 1);
    assert.equal(s.sqlite.prepare("SELECT previous_secret FROM mcp_subscriptions WHERE id=?").get(rotated.id)!.previous_secret, null);
  } finally { s.sqlite.close(); }
});

test("revoked application authorization and changed project ownership cancel deliveries", async () => {
  for (const revoke of ["grant", "project"]) {
    const s = await setup();
    try {
      const input = params(s.project.id); await s.events.subscribe(input); const n = await s.node();
      await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: 1, status: "done" });
      s.status(503); await s.run(); s.advance(1000);
      if (revoke === "grant") s.deny(); else s.sqlite.prepare("UPDATE projects SET owner_id='other' WHERE id=?").run(s.project.id);
      await s.run(); assert.equal(s.deliveries().length, 1);
      assert.equal(s.sqlite.prepare("SELECT state FROM mcp_deliveries").get()!.state, "cancelled");
      await s.events.unsubscribe(stopParams(input));
    } finally { s.sqlite.close(); }
  }
});

test("older retry can follow newer event; immutable versions remain safe for stale optimistic writes", async () => {
  const s = await setup();
  try {
    await s.events.subscribe(params(s.project.id)); let n = await s.node();
    n = await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: 1, status: "done" });
    s.status(503); await s.run();
    n = await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: n.version, status: "working" });
    s.status(200); await s.run(); s.advance(1000); await s.run();
    assert.deepEqual(s.deliveries().map(r => (r.body.data as Record<string, unknown>).version), [2, 3, 2]);
    await assert.rejects(s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: 2, status: "done" }), /reconcile/);
    assert.equal((await s.work.node(s.project.id, n.id)).version, 3);
  } finally { s.sqlite.close(); }
});

test("default host is disabled without outbound traffic; dispatcher health is mandatory", async () => {
  const s = await setup();
  try {
    await assert.rejects(new McpEvents(s.db, "owner").subscribe(params(s.project.id)), (e: unknown) => e instanceof EventError && e.reason === "host_unavailable");
    s.unavailable();
    await assert.rejects(s.events.subscribe(params(s.project.id)), (e: unknown) => e instanceof EventError && e.reason === "dispatch_unavailable");
    await assert.rejects(s.run(), /dispatcher/);
    assert.equal(s.requests.length, 0);
  } finally { s.sqlite.close(); }
});

const modernRequest = (method: string, parameters: Record<string, unknown> = {}, owner = "owner", name = parameters.name) => new Request("https://agen8.example.com/mcp", {
  method: "POST", headers: { "mcp-protocol-version": "2026-07-28", "mcp-method": method,
    ...(owner ? { "oai-authenticated-user-id": owner } : {}), ...(typeof name === "string" ? { "mcp-name": name } : {}) },
  body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params: { ...parameters, _meta: { "io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": {} } } }),
});
type Rpc = { result: { capabilities: Record<string, unknown>; events: unknown[]; tools: unknown[]; id: string; cacheScope: string }; error: { code: number; data: { reason: string } } };
const rpc = async (response: Promise<Response>) => await (await response).json() as Rpc;
test("MCP catalog is authenticated/private, host-gated and modern-only; original tools remain unchanged", async () => {
  const s = await setup();
  try {
    const runtime = { host: s.host, db: s.db };
    const discovery = await rpc(mcpResponse(modernRequest("server/discover"))); assert.equal(discovery.result.capabilities.events, undefined);
    const disabled = await rpc(mcpResponse(modernRequest("events/list"))); assert.deepEqual(disabled.result.events, []);
    const missing = await mcpResponse(modernRequest("events/list", {}, ""), runtime); assert.equal(missing.status, 401);
    const listed = await rpc(mcpResponse(modernRequest("events/list"), runtime));
    assert.deepEqual(listed.result.events, JSON.parse(JSON.stringify(eventDefinitions))); assert.equal(listed.result.cacheScope, "private");
    assert.equal((await rpc(mcpResponse(modernRequest("server/discover"), runtime))).result.capabilities.events !== undefined, true);
    const subscription = await rpc(mcpResponse(modernRequest("events/subscribe", params(s.project.id)), runtime)); assert(subscription.result.id);
    const mismatch = await rpc(mcpResponse(modernRequest("events/subscribe", params(s.project.id), "owner", "decision.created"), runtime)); assert.equal(mismatch.error.code, -32020);
    const unavailable = await rpc(mcpResponse(modernRequest("events/subscribe", params(s.project.id)))); assert.equal(unavailable.error.data.reason, "host_unavailable");
    const legacy = new Request("https://agen8.example.com/mcp", { method: "POST", body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "events/list" }) });
    assert.equal((await rpc(mcpResponse(legacy, runtime))).error.code, -32601);
    const plain = await rpc(mcpResponse(modernRequest("tools/list"))), configured = await rpc(mcpResponse(modernRequest("tools/list"), runtime));
    assert.deepEqual(plain, configured); assert.equal(plain.result.tools.length, 9);
  } finally { s.sqlite.close(); }
});

test("Standard Webhooks size limit and timestamp signing cover exact serialized UTF-8 bytes", async () => {
  const body = JSON.stringify({ summary: "Unicode α🙂", eventId: "evt_fixture" });
  const headers = await webhookHeaders("evt_fixture", "sub_fixture", body, [secret], 1791060000000);
  verifySignature({ init: { headers, body } });
  assert.equal(headers["webhook-timestamp"], "1791060000");
  await assert.rejects(webhookHeaders("evt_fixture", "sub_fixture", "🙂".repeat(70_000), [secret], 0), /256 KiB/);
});

test("unsubscribe or expiration during authorization prevents sending already-claimed events", async () => {
  for (const cancel of ["unsubscribe", "expire"]) {
    const s = await setup();
    try {
      const input = params(s.project.id, { ttlMs: 60_000 }); await s.events.subscribe(input);
      const n = await s.node(); await s.work.updateNode({ project_id: s.project.id, node_id: n.id, expected_version: 1, status: "done" });
      s.host.ownerHasAccess = async () => {
        if (cancel === "unsubscribe") await s.events.unsubscribe(stopParams(input)); else s.advance(60_000);
        return true;
      };
      await s.run(); assert.equal(s.deliveries().length, 0);
      assert.equal(s.sqlite.prepare("SELECT state FROM mcp_deliveries").get()!.state, "cancelled");
    } finally { s.sqlite.close(); }
  }
});
