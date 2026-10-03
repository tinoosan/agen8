import { z } from "zod";
import { callbackUrl, dispatchEvents, readEventResponse, type EventHost } from "./mcp-events";

const configSchema = z.object({ url: z.string(), relayToken: z.string(), encryptionKey: z.string() }).strict();
function keyBytes(value: string) {
  if (!/^[A-Za-z0-9+/]+={0,2}$/.test(value)) throw new Error("Invalid event adapter key.");
  const bytes = Uint8Array.from(atob(value), c => c.charCodeAt(0));
  if (bytes.length !== 32 || btoa(String.fromCharCode(...bytes)) !== value) throw new Error("Invalid event adapter key.");
  return bytes;
}
/** Fixed trusted relay origin is deployment configuration, never subscription input.
 * ownerHasAccess must come from a verified platform integration; no default grant.
 * The relay receives signed bytes only, never the webhook signing key or D1 credentials.
 */
export async function createRelayHost(configuration: z.infer<typeof configSchema>, ownerHasAccess: EventHost["ownerHasAccess"], relayFetch = fetch): Promise<EventHost> {
  const config = configSchema.parse(configuration), base = new URL(callbackUrl(config.url));
  if (base.pathname !== "/" || base.search) throw new Error("Configure only the trusted relay HTTPS origin.");
  keyBytes(config.relayToken);
  const encryptionKey = await crypto.subtle.importKey("raw", keyBytes(config.encryptionKey), "AES-GCM", false, ["encrypt", "decrypt"]);
  const call = async (path: string, init: RequestInit) => {
    const response = await relayFetch(new URL(path, base), { ...init, redirect: "error",
      headers: { Authorization: `Bearer ${config.relayToken}`, "Content-Type": "application/json" } });
    if (response.redirected || !response.ok) { await response.body?.cancel(); throw new Error("Event relay is unavailable."); }
    const value = await readEventResponse(response, 32_768);
    if (!value) throw new Error("Invalid relay response.");
    return value;
  };
  return {
    encryptionKey, ownerHasAccess,
    assertDispatchReady: async () => {
      const response = await call("/health", { method: "GET", signal: AbortSignal.timeout(5000) });
      if (response.ready !== true) throw new Error("Independent event dispatch is unavailable.");
    },
    webhookFetch: async (url, init) => {
      const response = await call("/relay", { method: "POST", signal: init.signal,
        body: JSON.stringify({ url: callbackUrl(url), body: init.body, headers: Object.fromEntries(new Headers(init.headers)) }) });
      const receipt = z.object({ status: z.number().int().min(100).max(599), body: z.string().max(4096) }).strict().parse(response);
      if (receipt.status >= 300 && receipt.status < 400) throw new Error("Callback redirect rejected.");
      return new Response(receipt.body || null, { status: receipt.status });
    },
  };
}
async function keyMatches(supplied: string | null, expected: string) {
  if (!supplied) return false;
  keyBytes(expected);
  const encoder = new TextEncoder();
  const left = new Uint8Array(await crypto.subtle.digest("SHA-256", encoder.encode(supplied)));
  const right = new Uint8Array(await crypto.subtle.digest("SHA-256", encoder.encode(expected)));
  let mismatch = 0; for (let i = 0; i < left.length; i++) mismatch |= left[i] ^ right[i];
  return mismatch === 0;
}
export async function eventDispatchResponse(request: Request, integration?: { db: D1Database; host: EventHost; owner: string; dispatchKey: string }) {
  if (!integration) return Response.json({ error: "Event dispatch integration is unavailable." }, { status: 503 });
  const owner = request.headers.get("oai-authenticated-user-id");
  if (request.method !== "POST") return new Response(null, { status: 405 });
  // Sites must supply this identity. A service bypass token alone never grants owner access.
  try {
    if (!owner || owner !== integration.owner || !await keyMatches(request.headers.get("x-agen8-dispatch-key"), integration.dispatchKey)) return Response.json({ error: "Event dispatch is unauthorized." }, { status: 401 });
    if (!await integration.host.ownerHasAccess(owner)) return Response.json({ error: "Event access was revoked." }, { status: 403 });
    return Response.json(await dispatchEvents(integration.db, integration.host, owner, Date.now, 1));
  } catch { return Response.json({ error: "Event dispatch is unavailable." }, { status: 503 }); }
}
