CREATE TABLE `mcp_dispatch_grants` (
	`id` text PRIMARY KEY NOT NULL,
	`owner_id` text NOT NULL,
	`active` integer NOT NULL,
	`expires_at` integer NOT NULL
);
--> statement-breakpoint
CREATE UNIQUE INDEX `mcp_dispatch_grant_owner` ON `mcp_dispatch_grants` (`owner_id`);