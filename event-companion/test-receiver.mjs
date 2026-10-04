import { createServer } from "node:http";
import { createHmac, timingSafeEqual } from "node:crypto";
import { writeFile, rename } from "node:fs/promises";
import { pathToFileURL } from "node:url";

/** Temporary receiver for approved synthetic staging tests, never general graph data. */
export function testReceiver({ secret, projectId, receiptFile, failures = 2, clock = Date.now }) {
  const key = Buffer.from(String(secret).slice(6), "base64");
  if (!String(secret).startsWith("whsec_") || key.length < 24 || key.length > 64 || `whsec_${key.toString("base64")}` !== secret) throw new Error("A test-only signing secret is required.");
  if (!projectId?.startsWith("project_") || !Number.isInteger(failures) || failures < 0 || failures > 2) throw new Error("A synthetic project and bounded failures are required.");
  const receipts = new Map();
  let receiptWrites = Promise.resolve();
  const server = createServer(async (request, response) => {
    const reply = (status, body = {}) => { response.writeHead(status, { "Content-Type": "application/json" }); response.end(JSON.stringify(body)); };
    if (request.method !== "POST" || request.url !== "/test/callback") return reply(404);
    try {
      const parts = []; let length = 0;
      for await (const part of request) { length += part.length; if (length > 262_144) return reply(413); parts.push(part); }
      const body = Buffer.concat(parts).toString("utf8"), id = request.headers["webhook-id"], timestamp = request.headers["webhook-timestamp"], signatures = request.headers["webhook-signature"];
      if (typeof id !== "string" || id.length > 200 || typeof timestamp !== "string" || !/^\d+$/.test(timestamp) || Math.abs(clock() / 1000 - Number(timestamp)) > 300 || typeof signatures !== "string") return reply(401);
      const expected = createHmac("sha256", key).update(`${id}.${timestamp}.${body}`).digest();
      const valid = signatures.split(" ").some(value => {
        if (!value.startsWith("v1,")) return false;
        const actual = Buffer.from(value.slice(3), "base64");
        return actual.length === expected.length && timingSafeEqual(actual, expected);
      });
      if (!valid) return reply(401);
      const event = JSON.parse(body);
      if (event.type === "verification" && typeof event.challenge === "string") return reply(200, { challenge: event.challenge });
      if (event.eventId !== id || event.data?.project_id !== projectId || !["work.status_changed", "decision.created"].includes(event.name) || !Number.isSafeInteger(event.data?.version)) return reply(400);
      const previous = receipts.get(id);
      const record = previous ?? { eventId: id, name: event.name, version: event.data.version, status: event.data.status, attempts: 0, accepted: false, signingTimestamps: [] };
      record.attempts++; record.signingTimestamps.push(Number(timestamp)); receipts.set(id, record);
      if (record.attempts > failures) record.accepted = true;
      const accepted = record.accepted;
      if (receiptFile) {
        const snapshot = JSON.stringify([...receipts.values()], null, 2);
        const save = async () => { await writeFile(`${receiptFile}.tmp`, snapshot, { mode: 0o600 }); await rename(`${receiptFile}.tmp`, receiptFile); };
        receiptWrites = receiptWrites.then(save, save);
        await receiptWrites;
      }
      return reply(accepted ? 200 : 503);
    } catch { return reply(400); }
  });
  return { server, receipts };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const { server } = testReceiver({ secret: process.env.AGEN8_TEST_WEBHOOK_SECRET, projectId: process.env.AGEN8_TEST_PROJECT_ID, receiptFile: process.env.AGEN8_TEST_RECEIPTS });
    server.listen(8790, "127.0.0.1");
    setTimeout(() => { server.closeAllConnections(); server.close(); }, 10 * 60_000);
  } catch { console.error("Synthetic test receiver configuration is invalid."); process.exitCode = 1; }
}
