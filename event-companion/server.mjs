import { createServer } from "node:http";
import { createHash, timingSafeEqual } from "node:crypto";
import { readFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";
import { publicHttpsPost, httpsDestination } from "./transport.mjs";

export function credential(value) {
  if (typeof value !== "string" || !/^[A-Za-z0-9+/]+={0,2}$/.test(value) || Buffer.from(value, "base64").length !== 32 || Buffer.from(value, "base64").toString("base64") !== value) throw new Error("A canonical base64 32-byte credential is required.");
  return value;
}
const hash = value => createHash("sha256").update(value).digest();
export const authorized = (header, token) => typeof header === "string" && timingSafeEqual(hash(header), hash(`Bearer ${token}`));
export function relayEnvelope(input, now = Date.now()) {
  if (!input || typeof input !== "object" || Object.keys(input).sort().join(",") !== "body,headers,url") throw new Error("invalid_envelope");
  httpsDestination(input.url);
  if (typeof input.body !== "string" || Buffer.byteLength(input.body) > 262_144 || !input.headers || typeof input.headers !== "object") throw new Error("invalid_envelope");
  const expected = ["content-type", "webhook-id", "webhook-timestamp", "webhook-signature", "x-mcp-subscription-id"];
  const headers = Object.fromEntries(Object.entries(input.headers).map(([key, value]) => [key.toLowerCase(), value]));
  if (Object.keys(headers).length !== expected.length || expected.some(key => typeof headers[key] !== "string")) throw new Error("invalid_headers");
  if (headers["content-type"] !== "application/json" || !/^[\w-]{1,200}$/.test(headers["webhook-id"]) ||
      !/^[\w-]{1,200}$/.test(headers["x-mcp-subscription-id"]) || !/^\d{1,12}$/.test(headers["webhook-timestamp"]) ||
      Math.abs(now / 1000 - Number(headers["webhook-timestamp"])) > 300 ||
      headers["webhook-signature"].length > 200 || !/^v1,[A-Za-z0-9+/]+={0,2}( v1,[A-Za-z0-9+/]+={0,2})*$/.test(headers["webhook-signature"])) throw new Error("invalid_headers");
  const body = JSON.parse(input.body);
  if (!body || typeof body !== "object" || (body.type === "verification" ? typeof body.challenge !== "string" || body.challenge.length > 200 : body.type !== undefined || body.eventId !== headers["webhook-id"] || !["work.status_changed", "decision.created"].includes(body.name))) throw new Error("invalid_event");
  return { ...input, headers };
}
async function boundedBody(request) {
  const parts = []; let length = 0;
  for await (const chunk of request) {
    length += chunk.length; if (length > 2 * 1024 * 1024) throw new Error("request_too_large"); parts.push(chunk);
  }
  return JSON.parse(Buffer.concat(parts).toString("utf8"));
}
export function relayServer({ token, heartbeatPath, send = publicHttpsPost, readHeartbeat = () => readFile(heartbeatPath, "utf8"), clock = Date.now, capacity = 8 }) {
  credential(token); let active = 0;
  const server = createServer(async (request, response) => {
    const json = (status, value) => { response.writeHead(status, { "Content-Type": "application/json", "Cache-Control": "no-store" }); response.end(JSON.stringify(value)); };
    if (!authorized(request.headers.authorization, token)) { response.setHeader("Connection", "close"); json(401, { error: "unauthorized" }); return; }
    if (request.url === "/health" && request.method === "GET") {
      let ready = false;
      try { const { startedAt } = JSON.parse(await readHeartbeat()); ready = Number.isSafeInteger(startedAt) && clock() >= startedAt && clock() - startedAt < 90_000; } catch { /* No fresh independent scheduler invocation. */ }
      json(ready ? 200 : 503, { ready }); return;
    }
    if (request.url !== "/relay" || request.method !== "POST") { json(404, { error: "not_found" }); return; }
    if (active >= capacity) { json(429, { error: "capacity" }); return; }
    active++;
    try {
      const input = relayEnvelope(await boundedBody(request), clock());
      const controller = new AbortController();
      response.on("close", () => { if (!response.writableFinished) controller.abort(); });
      const receipt = await send(input.url, input.body, input.headers, { signal: controller.signal });
      json(200, receipt);
    } catch { json(502, { error: "callback_rejected" }); }
    finally { active--; }
  });
  server.requestTimeout = 12_000; server.headersTimeout = 5_000; server.maxHeadersCount = 20;
  return server;
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const token = credential(process.env.AGEN8_RELAY_TOKEN);
  const heartbeatPath = process.env.AGEN8_HEARTBEAT_PATH;
  if (!heartbeatPath?.startsWith("/")) throw new Error("An absolute heartbeat file path is required.");
  relayServer({ token, heartbeatPath }).listen(8788, "127.0.0.1");
}
