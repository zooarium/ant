-- Create "ant_storefront" table
CREATE TABLE `ant_storefront` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL, `app_id` integer NOT NULL, `division_id` integer NOT NULL, `hero_image` text NULL, `assessments` json NOT NULL, `gallery` json NOT NULL, `food_tags` json NOT NULL, `status` integer NOT NULL DEFAULT (1));
-- Create index "storefront_app_id_division_id" to table: "ant_storefront"
CREATE UNIQUE INDEX `storefront_app_id_division_id` ON `ant_storefront` (`app_id`, `division_id`);
