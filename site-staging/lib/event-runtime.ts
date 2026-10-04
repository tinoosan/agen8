import { z } from "zod";
import { database } from "./database";
import { createRelayHost, dispatchKeyMatches } from "./event-relay";
import { createDispatchGrant, grantOwnerAccess, revokeDispatchGrant } from "./event-grants";
import { readEventResponse } from "./mcp-events";
import { WorkError } from "./validation";

export type EventEnvironment = {
  AGEN8_EVENTS_ENABLED?: string;
  AGEN8_EVENT_RELAY_URL?: string;
  AGEN8_EVENT_RELAY_TOKEN?: string;
  AGEN8_EVENT_ENCRYPTION_KEY?: string;
  AGEN8_EVENT_GRANT_ID?: string;
  AGEN8_EVENT_DISPATCH_KEY?: string;
};
async function environment() { return (await import("cloudflare:workers")).env as EventEnvironment; }
function required(value: string | undefined) { if (!value) throw new Error("Event runtime configuration is incomplete."); return value; }

/** Server-only runtime wiring. Deployment settings default to disabled. */
export async function configuredEvents(env?: EventEnvironment, db?: D1Database) {
  env ??= await environment();
  if (env.AGEN8_EVENTS_ENABLED !== "true") return undefined;
  const grantId = required(env.AGEN8_EVENT_GRANT_ID), dispatchKey = required(env.AGEN8_EVENT_DISPATCH_KEY);
  const configuration = { url: required(env.AGEN8_EVENT_RELAY_URL), relayToken: required(env.AGEN8_EVENT_RELAY_TOKEN), encryptionKey: required(env.AGEN8_EVENT_ENCRYPTION_KEY) };
  await dispatchKeyMatches(dispatchKey, dispatchKey);
  db ??= await database();
  const host = await createRelayHost(configuration, grantOwnerAccess(db, grantId));
  return { db, host, grantId, dispatchKey };
}

/** Operational owner grant endpoint, separate from the read-only graph UI.
 * Requires Sites identity and the dispatch key. Service callers cannot renew grants.
 */
export async function eventAccessResponse(request: Request, env?: EventEnvironment, db?: D1Database) {
  if (request.method !== "POST") return new Response(null, { status: 405 });
  env ??= await environment();
  if (!env.AGEN8_EVENT_DISPATCH_KEY) return Response.json({ error: "Event access setup is disabled." }, { status: 503 });
  const owner = request.headers.get("oai-authenticated-user-id");
  if (!owner) return Response.json({ error: "Sign in with ChatGPT to manage event access." }, { status: 401 });
  try {
    if (!await dispatchKeyMatches(request.headers.get("x-agen8-dispatch-key"), env.AGEN8_EVENT_DISPATCH_KEY)) return Response.json({ error: "Event access is unauthorized." }, { status: 401 });
    const input = z.discriminatedUnion("action", [
      z.object({ action: z.literal("grant"), ttlMs: z.number().int().min(60_000).max(86_400_000).default(86_400_000) }).strict(),
      z.object({ action: z.literal("revoke"), id: z.string().min(1).max(100) }).strict(),
    ]).parse(await readEventResponse(new Response(request.body), 4096));
    db ??= await database();
    if (input.action === "grant") return Response.json(await createDispatchGrant(db, request, input.ttlMs));
    await revokeDispatchGrant(db, owner, input.id);
    return Response.json({});
  } catch (error) {
    return Response.json({ error: error instanceof z.ZodError || error instanceof SyntaxError ? "Invalid event access request." : error instanceof WorkError ? error.message : "Event access is unavailable." }, { status: error instanceof z.ZodError || error instanceof SyntaxError ? 400 : error instanceof WorkError ? error.status : 503 });
  }
}
