CREATE TABLE `preferences` (
	`owner_id` text PRIMARY KEY NOT NULL,
	`share_selected_context` integer DEFAULT true NOT NULL
);
