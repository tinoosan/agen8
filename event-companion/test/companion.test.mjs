import test from "node:test";
import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { Readable } from "node:stream";
import { mkdtemp, readFile, stat, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { publicAddress, publicLookup, publicHttpsPost, httpsDestination } from "../transport.mjs";
import { credential, relayEnvelope, relayServer } from "../server.mjs";
import { dispatchOnce } from "../dispatch.mjs";

const token = Buffer.alloc(32, 7).toString("base64");
const url = "https://receiver.example.com/callback";
const headers = () => ({ "content-type": "application/json", "webhook-id": "evt_fixture", "webhook-timestamp": String(Math.floor(Date.now() / 1000)), "webhook-signature": "v1,YWJj", "x-mcp-subscription-id": "sub_fixture" });
const body = JSON.stringify({ eventId: "evt_fixture", name: "work.status_changed", data: { summary: "Synthetic fixture" } });
function lookup(resolve, all = false) { return new Promise((accept, reject) => publicLookup(resolve)("receiver.example.com", { all }, (error, address, family) => error ? reject(error) : accept({ address, family }))); }
function fakeSocket({ status = 200, chunks = [Buffer.from('{"challenge":"fixture"}')], failure, records = [{ address: "93.184.215.14", family: 4 }] } = {}) {
  const calls = []; let resolutions = 0;
  return {
    calls, resolutions: () => resolutions,
    resolve: async (hostname, options) => { resolutions++; assert.equal(hostname, "receiver.example.com"); assert.equal(options.all, true); return records; },
    request: (destination, options, callback) => {
      const req = new EventEmitter();
      req.destroy = error => req.emit("error", error);
      options.signal.addEventListener("abort", () => req.emit("error", options.signal.reason), { once: true });
      req.end = sent => options.lookup(destination.hostname, { all: false }, (error, address, family) => {
        if (error) { req.emit("error", error); return; }
        calls.push({ destination, options, sent, address, family });
        if (failure) { req.emit("error", new Error(failure)); return; }
        const response = Readable.from(chunks); response.statusCode = status;
        callback(response);
      });
      return req;
    },
  };
}
async function hit(server, { path = "/relay", method = "POST", auth = `Bearer ${token}`, input = { url, headers: headers(), body } } = {}) {
  const request = Readable.from([Buffer.from(JSON.stringify(input))]);
  Object.assign(request, { method, url: path, headers: { authorization: auth } });
  const response = new EventEmitter(); let status;
  response.setHeader = () => {};
  response.writeHead = value => { status = value; };
  const finished = new Promise(resolve => { response.end = data => { response.writableFinished = true; resolve({ status, body: JSON.parse(data) }); }; });
  server.emit("request", request, response); return finished;
}

test("public address classification excludes local, private, reserved, mapped and tunnel destinations", () => {
  for (const address of ["93.184.215.14", "8.8.8.8", "2606:4700:4700::1111", "2001:4860:4860::8888"]) assert.equal(publicAddress(address), true, address);
  for (const address of ["0.0.0.0", "10.0.0.1", "100.64.0.1", "127.0.0.1", "169.254.169.254", "172.16.0.1", "192.168.0.1", "192.0.0.1", "192.0.2.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1", "255.255.255.255", "::", "::1", "fc00::1", "fe80::1", "fe80::1%en0", "ff02::1", "2001:db8::1", "::ffff:127.0.0.1", "::ffff:8.8.8.8", "64:ff9b::808:808", "2002:0808:0808::1", "2001::1", "2001:20::1", "not-an-ip"]) assert.equal(publicAddress(address), false, address);
});

test("socket lookup checks all answers and passes only validated numeric addresses to the connector", async () => {
  const records = [{ address: "93.184.215.14", family: 4 }, { address: "2606:4700:4700::1111", family: 6 }];
  assert.deepEqual(await lookup(async () => records), { address: records[0].address, family: 4 });
  assert.deepEqual((await lookup(async () => records, true)).address, records);
  for (const result of [[], [{ address: "127.0.0.1", family: 4 }], [...records, { address: "10.1.1.1", family: 4 }], [{ address: "93.184.215.14", family: 6 }]]) await assert.rejects(lookup(async () => result), /rejected/);
});

test("DNS rebinding is rechecked on each connection without a second resolution; TLS hostname and exact body survive pinning", async () => {
  const socket = fakeSocket();
  const result = await publicHttpsPost(url, body, headers(), socket);
  assert.equal(result.status, 200); assert.equal(socket.resolutions(), 1);
  const call = socket.calls[0];
  assert.equal(call.address, "93.184.215.14"); assert.equal(call.destination.hostname, "receiver.example.com");
  assert.equal(call.options.servername, "receiver.example.com"); assert.equal(call.options.rejectUnauthorized, true);
  assert.equal(call.options.autoSelectFamily, false); assert.deepEqual(call.options.agent.options.proxyEnv, {});
  assert.equal(call.sent, body); assert.equal(call.options.headers["Content-Length"], String(Buffer.byteLength(body)));
  let lookups = 0;
  const rebind = { ...socket, resolve: async () => { lookups++; return [{ address: lookups === 1 ? "93.184.215.14" : "127.0.0.1", family: 4 }]; } };
  await publicHttpsPost(url, body, headers(), rebind);
  await assert.rejects(publicHttpsPost(url, body, headers(), rebind), /rejected/);
  assert.equal(lookups, 2);
});

test("literal/encoded IPs, HTTP, credentials, fragments and nonstandard ports fail before connecting", async () => {
  for (const value of ["http://receiver.example.com", "https://127.1", "https://0x7f000001", "https://2130706433", "https://[::1]", "https://[::ffff:127.0.0.1]", "https://foo.local", "https://foo.internal", "https://u:p@receiver.example.com", "https://receiver.example.com:444", "https://receiver.example.com#secret", "https://receiver.example.com."]) assert.throws(() => httpsDestination(value));
  const socket = fakeSocket(); await assert.rejects(publicHttpsPost(url, "x".repeat(262145), headers(), socket), /large/);
  assert.equal(socket.calls.length, 0);
});

test("redirects, bad TLS certificates, oversized replies and aborted exchanges never succeed", async () => {
  for (const fixture of [{ status: 302 }, { failure: "ERR_TLS_CERT_ALTNAME_INVALID" }, { chunks: [Buffer.alloc(5000)] }]) await assert.rejects(publicHttpsPost(url, body, headers(), fakeSocket(fixture)));
  const controller = new AbortController();
  const socket = fakeSocket();
  socket.request = (_destination, options) => { const req = new EventEmitter(); req.end = () => { controller.abort(new Error("aborted")); }; options.signal.addEventListener("abort", () => req.emit("error", options.signal.reason)); return req; };
  await assert.rejects(publicHttpsPost(url, body, headers(), { ...socket, signal: controller.signal }), /aborted/);
});

test("relay allows only authenticated signed event POSTs and bounded protocol headers", async () => {
  const server = relayServer({ token, readHeartbeat: async () => JSON.stringify({ startedAt: Date.now() }), send: async (target, data, h) => {
    assert.equal(target, url); assert.equal(data, body); assert.equal(h.authorization, undefined); return { status: 200, body: "" };
  } });
  assert.equal((await hit(server, { auth: "wrong" })).status, 401);
  assert.equal((await hit(server)).status, 200);
  assert.equal((await hit(server, { method: "GET" })).status, 404);
  assert.equal((await hit(server, { path: "/anything" })).status, 404);
  for (const input of [{ url, headers: { ...headers(), Authorization: "arbitrary" }, body }, { url, headers: headers(), body: '{}' }, { url, headers: { ...headers(), "webhook-id": "different" }, body }, { url, headers: { ...headers(), "webhook-timestamp": "0" }, body }]) assert.throws(() => relayEnvelope(input));
  assert.throws(() => credential("short"));
});

test("readiness expires without independent scheduler invocations; unauthorized callers cannot inspect it", async () => {
  let startedAt = 100_000, now = startedAt;
  const server = relayServer({ token, clock: () => now, readHeartbeat: async () => JSON.stringify({ startedAt }) });
  assert.equal((await hit(server, { path: "/health", method: "GET" })).body.ready, true);
  now += 90_000; assert.equal((await hit(server, { path: "/health", method: "GET" })).status, 503);
  startedAt = now + 1; assert.equal((await hit(server, { path: "/health", method: "GET" })).body.ready, false);
  assert.equal((await hit(server, { path: "/health", method: "GET", auth: "wrong" })).status, 401);
});

test("relay capacity rejects bursts without opening another callback connection", async () => {
  let finish; let sends = 0;
  const server = relayServer({ token, capacity: 1, send: async () => { sends++; return await new Promise(resolve => { finish = resolve; }); } });
  const first = hit(server);
  while (!finish) await new Promise(resolve => setImmediate(resolve));
  assert.equal((await hit(server)).status, 429); assert.equal(sends, 1);
  finish({ status: 200, body: "" }); assert.equal((await first).status, 200);
});

test("dispatch client uses fixed Site route, separate key and service access without fabricating identity", async () => {
  const dir = await mkdtemp(join(tmpdir(), "agen8-events-"));
  try {
    const config = { url: "https://agen8.example.com/api/events/dispatch", dispatchKey: token, siteServiceBearer: "synthetic-service", heartbeatPath: join(dir, "state", "heartbeat.json") };
    let calls = 0;
    const send = async (target, data, h, options) => {
      calls++; assert.equal(target, config.url); assert.equal(data, "{}"); assert.equal(h["X-Agen8-Dispatch-Key"], token);
      assert.equal(h["OAI-Sites-Authorization"], "Bearer synthetic-service"); assert.equal(h.Authorization, undefined); assert.equal(h["oai-authenticated-user-id"], undefined);
      assert.equal(options.timeoutMs, 20_000); return { status: 200, body: '{"attempted":1}' };
    };
    assert.deepEqual(await dispatchOnce(config, { send, clock: () => 1000 }), { attempted: 1 });
    assert.deepEqual(JSON.parse(await readFile(config.heartbeatPath, "utf8")), { startedAt: 1000 });
    assert.equal((await stat(config.heartbeatPath)).mode & 0o777, 0o640);
    await assert.rejects(dispatchOnce({ ...config, url: "https://agen8.example.com/mcp" }, { send }));
    for (const siteServiceBearer of ["", "bad\r\nheader", "bad token", "bad\0token"]) await assert.rejects(dispatchOnce({ ...config, siteServiceBearer }, { send }));
    assert.equal(calls, 1);
    await assert.rejects(dispatchOnce(config, { send: async () => ({ status: 401, body: "" }) }), /rejected/);
  } finally { await rm(dir, { recursive: true, force: true }); }
});
