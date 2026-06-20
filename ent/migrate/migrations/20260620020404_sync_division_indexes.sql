-- Disable the enforcement of foreign-keys constraints
PRAGMA foreign_keys = off;
-- Create "new_ant_attribute" table
CREATE TABLE `new_ant_attribute` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL, `app_id` integer NOT NULL, `user_id` integer NOT NULL, `division_id` integer NOT NULL, `name` text NOT NULL, `status` integer NOT NULL DEFAULT (1));
-- Copy rows from old table "ant_attribute" to new temporary table "new_ant_attribute"
INSERT INTO `new_ant_attribute` (`id`, `created_at`, `updated_at`, `app_id`, `user_id`, `name`, `status`) SELECT `id`, `created_at`, `updated_at`, `app_id`, `user_id`, `name`, `status` FROM `ant_attribute`;
-- Drop "ant_attribute" table after copying rows
DROP TABLE `ant_attribute`;
-- Rename temporary table "new_ant_attribute" to "ant_attribute"
ALTER TABLE `new_ant_attribute` RENAME TO `ant_attribute`;
-- Create index "attribute_app_id_division_id" to table: "ant_attribute"
CREATE INDEX `attribute_app_id_division_id` ON `ant_attribute` (`app_id`, `division_id`);
-- Create index "ordergroup_app_id_division_id_status" to table: "ant_order_group"
CREATE INDEX `ordergroup_app_id_division_id_status` ON `ant_order_group` (`app_id`, `division_id`, `status`);
-- Create index "order_app_id_division_id_status" to table: "ant_order"
CREATE INDEX `order_app_id_division_id_status` ON `ant_order` (`app_id`, `division_id`, `status`);
-- Create "new_ant_product" table
CREATE TABLE `new_ant_product` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL, `app_id` integer NOT NULL, `user_id` integer NOT NULL, `division_id` integer NOT NULL, `name` text NOT NULL, `price` real NOT NULL DEFAULT (0), `status` integer NOT NULL DEFAULT (1), `category_id` integer NULL, CONSTRAINT `ant_product_ant_category_products` FOREIGN KEY (`category_id`) REFERENCES `ant_category` (`id`) ON DELETE SET NULL);
-- Copy rows from old table "ant_product" to new temporary table "new_ant_product"
INSERT INTO `new_ant_product` (`id`, `created_at`, `updated_at`, `app_id`, `user_id`, `name`, `price`, `status`, `category_id`) SELECT `id`, `created_at`, `updated_at`, `app_id`, `user_id`, `name`, `price`, `status`, `category_id` FROM `ant_product`;
-- Drop "ant_product" table after copying rows
DROP TABLE `ant_product`;
-- Rename temporary table "new_ant_product" to "ant_product"
ALTER TABLE `new_ant_product` RENAME TO `ant_product`;
-- Create index "product_app_id_division_id" to table: "ant_product"
CREATE INDEX `product_app_id_division_id` ON `ant_product` (`app_id`, `division_id`);
-- Create index "product_app_id_division_id_category_id" to table: "ant_product"
CREATE INDEX `product_app_id_division_id_category_id` ON `ant_product` (`app_id`, `division_id`, `category_id`);
-- Create "new_ant_category" table
CREATE TABLE `new_ant_category` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL, `app_id` integer NOT NULL, `division_id` integer NOT NULL, `name` text NOT NULL, `path` text NOT NULL, `depth` integer NOT NULL DEFAULT (0), `status` integer NOT NULL DEFAULT (1), `parent_id` integer NULL, CONSTRAINT `ant_category_ant_category_children` FOREIGN KEY (`parent_id`) REFERENCES `ant_category` (`id`) ON DELETE SET NULL);
-- Copy rows from old table "ant_category" to new temporary table "new_ant_category"
INSERT INTO `new_ant_category` (`id`, `created_at`, `updated_at`, `app_id`, `name`, `path`, `depth`, `status`, `parent_id`) SELECT `id`, `created_at`, `updated_at`, `app_id`, `name`, `path`, `depth`, `status`, `parent_id` FROM `ant_category`;
-- Drop "ant_category" table after copying rows
DROP TABLE `ant_category`;
-- Rename temporary table "new_ant_category" to "ant_category"
ALTER TABLE `new_ant_category` RENAME TO `ant_category`;
-- Create index "category_path" to table: "ant_category"
CREATE INDEX `category_path` ON `ant_category` (`path`);
-- Create index "category_app_id_division_id_parent_id" to table: "ant_category"
CREATE INDEX `category_app_id_division_id_parent_id` ON `ant_category` (`app_id`, `division_id`, `parent_id`);
-- Enable back the enforcement of foreign-keys constraints
PRAGMA foreign_keys = on;
