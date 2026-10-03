import { writeFile, rename, mkdir } from "node:fs/promises";
import { dirname } from "node:path";
import { pathToFileURL } from "node:url";
import { publicHttpsPost, httpsDestination } from "./transport.mjs";
import { credential } from "./server.mjs";

export async function dispatchOnce({ url, dispatchKey, identityBearer, heartbeatPath }, { send = publicHttpsPost, clock = Date.now, heartbeat } = {}) {
  const destination = httpsDestination(url);
  if (destination.pathname !== "/api/events/dispatch" || destination.search) throw new Error("Dispatch requires the exact Site dispatch route.");
  credential(dispatchKey);
  if (!identityBearer || /[\r\n]/.test(identityBearer)) throw new Error("Approved identity-bearing Site authorization is required; service bypass alone is insufficient.");
  const record = JSON.stringify({ startedAt: clock() });
  if (heartbeat) await heartbeat(record);
  else {
    if (!heartbeatPath?.startsWith("/")) throw new Error("An absolute heartbeat file path is required.");
    await mkdir(dirname(heartbeatPath), { recursive: true, mode: 0o750 });
    const temporary = `${heartbeatPath}.${process.pid}`;
    await writeFile(temporary, record, { mode: 0o640 }); await rename(temporary, heartbeatPath);
  }
  const receipt = await send(destination.href, "{}", {
    "Content-Type": "application/json", Authorization: `Bearer ${identityBearer}`, "X-Agen8-Dispatch-Key": dispatchKey,
  }, { timeoutMs: 20_000 });
  if (receipt.status !== 200) throw new Error("Site dispatch was rejected.");
  const result = JSON.parse(receipt.body);
  if (!Number.isInteger(result.attempted) || result.attempted < 0 || result.attempted > 1) throw new Error("Invalid dispatch receipt.");
  return { attempted: result.attempted };
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const result = await dispatchOnce({ url: process.env.AGEN8_DISPATCH_URL, dispatchKey: process.env.AGEN8_DISPATCH_KEY,
      identityBearer: process.env.AGEN8_SITE_IDENTITY_BEARER, heartbeatPath: process.env.AGEN8_HEARTBEAT_PATH });
    console.log(JSON.stringify(result));
  } catch { console.error("Agen8 dispatch failed; check authorization and relay readiness."); process.exitCode = 1; }
}
