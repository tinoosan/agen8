const RETENTION = 30 * 86_400_000;

/** Bounded, owner-scoped cleanup. Keep queued events and their deduplication rows.
 * AUTOINCREMENT sequences survive pruning, so new subscriptions/events cannot
 * reuse a cursor after an idle period or database maintenance.
 */
export async function pruneEvents(db: D1Database, owner: string, clock = Date.now) {
  const at = clock(), cutoff = at - RETENTION;
  await db.batch([
    db.prepare(`UPDATE mcp_subscriptions SET active=0,secret='',previous_secret=NULL,rotation_until=NULL
      WHERE owner_id=? AND (expires_at<=? OR EXISTS(SELECT 1 FROM mcp_dispatch_grants g WHERE g.owner_id=? AND (g.active=0 OR g.expires_at<=?)))`).bind(owner, at, owner, at),
    db.prepare(`UPDATE mcp_deliveries SET state='cancelled',claim=NULL,lease_until=0,next_attempt_at=?
      WHERE state IN ('pending','sending') AND subscription_id IN (SELECT id FROM mcp_subscriptions WHERE owner_id=? AND active=0)`).bind(at, owner),
    db.prepare(`DELETE FROM mcp_event_outbox WHERE sequence IN (
      SELECT e.sequence FROM mcp_event_outbox e WHERE e.owner_id=? AND e.occurred_at<?
      AND NOT EXISTS(SELECT 1 FROM mcp_deliveries d WHERE d.event_id=e.id AND d.state IN ('pending','sending'))
      AND NOT EXISTS(SELECT 1 FROM mcp_subscriptions s WHERE s.owner_id=e.owner_id AND s.project_id=e.project_id AND s.name=e.name
        AND s.active=1 AND s.expires_at>? AND e.sequence>s.start_sequence
        AND (s.node_id IS NULL OR s.node_id=e.node_id) AND (s.status IS NULL OR s.status=e.transition)
        AND NOT EXISTS(SELECT 1 FROM mcp_deliveries d WHERE d.subscription_id=s.id AND d.event_id=e.id))
      ORDER BY e.sequence LIMIT 250)`).bind(owner, new Date(cutoff).toISOString(), at),
    db.prepare(`DELETE FROM mcp_deliveries WHERE rowid IN (
      SELECT d.rowid FROM mcp_deliveries d JOIN mcp_subscriptions s ON s.id=d.subscription_id
      WHERE s.owner_id=? AND d.state NOT IN ('pending','sending') AND d.next_attempt_at<?
        AND NOT EXISTS(SELECT 1 FROM mcp_event_outbox e WHERE e.id=d.event_id)
      LIMIT 250)`).bind(owner, cutoff),
    db.prepare(`DELETE FROM mcp_subscriptions WHERE id IN (SELECT s.id FROM mcp_subscriptions s
      WHERE s.owner_id=? AND s.active=0 AND s.expires_at<?
        AND NOT EXISTS(SELECT 1 FROM mcp_deliveries d WHERE d.subscription_id=s.id) LIMIT 250)`).bind(owner, cutoff),
  ]);
}
