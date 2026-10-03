import test from "node:test";
import assert from "node:assert/strict";
import { fixture } from "./sqlite";
import { Work } from "../lib/work";
import { McpEvents, dispatchEvents } from "../lib/mcp-events";
import { createRelayHost, eventDispatchResponse } from "../lib/event-relay";

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
const dispatchRequest = (owner = "owner", token = key) => new Request("https://agen8.example.com/api/events/dispatch", { method: "POST", headers: { ...(owner ? { "oai-authenticated-user-id": owner } : {}), "x-agen8-dispatch-key": token }, body: "{}" });

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

test("dispatch bridge is disabled by default and requires both trusted owner identity and separate key", async () => {
  const { db, sqlite } = fixture();
  try {
    const host = await createRelayHost(config, async () => true, mockRelay().send);
    const integration = { db, host, owner: "owner", dispatchKey: key };
    assert.equal((await eventDispatchResponse(dispatchRequest())).status, 503);
    for (const request of [dispatchRequest(""), dispatchRequest("other"), dispatchRequest("owner", "wrong")]) assert.equal((await eventDispatchResponse(request, integration)).status, 401);
    assert.equal((await eventDispatchResponse(dispatchRequest(), integration)).status, 200);
    host.ownerHasAccess = async () => false;
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
    await eventDispatchResponse(dispatchRequest(), { db, host, owner: "owner", dispatchKey: key });
    const delivery = relay.calls.filter(c => JSON.parse(String(c.envelope.body)).eventId);
    assert.equal(delivery.length, 1); assert.equal(JSON.parse(String(delivery[0].envelope.body)).data.project_id, p.id);
    assert.equal(sqlite.prepare("SELECT count(*) AS n FROM mcp_deliveries").get()!.n, 1);
    sqlite.prepare("UPDATE mcp_subscriptions SET expires_at=0 WHERE owner_id='other'").run();
    await dispatchEvents(db, host, "owner");
    assert.notEqual(sqlite.prepare("SELECT secret FROM mcp_subscriptions WHERE owner_id='other'").get()!.secret, "");
    await assert.rejects(dispatchEvents(db, host, ""), /owner/);
  } finally { sqlite.close(); }
});
