import test from "node:test";
import assert from "node:assert/strict";
import { createHmac } from "node:crypto";
import { once } from "node:events";
import { mkdtemp, readFile, rm, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { testReceiver } from "../test-receiver.mjs";

test("real HTTP receiver verifies exact signatures, restricts synthetic project and records stable retry IDs", async () => {
  const secret = `whsec_${Buffer.alloc(32, 7).toString("base64")}`;
  const directory = await mkdtemp(join(tmpdir(), "agen8-receipts-"));
  const receiptFile = join(directory, "receipts.json");
  const receiver = testReceiver({ secret, projectId: "project_synthetic", receiptFile });
  receiver.server.listen(0, "127.0.0.1"); await once(receiver.server, "listening");
  const url = `http://127.0.0.1:${receiver.server.address().port}/test/callback`;
  const send = async (value, id = "event_synthetic", invalid = false) => {
    const body = JSON.stringify(value), timestamp = String(Math.floor(Date.now() / 1000));
    const signature = createHmac("sha256", Buffer.from(secret.slice(6), "base64")).update(`${id}.${timestamp}.${body}`).digest("base64");
    return fetch(url, { method: "POST", body, headers: { "webhook-id": id, "webhook-timestamp": timestamp, "webhook-signature": `v1,${invalid ? "invalid" : signature}` } });
  };
  try {
    assert.equal((await send({ type: "verification", challenge: "synthetic" }, "verification", true)).status, 401);
    assert.deepEqual(await (await send({ type: "verification", challenge: "synthetic" }, "verification")).json(), { challenge: "synthetic" });
    const event = { eventId: "event_synthetic", name: "work.status_changed", data: { project_id: "project_synthetic", version: 2, status: "done" } };
    assert.equal((await send({ ...event, data: { ...event.data, project_id: "project_other" } })).status, 400);
    for (const status of [503, 503, 200, 200]) assert.equal((await send(event)).status, status);
    assert.equal(receiver.receipts.size, 1);
    assert.deepEqual({ ...receiver.receipts.get(event.eventId), signingTimestamps: [] }, { eventId: event.eventId, name: event.name, version: 2, status: "done", attempts: 4, accepted: true, signingTimestamps: [] });
    const parallel = { ...event, eventId: "event_parallel" };
    const responses = await Promise.all(Array.from({ length: 4 }, () => send(parallel, parallel.eventId)));
    assert.deepEqual(responses.map(r => r.status).sort(), [200, 200, 503, 503]);
    const persisted = JSON.parse(await readFile(receiptFile, "utf8"));
    assert.equal(persisted.length, 2); assert.equal(persisted[1].attempts, 4);
    assert.equal((await stat(receiptFile)).mode & 0o777, 0o600);
  } finally {
    receiver.server.closeAllConnections(); await new Promise(resolve => receiver.server.close(resolve));
    await rm(directory, { recursive: true, force: true });
  }
});
