CREATE TABLE `mcp_deliveries` (
	`subscription_id` text NOT NULL,
	`event_id` text NOT NULL,
	`state` text NOT NULL,
	`attempts` integer DEFAULT 0 NOT NULL,
	`next_attempt_at` integer NOT NULL,
	`lease_until` integer DEFAULT 0 NOT NULL,
	`claim` text
);
--> statement-breakpoint
CREATE UNIQUE INDEX `mcp_delivery_identity` ON `mcp_deliveries` (`subscription_id`,`event_id`);--> statement-breakpoint
CREATE INDEX `mcp_deliveries_due` ON `mcp_deliveries` (`state`,`next_attempt_at`);--> statement-breakpoint
CREATE TABLE `mcp_event_outbox` (
	`id` text PRIMARY KEY NOT NULL,
	`owner_id` text NOT NULL,
	`project_id` text NOT NULL,
	`node_id` text NOT NULL,
	`name` text NOT NULL,
	`version` integer NOT NULL,
	`status` text NOT NULL,
	`previous_status` text,
	`transition` text NOT NULL,
	`summary` text NOT NULL,
	`occurred_at` text NOT NULL
);
--> statement-breakpoint
CREATE INDEX `mcp_events_owner_project` ON `mcp_event_outbox` (`owner_id`,`project_id`);--> statement-breakpoint
CREATE TABLE `mcp_subscriptions` (
	`id` text PRIMARY KEY NOT NULL,
	`owner_id` text NOT NULL,
	`name` text NOT NULL,
	`arguments` text NOT NULL,
	`project_id` text NOT NULL,
	`node_id` text,
	`status` text,
	`url` text NOT NULL,
	`secret` text NOT NULL,
	`secret_hash` text NOT NULL,
	`previous_secret` text,
	`rotation_until` integer,
	`verified_at` integer NOT NULL,
	`expires_at` integer NOT NULL,
	`start_sequence` integer NOT NULL,
	`active` integer NOT NULL
);
--> statement-breakpoint
CREATE INDEX `mcp_subscriptions_owner` ON `mcp_subscriptions` (`owner_id`);--> statement-breakpoint
CREATE INDEX `mcp_subscriptions_callback` ON `mcp_subscriptions` (`owner_id`,`url`);