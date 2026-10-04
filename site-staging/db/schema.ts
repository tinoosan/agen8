import { sqliteTable, text, integer, index, uniqueIndex } from "drizzle-orm/sqlite-core";

export const preferences = sqliteTable("preferences", {
  ownerId: text("owner_id").primaryKey(),
  shareSelectedContext: integer("share_selected_context", { mode: "boolean" }).notNull().default(true),
});

export const projects = sqliteTable("projects", {
  id: text("id").primaryKey(), ownerId: text("owner_id").notNull(),
  title: text("title").notNull(), objective: text("objective").notNull().default(""),
  status: text("status").notNull().default("open"),
  createdAt: text("created_at").notNull(), updatedAt: text("updated_at").notNull(),
}, t => [index("projects_owner").on(t.ownerId, t.updatedAt)]);
// Retained legacy storage. Application records no longer read or write this table.
export const agents = sqliteTable("agents", {
  id: text("id").primaryKey(), projectId: text("project_id").notNull().references(() => projects.id),
  sessionRef: text("session_ref").notNull(), name: text("name").notNull(),
  focus: text("focus").notNull().default(""), lastSeenAt: text("last_seen_at").notNull(),
}, t => [uniqueIndex("agents_project_session").on(t.projectId, t.sessionRef)]);
export const nodes = sqliteTable("nodes", {
  id: text("id").primaryKey(), projectId: text("project_id").notNull().references(() => projects.id),
  kind: text("kind").notNull(), title: text("title").notNull(),
  summary: text("summary").notNull().default(""), body: text("body").notNull().default(""),
  status: text("status").notNull().default("pending"), agentId: text("agent_id"),
  stopReason: text("stop_reason").notNull().default(""),
  blocker: text("blocker").notNull().default(""), outcome: text("outcome").notNull().default(""),
  artifacts: text("artifacts").notNull().default("[]"), version: integer("version").notNull().default(1), mutationId: text("mutation_id").notNull().default(""),
  createdAt: text("created_at").notNull(), updatedAt: text("updated_at").notNull(),
}, t => [index("nodes_project_updated").on(t.projectId, t.updatedAt)]);
export const links = sqliteTable("links", {
  id: text("id").primaryKey(), projectId: text("project_id").notNull().references(() => projects.id),
  sourceId: text("source_id").notNull().references(() => nodes.id), targetId: text("target_id").notNull().references(() => nodes.id),
  relation: text("relation").notNull(), rationale: text("rationale").notNull().default(""), createdAt: text("created_at").notNull(),
}, t => [uniqueIndex("links_identity").on(t.projectId, t.sourceId, t.targetId, t.relation)]);
export const activity = sqliteTable("activity", {
  id: text("id").primaryKey(), projectId: text("project_id").notNull().references(() => projects.id),
  nodeId: text("node_id"), agentId: text("agent_id"), event: text("event").notNull(),
  summary: text("summary").notNull(), createdAt: text("created_at").notNull(),
  details: text("details").notNull().default("{}"),
}, t => [index("activity_project_created").on(t.projectId, t.createdAt), index("activity_node").on(t.projectId, t.nodeId)]);

// Minimal committed notifications. No bodies, artifacts or callback credentials.
export const mcpDispatchGrants = sqliteTable("mcp_dispatch_grants", {
  id: text("id").primaryKey(), ownerId: text("owner_id").notNull(),
  active: integer("active").notNull(), expiresAt: integer("expires_at").notNull(),
}, t => [uniqueIndex("mcp_dispatch_grant_owner").on(t.ownerId)]);
export const mcpEventOutbox = sqliteTable("mcp_event_outbox", {
  sequence: integer("sequence").primaryKey({ autoIncrement: true }),
  id: text("id").notNull().unique(), ownerId: text("owner_id").notNull(),
  projectId: text("project_id").notNull(), nodeId: text("node_id").notNull(),
  name: text("name").notNull(), version: integer("version").notNull(),
  status: text("status").notNull(), previousStatus: text("previous_status"),
  transition: text("transition").notNull(), summary: text("summary").notNull(),
  occurredAt: text("occurred_at").notNull(),
}, t => [index("mcp_events_owner_project").on(t.ownerId, t.projectId)]);
export const mcpSubscriptions = sqliteTable("mcp_subscriptions", {
  id: text("id").primaryKey(), ownerId: text("owner_id").notNull(), name: text("name").notNull(),
  arguments: text("arguments").notNull(), projectId: text("project_id").notNull(),
  nodeId: text("node_id"), status: text("status"), url: text("url").notNull(),
  secret: text("secret").notNull(), secretHash: text("secret_hash").notNull(),
  previousSecret: text("previous_secret"), rotationUntil: integer("rotation_until"),
  verifiedAt: integer("verified_at").notNull(), expiresAt: integer("expires_at").notNull(),
  startSequence: integer("start_sequence").notNull(), active: integer("active").notNull(),
}, t => [index("mcp_subscriptions_owner").on(t.ownerId), index("mcp_subscriptions_callback").on(t.ownerId, t.url)]);
export const mcpDeliveries = sqliteTable("mcp_deliveries", {
  subscriptionId: text("subscription_id").notNull(), eventId: text("event_id").notNull(),
  state: text("state").notNull(), attempts: integer("attempts").notNull().default(0),
  nextAttemptAt: integer("next_attempt_at").notNull(), leaseUntil: integer("lease_until").notNull().default(0),
  claim: text("claim"),
}, t => [uniqueIndex("mcp_delivery_identity").on(t.subscriptionId, t.eventId), index("mcp_deliveries_due").on(t.state, t.nextAttemptAt)]);
