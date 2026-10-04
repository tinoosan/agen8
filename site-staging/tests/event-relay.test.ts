import test from "node:test";
import assert from "node:assert/strict";
import { fixture } from "./sqlite";
import { Work } from "../lib/work";
import { McpEvents, dispatchEvents } from "../lib/mcp-events";
import { createRelayHost, eventDispatchResponse } from "../lib/event-relay";
import { createDispatchGrant, dispatchGrant, grantOwnerAccess, revokeDispatchGrant } from "../lib/event-grants";

const key = Buffer.alloc(32, 7).toString("base64");
const config = { url: "https://relay.example.com/", relayToken: key, encryptionKey: key };
const callback = "https://receiver.example.com/callback";
const subscribe = (project: string) => ({ name: "decision.created", arguments: { project_id: project }, delivery: { mode: "webhook", url: callback, secret: `whsec_${key}` } });
function mockRelay(healthy = true) {
  const calls: { path: string; envelope: Record<string, unknown> }[] = [];
  const send: typeof fetch = async (input, init) => {
    const url = new URL(String(input)); assert.equal(url.origin, "https://relay.example.com");
    assert.equal(init?.redirect, "error"); assert.equal(new Headers(init?.headers).get("Authorization"), `Bearer ${key}`);
    if (url.pathname === "/health") return Response.json({ ready: healthy }, { status: healthy ? 200 : 503 });
    assert.equal(url.pathname, "/relay");
    const envelope = JSON.parse(String(init?.body)); calls.push({ path: url.pathname, envelope });
    const event = JSON.parse(envelope.body);
    return Response.json({ status: 200, body: event.type === "verification" ? JSON.stringify({ challenge: event.challenge }) : "" });
  };
  return { calls, send };
}
const dispatchRequest = (owner = "", token = key) => new Request("https://agen8.example.com/api/events/dispatch", { method: "POST", headers: { ...(owner ? { "oai-authenticated-user-id": owner } : {}), "x-agen8-dispatch-key": token }, body: "{}" });

test("relay adapter sends exact signed bytes to fixed gateway, never a signing key or owner identity", async () => {
  const { db, sqlite } = fixture(); const relay = mockRelay();
  try {
    const work = new Work(db, "owner"), p = await work.createProject({ title: "Synthetic" });
    const host = await createRelayHost(config, async () => true, relay.send);
    const events = new McpEvents(db, "owner", host); await events.subscribe(subscribe(p.id));
    await work.createNode({ project_id: p.id, kind: "decision", title: "Synthetic decision" });
    await dispatchEvents(db, host, "owner");
    assert.equal(relay.calls.length, 2);
    for (const call of relay.calls) {
      assert.equal(call.envelope.url, callback); assert.equal(call.envelope.secret, undefined);
      assert(!JSON.stringify(call.envelope).includes(`whsec_${key}`));
      const headers = call.envelope.headers as Record<string, string>;
      assert.equal(headers["oai-authenticated-user-id"], undefined); assert.equal(headers.authorization, undefined);
      assert(headers["webhook-signature"].startsWith("v1,"));
    }
  } finally { sqlite.close(); }
});

test("unhealthy relay, redirects, invalid or oversized receipts fail closed", async () => {
  const unhealthy = await createRelayHost(config, async () => true, mockRelay(false).send);
  await assert.rejects(unhealthy.assertDispatchReady());
  for (const receipt of [{ status: 302, body: "" }, { status: 200, body: "x".repeat(4097) }, { status: 200, body: "", unexpected: "extra" }]) {
    const host = await createRelayHost(config, async () => true, async () => Response.json(receipt));
    await assert.rejects(host.webhookFetch(callback, { method: "POST", body: "{}", signal: AbortSignal.timeout(1000) }));
  }
  const huge = await createRelayHost(config, async () => true, async () => new Response("x".repeat(32769)));
  await assert.rejects(huge.assertDispatchReady());
  await assert.rejects(createRelayHost({ ...config, url: "https://relay.example.com/arbitrary" }, async () => true));
  await assert.rejects(createRelayHost({ ...config, encryptionKey: "short" }, async () => true));
});

test("service dispatch derives only its stored Site principal and requires a separate key", async () => {
  const { db, sqlite } = fixture();
  try {
    const grant = await createDispatchGrant(db, dispatchRequest("site-principal"));
    const host = await createRelayHost(config, grantOwnerAccess(db, grant.id), mockRelay().send);
    const integration = { db, host, grantId: grant.id, dispatchKey: key };
    assert.equal((await eventDispatchResponse(dispatchRequest())).status, 503);
    assert.equal((await eventDispatchResponse(dispatchRequest("", "wrong"), integration)).status, 401);
    assert.equal((await eventDispatchResponse(dispatchRequest("fabricated-account-id"), integration)).status, 200);
    assert.equal((await eventDispatchResponse(dispatchRequest(), { ...integration, grantId: "missing" })).status, 403);
    assert.equal((await eventDispatchResponse(dispatchRequest(), integration)).status, 200);
    host.ownerHasAccess = async () => false;
    assert.equal((await eventDispatchResponse(dispatchRequest(), integration)).status, 403);
    host.ownerHasAccess = async () => true;
    sqlite.prepare("UPDATE mcp_dispatch_grants SET expires_at=0 WHERE id=?").run(grant.id);
    assert.equal((await eventDispatchResponse(dispatchRequest(), integration)).status, 403);
  } finally { sqlite.close(); }
});

test("one-owner dispatch cannot fan out, send or clean up another tenant's state", async () => {
  const { db, sqlite } = fixture(); const relay = mockRelay();
  try {
    const host = await createRelayHost(config, async () => true, relay.send);
    const work = new Work(db, "owner"), other = new Work(db, "other");
    const p = await work.createProject({ title: "Owner" }), q = await other.createProject({ title: "Other" });
    await new McpEvents(db, "owner", host).subscribe(subscribe(p.id));
    await new McpEvents(db, "other", host).subscribe(subscribe(q.id));
    await work.createNode({ project_id: p.id, kind: "decision", title: "Owner event" });
    await other.createNode({ project_id: q.id, kind: "decision", title: "Other event" });
    const grant = await createDispatchGrant(db, dispatchRequest("owner"));
    host.ownerHasAccess = grantOwnerAccess(db, grant.id);
    // Create the grant before subscribe; its replacement deliberately revokes prior subscriptions.
    await new McpEvents(db, "owner", host).subscribe(subscribe(p.id));
    await work.createNode({ project_id: p.id, kind: "decision", title: "Owner event after grant" });
    await eventDispatchResponse(dispatchRequest("other"), { db, host, grantId: grant.id, dispatchKey: key });
    const delivery = relay.calls.filter(c => JSON.parse(String(c.envelope.body)).eventId);
    assert.equal(delivery.length, 1); assert.equal(JSON.parse(String(delivery[0].envelope.body)).data.project_id, p.id);
    assert.equal(sqlite.prepare("SELECT count(*) AS n FROM mcp_deliveries").get()!.n, 1);
    sqlite.prepare("UPDATE mcp_subscriptions SET expires_at=0 WHERE owner_id='other'").run();
    await dispatchEvents(db, host, "owner");
    assert.notEqual(sqlite.prepare("SELECT secret FROM mcp_subscriptions WHERE owner_id='other'").get()!.secret, "");
    await assert.rejects(dispatchEvents(db, host, ""), /owner/);
  } finally { sqlite.close(); }
});

test("grants bind authenticated Site identity, survive restart and fail closed on expiry/replacement", async () => {
  const { db, sqlite } = fixture(); let time = Date.now(); const clock = () => time;
  try {
    await assert.rejects(createDispatchGrant(db, dispatchRequest()), /authenticated/);
    for (const ttl of [0, 59_999, 86_400_001, NaN]) await assert.rejects(createDispatchGrant(db, dispatchRequest("site-owner"), ttl));
    const grant = await createDispatchGrant(db, dispatchRequest("site-owner"), 60_000, clock);
    assert.equal(grant.ownerId, "site-owner");
    assert.equal(await grantOwnerAccess(db, grant.id, clock)("account-metadata-id"), false);
    assert.equal(await grantOwnerAccess(db, grant.id, clock)("site-owner"), true);
    time += 60_000;
    assert.equal(await dispatchGrant(db, grant.id, clock), null);
    assert.equal(await grantOwnerAccess(db, grant.id, clock)("site-owner"), false);
    const replacement = await createDispatchGrant(db, dispatchRequest("site-owner"), 60_000, clock);
    assert.notEqual(replacement.id, grant.id);
    assert.equal(await dispatchGrant(db, grant.id, clock), null);
    await revokeDispatchGrant(db, "other", replacement.id);
    assert(await dispatchGrant(db, replacement.id, clock));
    await revokeDispatchGrant(db, "site-owner", replacement.id);
    assert.equal(await dispatchGrant(db, replacement.id, clock), null);
  } finally { sqlite.close(); }
});

test("grant revocation cancels retries, clears secrets, rejects refresh and cannot be undone by subscribe", async () => {
  const { db, sqlite } = fixture(); const relay = mockRelay();
  try {
    const grant = await createDispatchGrant(db, dispatchRequest("owner"));
    const host = await createRelayHost(config, grantOwnerAccess(db, grant.id), relay.send);
    const work = new Work(db, "owner"), p = await work.createProject({ title: "Synthetic" });
    const events = new McpEvents(db, "owner", host), input = subscribe(p.id);
    await events.subscribe(input);
    const expiry = (await dispatchGrant(db, grant.id))!.expires_at;
    await events.subscribe(input);
    assert.equal((await dispatchGrant(db, grant.id))!.expires_at, expiry);
    await work.createNode({ project_id: p.id, kind: "decision", title: "Synthetic choice" });
    const original = host.webhookFetch; host.webhookFetch = async () => new Response(null, { status: 503 });
    await dispatchEvents(db, host, "owner");
    assert.equal(sqlite.prepare("SELECT state FROM mcp_deliveries").get()!.state, "pending");
    await revokeDispatchGrant(db, "owner", grant.id);
    assert.equal(sqlite.prepare("SELECT state FROM mcp_deliveries").get()!.state, "cancelled");
    assert.equal(sqlite.prepare("SELECT secret FROM mcp_subscriptions").get()!.secret, "");
    host.webhookFetch = original;
    await assert.rejects(events.subscribe(input), /unavailable/);
    assert.equal((await eventDispatchResponse(dispatchRequest(), { db, host, grantId: grant.id, dispatchKey: key })).status, 403);
    assert.equal(relay.calls.length, 1);
    const fresh = await createDispatchGrant(db, dispatchRequest("owner"));
    assert.notEqual(fresh.id, grant.id);
    assert.equal(sqlite.prepare("SELECT active FROM mcp_subscriptions").get()!.active, 0);
    assert.equal(await grantOwnerAccess(db, grant.id)("owner"), false);
  } finally { sqlite.close(); }
});

test("grant revoked during challenge or final delivery authorization sends no graph event", async () => {
  for (const stage of ["challenge", "send"]) {
    const { db, sqlite } = fixture(); const relay = mockRelay();
    try {
      const grant = await createDispatchGrant(db, dispatchRequest("owner"));
      const access = grantOwnerAccess(db, grant.id);
      const host = await createRelayHost(config, access, relay.send);
      const work = new Work(db, "owner"), p = await work.createProject({ title: "Synthetic" });
      const events = new McpEvents(db, "owner", host);
      if (stage === "challenge") {
        const original = host.webhookFetch;
        host.webhookFetch = async (url, init) => { const response = await original(url, init); await revokeDispatchGrant(db, "owner", grant.id); return response; };
        await assert.rejects(events.subscribe(subscribe(p.id)), /revoked/);
        assert.equal(sqlite.prepare("SELECT count(*) AS n FROM mcp_subscriptions").get()!.n, 0);
      } else {
        await events.subscribe(subscribe(p.id));
        await work.createNode({ project_id: p.id, kind: "decision", title: "Synthetic" });
        let checks = 0;
        host.ownerHasAccess = async owner => { if (++checks === 2) await revokeDispatchGrant(db, owner, grant.id); return access(owner); };
        await dispatchEvents(db, host, "owner");
        assert.equal(sqlite.prepare("SELECT state FROM mcp_deliveries").get()!.state, "cancelled");
      }
      assert.equal(relay.calls.length, 1);
    } finally { sqlite.close(); }
  }
});
