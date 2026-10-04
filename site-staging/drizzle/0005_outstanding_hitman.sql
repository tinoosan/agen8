CREATE TABLE `__new_mcp_event_outbox` (
	`sequence` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`id` text NOT NULL,
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
INSERT INTO `__new_mcp_event_outbox`("sequence", "id", "owner_id", "project_id", "node_id", "name", "version", "status", "previous_status", "transition", "summary", "occurred_at") SELECT rowid, "id", "owner_id", "project_id", "node_id", "name", "version", "status", "previous_status", "transition", "summary", "occurred_at" FROM `mcp_event_outbox`;--> statement-breakpoint
DROP TABLE `mcp_event_outbox`;--> statement-breakpoint
ALTER TABLE `__new_mcp_event_outbox` RENAME TO `mcp_event_outbox`;--> statement-breakpoint
CREATE UNIQUE INDEX `mcp_event_outbox_id_unique` ON `mcp_event_outbox` (`id`);--> statement-breakpoint
CREATE INDEX `mcp_events_owner_project` ON `mcp_event_outbox` (`owner_id`,`project_id`);