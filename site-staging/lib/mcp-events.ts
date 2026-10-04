import { z } from "zod";
import { Work } from "./work";
import { WorkError, idSchema } from "./validation";
import { graphUrl } from "./navigation";

const names = ["work.status_changed", "decision.created"] as const;
const transitions = ["blocked", "done", "stopped", "reopened"] as const;
const filters = z.object({ project_id: idSchema, node_id: idSchema.optional(), status: z.enum(transitions).optional() }).strict();
type Filters = z.infer<typeof filters>;
type Row = Record<string, string | number | null>;
const DAY = 86_400_000, MINUTE = 60_000, ROTATION = 5 * MINUTE;
const encoder = new TextEncoder();
const bytes = (value: string) => Uint8Array.from(atob(value), c => c.charCodeAt(0));
const base64 = (value: ArrayBuffer | Uint8Array) => btoa(String.fromCharCode(...new Uint8Array(value)));

export class EventError extends Error {
  constructor(public code: number, message: string, public reason?: string) { super(message); }
}

/** No default implementation: ordinary Worker fetch cannot pin public DNS answers.
 * The hosting integration must enforce public destination IPs at every connection,
 * preserve the original TLS hostname, reject redirects and honour the abort signal.
 * assertDispatchReady must check an independently recurring durable dispatcher.
 * ownerHasAccess must check the current revocable Site-owner application grant.
 * Resource ownership is rechecked separately; no visitor identity is needed for dispatch.
 */
export type EventHost = {
  webhookFetch: (url: string, init: RequestInit) => Promise<Response>;
  assertDispatchReady: () => Promise<void>;
  ownerHasAccess: (owner: string) => Promise<boolean>;
  encryptionKey: CryptoKey;
};
export const eventDefinitions = names.map(name => ({
  name, delivery: ["webhook"],
  description: name === "decision.created" ? "A decision was logged in your project. Reread the record for current reasoning." : "Work became blocked, done or stopped, or reopened after done/stopped. Reread the versioned record before acting.",
  inputSchema: { type: "object", properties: {
    project_id: { type: "string", description: "Exact project ID owned by the connected account." },
    node_id: { type: "string", description: "Optional exact record ID in that project." },
    ...(name === "work.status_changed" ? { status: { type: "string", enum: transitions, description: "Transition filter. Reopened includes done/stopped to planned, working or blocked; payload status is the resulting record state." } } : {}),
  }, required: ["project_id"], additionalProperties: false },
  payloadSchema: { type: "object", properties: {
    project_id: { type: "string" }, node_id: { type: "string" }, version: { type: "integer", minimum: 1 },
    status: { type: "string", enum: ["planned", "working", "blocked", "done", "stopped"] },
    ...(name === "work.status_changed" ? { previous_status: { type: "string" }, transition: { type: "string", enum: transitions } } : {}),
    summary: { type: "string", maxLength: 360 }, url: { type: "string" },
  }, required: ["project_id", "node_id", "version", "status", "summary", "url", ...(name === "work.status_changed" ? ["previous_status", "transition"] : [])], additionalProperties: false },
}));

export function callbackUrl(input: unknown) {
  const raw = z.string().max(2048).parse(input);
  let url: URL;
  try { url = new URL(raw); } catch { throw new EventError(-32602, "Invalid callback URL."); }
  // This is syntax validation only. The host MUST still validate and pin DNS at connection time.
  if (url.protocol !== "https:" || url.username || url.password || url.hash || (url.port && url.port !== "443") ||
      !url.hostname.includes(".") || /[:\[\]]/.test(url.hostname) || /^\d+\.\d+\.\d+\.\d+$/.test(url.hostname) ||
      /\.(localhost|local|internal|test|invalid)$/.test(url.hostname) || url.hostname.endsWith(".")) {
    throw new EventError(-32602, "Callback requires a public HTTPS hostname on port 443.");
  }
  return url.href;
}
function signingKey(secret: unknown) {
  if (typeof secret !== "string" || secret.length < 38 || secret.length > 94 || !/^whsec_[A-Za-z0-9+/]+={0,2}$/.test(secret)) throw new EventError(-32602, "Invalid webhook signing secret.");
  let key: Uint8Array<ArrayBuffer>;
  try { key = bytes(secret.slice(6)); } catch { throw new EventError(-32602, "Invalid webhook signing secret."); }
  if (key.length < 24 || key.length > 64 || base64(key) !== secret.slice(6)) throw new EventError(-32602, "Invalid webhook signing secret.");
  return key;
}
async function digest(value: string) {
  return Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", encoder.encode(value))), b => b.toString(16).padStart(2, "0")).join("");
}
function canonical(args: Filters) {
  return JSON.stringify({ project_id: args.project_id, ...(args.node_id ? { node_id: args.node_id } : {}), ...(args.status ? { status: args.status } : {}) });
}
export async function subscriptionId(owner: string, name: string, args: Filters, url: string) {
  return `sub_${await digest(JSON.stringify([owner, name, canonical(args), url]))}`;
}
export async function webhookHeaders(id: string, subscription: string, body: string, secrets: string[], at: number) {
  if (encoder.encode(body).length > 262_144) throw new EventError(-32602, "Event exceeds 256 KiB.");
  const timestamp = String(Math.floor(at / 1000));
  const signatures = await Promise.all(secrets.map(async secret => {
    const key = await crypto.subtle.importKey("raw", signingKey(secret), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
    return `v1,${base64(await crypto.subtle.sign("HMAC", key, encoder.encode(`${id}.${timestamp}.${body}`)))}`;
  }));
  return { "Content-Type": "application/json", "webhook-id": id, "webhook-timestamp": timestamp,
    "webhook-signature": signatures.join(" "), "X-MCP-Subscription-Id": subscription };
}
async function seal(key: CryptoKey, identity: string, secret: string) {
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const encrypted = await crypto.subtle.encrypt({ name: "AES-GCM", iv, additionalData: encoder.encode(identity) }, key, encoder.encode(secret));
  return `${base64(iv)}.${base64(encrypted)}`;
}
async function unseal(key: CryptoKey, identity: string, encrypted: string) {
  const [iv, value] = encrypted.split(".");
  return new TextDecoder().decode(await crypto.subtle.decrypt({ name: "AES-GCM", iv: bytes(iv), additionalData: encoder.encode(identity) }, key, bytes(value)));
}
function constantEqual(a: string, b: string) {
  const left = encoder.encode(a), right = encoder.encode(b);
  let difference = left.length ^ right.length;
  for (let i = 0; i < left.length; i++) difference |= left[i] ^ (right[i] ?? 0);
  return difference === 0;
}

export class McpEvents {
  constructor(private db: D1Database, private owner: string, private host?: EventHost, private clock = Date.now) {
    if (!owner) throw new WorkError("Sign in with ChatGPT to access events.", 401);
  }
  private sql(query: string, ...values: unknown[]) { return this.db.prepare(query).bind(...values); }
  private async identity(params: Record<string, unknown>, subscribe: boolean) {
    const input = z.object({ name: z.enum(names), arguments: filters,
      delivery: z.object({ mode: z.literal("webhook"), url: z.string(), ...(subscribe ? { secret: z.string() } : {}) }).strict(),
      ...(subscribe ? { ttlMs: z.number().int().positive().safe().nullable().optional(), cursor: z.null().optional() } : {}),
    }).strict().parse(params);
    if (input.name === "decision.created" && input.arguments.status) throw new EventError(-32602, "Decisions do not have a status filter.");
    const work = new Work(this.db, this.owner);
    if (subscribe) await work.project(input.arguments.project_id);
    if (subscribe && input.arguments.node_id) {
      const node = await work.node(input.arguments.project_id, input.arguments.node_id);
      if ((node.kind === "decision") !== (input.name === "decision.created")) throw new EventError(-32602, "Record kind does not match event.");
    }
    const url = callbackUrl(input.delivery.url);
    return { ...input, url, id: await subscriptionId(this.owner, input.name, input.arguments, url) };
  }
  async subscribe(params: Record<string, unknown>) {
    const input = await this.identity(params, true);
    const secret = (input.delivery as { secret: string }).secret;
    signingKey(secret);
    if (!this.host) throw new EventError(-32015, "MCP Events needs a supported secure callback transport and durable dispatcher.", "host_unavailable");
    try { await this.host.assertDispatchReady(); } catch { throw new EventError(-32015, "Durable event dispatch is unavailable.", "dispatch_unavailable"); }
    if (!await this.host.ownerHasAccess(this.owner)) throw new WorkError("Event access is unavailable for this account.", 403);
    const secretHash = await digest(secret), at = this.clock();
    const cached = await this.sql("SELECT verified_at FROM mcp_subscriptions WHERE owner_id=? AND url=? AND secret_hash=? AND active=1 AND expires_at>? AND verified_at>? LIMIT 1", this.owner, input.url, secretHash, at, at - ROTATION).first<Row>();
    if (!cached) await this.verify(input.id, input.url, secret);
    const expires = this.clock() + Math.max(MINUTE, Math.min(Number(input.ttlMs ?? DAY), DAY));
    const encrypted = await seal(this.host.encryptionKey, JSON.stringify([this.owner, input.id]), secret);
    // Verification/encryption may overlap revocation or resource access changes.
    if (!await this.host.ownerHasAccess(this.owner)) throw new WorkError("Event access was revoked.", 403);
    const work = new Work(this.db, this.owner);
    await work.project(input.arguments.project_id);
    if (input.arguments.node_id) await work.node(input.arguments.project_id, input.arguments.node_id);
    await this.sql(`INSERT INTO mcp_subscriptions
      (id,owner_id,name,arguments,project_id,node_id,status,url,secret,secret_hash,verified_at,expires_at,start_sequence,active)
      VALUES (?,?,?,?,?,?,?,?,?,?,?,?,(SELECT COALESCE(MAX(rowid),0) FROM mcp_event_outbox),1)
      ON CONFLICT(id) DO UPDATE SET
      previous_secret=CASE WHEN mcp_subscriptions.secret_hash<>excluded.secret_hash THEN mcp_subscriptions.secret ELSE mcp_subscriptions.previous_secret END,
      rotation_until=CASE WHEN mcp_subscriptions.secret_hash<>excluded.secret_hash THEN ? ELSE mcp_subscriptions.rotation_until END,
      secret=excluded.secret,secret_hash=excluded.secret_hash,verified_at=excluded.verified_at,expires_at=excluded.expires_at,active=1,
      start_sequence=CASE WHEN mcp_subscriptions.expires_at<=? OR mcp_subscriptions.active=0 THEN excluded.start_sequence ELSE mcp_subscriptions.start_sequence END`,
      input.id, this.owner, input.name, canonical(input.arguments), input.arguments.project_id, input.arguments.node_id ?? null,
      input.arguments.status ?? null, input.url, encrypted, secretHash, cached ? cached.verified_at : this.clock(), expires, this.clock() + ROTATION, this.clock()).run();
    return { id: input.id, refreshBefore: new Date(expires).toISOString(), cursor: null, truncated: false };
  }
  private async verify(id: string, url: string, secret: string) {
    const challenge = crypto.randomUUID(), at = this.clock(), body = JSON.stringify({ type: "verification", challenge });
    try {
      const response = await this.host!.webhookFetch(url, { method: "POST", redirect: "error", signal: AbortSignal.timeout(10_000),
        headers: await webhookHeaders(`msg_verification_${crypto.randomUUID()}`, id, body, [secret], at), body });
      if (!response.ok || response.redirected || this.clock() - at >= 10_000) throw new Error();
      const returned = await readEventResponse(response);
      if (!returned || typeof returned.challenge !== "string" || !constantEqual(challenge, returned.challenge) || this.clock() - at >= 10_000) throw new Error();
    } catch (error) {
      throw new EventError(-32015, "Callback verification failed.", error instanceof Error && ["TimeoutError", "AbortError"].includes(error.name) ? "timeout" : "challenge_failed");
    }
  }
  async unsubscribe(params: Record<string, unknown>) {
    const input = await this.identity(params, false);
    await this.db.batch([
      this.sql("UPDATE mcp_subscriptions SET active=0,secret='',previous_secret=NULL,rotation_until=NULL WHERE id=? AND owner_id=?", input.id, this.owner),
      this.sql("UPDATE mcp_deliveries SET state='cancelled',claim=NULL WHERE subscription_id IN (SELECT id FROM mcp_subscriptions WHERE id=? AND owner_id=?) AND state IN ('pending','sending')", input.id, this.owner),
    ]);
    return {};
  }
}
export async function readEventResponse(response: Response, maxBytes = 4096): Promise<Record<string, unknown> | null> {
  if (!response.body) return null;
  const reader = response.body.getReader();
  const parts: Uint8Array[] = []; let length = 0;
  try {
    while (true) {
      const { done, value } = await reader.read(); if (done) break;
      length += value.length; if (length > maxBytes) return null; parts.push(value);
    }
  } finally { await reader.cancel(); }
  const result = new Uint8Array(length); let offset = 0;
  for (const part of parts) { result.set(part, offset); offset += part.length; }
  const value: unknown = JSON.parse(new TextDecoder().decode(result));
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

/** Invoke only from a verified durable host scheduler. No timers or waitUntil fallback. */
export async function dispatchEvents(db: D1Database, host: EventHost, owner: string, clock = Date.now, limit = 25) {
  if (!owner) throw new WorkError("A grant-bound dispatch owner is required.", 401);
  await host.assertDispatchReady();
  const sql = (query: string, ...values: unknown[]) => db.prepare(query).bind(...values);
  const at = clock();
  await sql(`INSERT INTO mcp_deliveries (subscription_id,event_id,state,next_attempt_at)
    SELECT s.id,e.id,'pending',? FROM mcp_subscriptions s JOIN mcp_event_outbox e
      ON e.owner_id=s.owner_id AND e.project_id=s.project_id AND e.name=s.name
    JOIN projects p ON p.id=e.project_id AND p.owner_id=s.owner_id
    WHERE s.owner_id=? AND s.active=1 AND s.expires_at>? AND e.rowid>s.start_sequence
      AND (s.node_id IS NULL OR s.node_id=e.node_id) AND (s.status IS NULL OR s.status=e.transition)
    ON CONFLICT(subscription_id,event_id) DO NOTHING`, at, owner, at).run();
  const due = await sql(`SELECT d.subscription_id,d.event_id FROM mcp_deliveries d
    JOIN mcp_subscriptions s ON s.id=d.subscription_id AND s.owner_id=?
    WHERE (d.state='pending' AND d.next_attempt_at<=?) OR (d.state='sending' AND d.lease_until<=?)
    ORDER BY d.next_attempt_at,d.event_id LIMIT ?`, owner, at, at, Math.max(1, Math.min(limit, 100))).all<Row>();
  let attempted = 0;
  for (const item of due.results) {
    const claim = crypto.randomUUID();
    const claimed = await sql(`UPDATE mcp_deliveries SET state='sending',claim=?,lease_until=?,attempts=attempts+1
      WHERE subscription_id=? AND event_id=? AND attempts<6 AND ((state='pending' AND next_attempt_at<=?) OR (state='sending' AND lease_until<=?)) RETURNING attempts`,
      claim, clock() + MINUTE, item.subscription_id, item.event_id, clock(), clock()).first<Row>();
    if (!claimed) {
      await sql("UPDATE mcp_deliveries SET state='failed' WHERE subscription_id=? AND event_id=? AND attempts>=6 AND lease_until<=?", item.subscription_id, item.event_id, clock()).run();
      continue;
    }
    const current = await sql(`SELECT s.*,e.id AS event_id,e.version,e.status AS event_status,e.previous_status,e.transition,e.summary,e.occurred_at,e.node_id AS event_node
      FROM mcp_subscriptions s JOIN mcp_event_outbox e ON e.id=? AND e.owner_id=s.owner_id AND e.project_id=s.project_id AND e.name=s.name
      JOIN projects p ON p.id=e.project_id AND p.owner_id=s.owner_id
      JOIN nodes n ON n.id=e.node_id AND n.project_id=p.id
      WHERE s.id=? AND s.active=1 AND s.expires_at>? AND e.rowid>s.start_sequence`, item.event_id, item.subscription_id, clock()).first<Row>();
    const finish = async (state: string, next = clock()) => {
      await sql("UPDATE mcp_deliveries SET state=?,next_attempt_at=?,lease_until=0,claim=NULL WHERE subscription_id=? AND event_id=? AND claim=?", state, next, item.subscription_id, item.event_id, claim).run();
    };
    if (!current) { await finish("cancelled"); continue; }
    let status = 0;
    try {
      if (!await host.ownerHasAccess(String(current.owner_id))) { await finish("cancelled"); continue; }
      const identity = JSON.stringify([current.owner_id, current.id]);
      const secrets = [await unseal(host.encryptionKey, identity, String(current.secret))];
      if (current.previous_secret && Number(current.rotation_until) > clock()) secrets.push(await unseal(host.encryptionKey, identity, String(current.previous_secret)));
      const body = JSON.stringify({ eventId: current.event_id, name: current.name, timestamp: current.occurred_at, cursor: null,
        data: { project_id: current.project_id, node_id: current.event_node, version: current.version, status: current.event_status,
          ...(current.name === "work.status_changed" ? { previous_status: current.previous_status, transition: current.transition } : {}),
          summary: current.summary, url: graphUrl(String(current.project_id), String(current.event_node)) } });
      const headers = await webhookHeaders(String(current.event_id), String(current.id), body, secrets, clock());
      // Authorization/crypto awaits can overlap grant revocation, unsubscribe or expiration.
      if (!await host.ownerHasAccess(String(current.owner_id))) { await finish("cancelled"); continue; }
      const stillActive = await sql(`SELECT s.secret_hash FROM mcp_subscriptions s
        JOIN projects p ON p.id=s.project_id AND p.owner_id=s.owner_id
        JOIN nodes n ON n.id=? AND n.project_id=p.id
        JOIN mcp_event_outbox e ON e.id=? AND e.rowid>s.start_sequence
        WHERE s.id=? AND s.active=1 AND s.expires_at>?`, current.event_node, current.event_id, current.id, clock()).first<Row>();
      if (!stillActive) { await finish("cancelled"); continue; }
      if (stillActive.secret_hash !== current.secret_hash) { await finish("pending", clock() + 1000); continue; }
      const response = await host.webhookFetch(callbackUrl(current.url), { method: "POST", redirect: "error", signal: AbortSignal.timeout(10_000),
        headers, body });
      status = response.redirected ? 302 : response.status;
      await response.body?.cancel();
    } catch { /* Network or secure transport rejection: bounded retry, never log callback data or secrets. */ }
    attempted++;
    if (status === 410) {
      await sql("UPDATE mcp_subscriptions SET active=0,secret='',previous_secret=NULL WHERE id=?", current.id).run();
      await finish("failed");
    } else if (status >= 200 && status < 300) await finish("delivered");
    else if (status === 0 || status === 408 || status === 429 || status >= 500) {
      const attempt = Number(claimed.attempts);
      await finish(attempt >= 6 ? "failed" : "pending", clock() + Math.min(30 * MINUTE, 1000 * 2 ** (attempt - 1)));
    } else await finish("failed");
  }
  await sql("UPDATE mcp_subscriptions SET secret='',previous_secret=NULL,rotation_until=NULL,active=0 WHERE owner_id=? AND expires_at<=?", owner, clock()).run();
  await sql("UPDATE mcp_subscriptions SET previous_secret=NULL,rotation_until=NULL WHERE owner_id=? AND rotation_until<=?", owner, clock()).run();
  return { attempted };
}
