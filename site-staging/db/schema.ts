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
