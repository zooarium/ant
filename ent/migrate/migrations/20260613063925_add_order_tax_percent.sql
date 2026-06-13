-- Disable the enforcement of foreign-keys constraints
PRAGMA foreign_keys = off;
-- Create "new_ant_order" table
CREATE TABLE `new_ant_order` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL, `app_id` integer NOT NULL, `user_id` integer NOT NULL, `division_id` integer NOT NULL, `customer_name` text NOT NULL, `customer_contact` text NOT NULL, `ordered_at` datetime NOT NULL, `status` integer NOT NULL DEFAULT (1), `tax_percent` real NOT NULL DEFAULT (0), `group_id` integer NOT NULL, CONSTRAINT `ant_order_ant_order_group_orders` FOREIGN KEY (`group_id`) REFERENCES `ant_order_group` (`id`) ON DELETE NO ACTION);
-- Copy rows from old table "ant_order" to new temporary table "new_ant_order"
INSERT INTO `new_ant_order` (`id`, `created_at`, `updated_at`, `app_id`, `user_id`, `division_id`, `customer_name`, `customer_contact`, `ordered_at`, `status`, `group_id`) SELECT `id`, `created_at`, `updated_at`, `app_id`, `user_id`, `division_id`, `customer_name`, `customer_contact`, `ordered_at`, `status`, `group_id` FROM `ant_order`;
-- Drop "ant_order" table after copying rows
DROP TABLE `ant_order`;
-- Rename temporary table "new_ant_order" to "ant_order"
ALTER TABLE `new_ant_order` RENAME TO `ant_order`;
-- Create index "order_app_id_status" to table: "ant_order"
CREATE INDEX `order_app_id_status` ON `ant_order` (`app_id`, `status`);
-- Create index "order_group_id" to table: "ant_order"
CREATE INDEX `order_group_id` ON `ant_order` (`group_id`);
-- Enable back the enforcement of foreign-keys constraints
PRAGMA foreign_keys = on;
