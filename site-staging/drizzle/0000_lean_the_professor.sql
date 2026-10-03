CREATE TABLE `activity` (
	`id` text PRIMARY KEY NOT NULL,
	`project_id` text NOT NULL,
	`node_id` text,
	`agent_id` text,
	`event` text NOT NULL,
	`summary` text NOT NULL,
	`created_at` text NOT NULL,
	FOREIGN KEY (`project_id`) REFERENCES `projects`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE INDEX `activity_project_created` ON `activity` (`project_id`,`created_at`);--> statement-breakpoint
CREATE TABLE `agents` (
	`id` text PRIMARY KEY NOT NULL,
	`project_id` text NOT NULL,
	`session_ref` text NOT NULL,
	`name` text NOT NULL,
	`focus` text DEFAULT '' NOT NULL,
	`last_seen_at` text NOT NULL,
	FOREIGN KEY (`project_id`) REFERENCES `projects`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE UNIQUE INDEX `agents_project_session` ON `agents` (`project_id`,`session_ref`);--> statement-breakpoint
CREATE TABLE `links` (
	`id` text PRIMARY KEY NOT NULL,
	`project_id` text NOT NULL,
	`source_id` text NOT NULL,
	`target_id` text NOT NULL,
	`relation` text NOT NULL,
	`rationale` text DEFAULT '' NOT NULL,
	`created_at` text NOT NULL,
	FOREIGN KEY (`project_id`) REFERENCES `projects`(`id`) ON UPDATE no action ON DELETE no action,
	FOREIGN KEY (`source_id`) REFERENCES `nodes`(`id`) ON UPDATE no action ON DELETE no action,
	FOREIGN KEY (`target_id`) REFERENCES `nodes`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE UNIQUE INDEX `links_identity` ON `links` (`project_id`,`source_id`,`target_id`,`relation`);--> statement-breakpoint
CREATE TABLE `nodes` (
	`id` text PRIMARY KEY NOT NULL,
	`project_id` text NOT NULL,
	`kind` text NOT NULL,
	`title` text NOT NULL,
	`summary` text DEFAULT '' NOT NULL,
	`body` text DEFAULT '' NOT NULL,
	`status` text DEFAULT 'pending' NOT NULL,
	`agent_id` text,
	`blocker` text DEFAULT '' NOT NULL,
	`outcome` text DEFAULT '' NOT NULL,
	`artifacts` text DEFAULT '[]' NOT NULL,
	`version` integer DEFAULT 1 NOT NULL,
	`mutation_id` text DEFAULT '' NOT NULL,
	`created_at` text NOT NULL,
	`updated_at` text NOT NULL,
	FOREIGN KEY (`project_id`) REFERENCES `projects`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE INDEX `nodes_project_updated` ON `nodes` (`project_id`,`updated_at`);--> statement-breakpoint
CREATE TABLE `projects` (
	`id` text PRIMARY KEY NOT NULL,
	`owner_id` text NOT NULL,
	`title` text NOT NULL,
	`objective` text DEFAULT '' NOT NULL,
	`status` text DEFAULT 'open' NOT NULL,
	`created_at` text NOT NULL,
	`updated_at` text NOT NULL
);
--> statement-breakpoint
CREATE INDEX `projects_owner` ON `projects` (`owner_id`,`updated_at`);