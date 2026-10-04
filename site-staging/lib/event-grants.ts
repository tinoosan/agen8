import { WorkError } from "./validation";

const DAY = 86_400_000;
type Grant = { id: string; owner_id: string; expires_at: number };

/** Privileged setup helper behind the separately keyed operational access route.
 * Call only after approval, with a Sites-authenticated request on this Site.
 * Never derive this principal from account metadata, email or a request body.
 * Replacing a grant cancels its subscriptions; subscribe cannot renew a grant.
 */
export async function createDispatchGrant(db: D1Database, request: Request, ttlMs = DAY, clock = Date.now) {
  const owner = request.headers.get("oai-authenticated-user-id");
  if (!owner) throw new WorkError("A Sites-authenticated owner is required.", 401);
  if (!Number.isSafeInteger(ttlMs) || ttlMs < 60_000 || ttlMs > DAY) throw new WorkError("Dispatch grants require a lifetime between one minute and 24 hours.");
  const id = `grant_${crypto.randomUUID()}`, expiresAt = clock() + ttlMs;
  await db.batch([
    db.prepare("UPDATE mcp_deliveries SET state='cancelled',claim=NULL WHERE subscription_id IN (SELECT id FROM mcp_subscriptions WHERE owner_id=?) AND state IN ('pending','sending')").bind(owner),
    db.prepare("UPDATE mcp_subscriptions SET active=0,secret='',previous_secret=NULL,rotation_until=NULL WHERE owner_id=?").bind(owner),
    db.prepare("INSERT INTO mcp_dispatch_grants (id,owner_id,active,expires_at) VALUES (?,?,1,?) ON CONFLICT(owner_id) DO UPDATE SET id=excluded.id,active=1,expires_at=excluded.expires_at").bind(id, owner, expiresAt),
  ]);
  return { id, ownerId: owner, expiresAt };
}

export async function dispatchGrant(db: D1Database, id: string, clock = Date.now) {
  return db.prepare("SELECT id,owner_id,expires_at FROM mcp_dispatch_grants WHERE id=? AND active=1 AND expires_at>?").bind(id, clock()).first<Grant>();
}

/** Use the same stored grant for MCP authorization and background delivery. */
export function grantOwnerAccess(db: D1Database, id: string, clock = Date.now) {
  return async (owner: string) => (await dispatchGrant(db, id, clock))?.owner_id === owner;
}

/** Privileged revocation; owner must be the authenticated Site principal. */
export async function revokeDispatchGrant(db: D1Database, owner: string, id: string) {
  await db.batch([
    db.prepare("UPDATE mcp_dispatch_grants SET active=0 WHERE id=? AND owner_id=?").bind(id, owner),
    db.prepare("UPDATE mcp_deliveries SET state='cancelled',claim=NULL WHERE subscription_id IN (SELECT s.id FROM mcp_subscriptions s JOIN mcp_dispatch_grants g ON g.owner_id=s.owner_id WHERE g.id=? AND g.owner_id=? AND g.active=0) AND state IN ('pending','sending')").bind(id, owner),
    db.prepare("UPDATE mcp_subscriptions SET active=0,secret='',previous_secret=NULL,rotation_until=NULL WHERE owner_id IN (SELECT owner_id FROM mcp_dispatch_grants WHERE id=? AND owner_id=? AND active=0)").bind(id, owner),
  ]);
}
