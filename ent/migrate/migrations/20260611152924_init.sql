-- Create "ant_attribute" table
CREATE TABLE `ant_attribute` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL, `app_id` integer NOT NULL, `user_id` integer NOT NULL, `name` text NOT NULL, `status` integer NOT NULL DEFAULT (1));
-- Create index "attribute_app_id" to table: "ant_attribute"
CREATE INDEX `attribute_app_id` ON `ant_attribute` (`app_id`);
-- Create "ant_attribute_option" table
CREATE TABLE `ant_attribute_option` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL, `value` text NOT NULL, `attribute_id` integer NOT NULL, CONSTRAINT `ant_attribute_option_ant_attribute_options` FOREIGN KEY (`attribute_id`) REFERENCES `ant_attribute` (`id`) ON DELETE NO ACTION);
-- Create index "attributeoption_attribute_id" to table: "ant_attribute_option"
CREATE INDEX `attributeoption_attribute_id` ON `ant_attribute_option` (`attribute_id`);
-- Create "ant_order" table
CREATE TABLE `ant_order` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL, `app_id` integer NOT NULL, `user_id` integer NOT NULL, `division_id` integer NOT NULL, `customer_name` text NOT NULL, `customer_contact` text NOT NULL, `ordered_at` datetime NOT NULL, `status` integer NOT NULL DEFAULT (1), `group_id` integer NOT NULL, CONSTRAINT `ant_order_ant_order_group_orders` FOREIGN KEY (`group_id`) REFERENCES `ant_order_group` (`id`) ON DELETE NO ACTION);
-- Create index "order_app_id_status" to table: "ant_order"
CREATE INDEX `order_app_id_status` ON `ant_order` (`app_id`, `status`);
-- Create index "order_group_id" to table: "ant_order"
CREATE INDEX `order_group_id` ON `ant_order` (`group_id`);
-- Create "ant_order_group" table
CREATE TABLE `ant_order_group` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL, `app_id` integer NOT NULL, `user_id` integer NOT NULL, `division_id` integer NOT NULL, `token` text NOT NULL, `label` text NULL, `status` integer NOT NULL DEFAULT (1));
-- Create index "ordergroup_app_id_token" to table: "ant_order_group"
CREATE UNIQUE INDEX `ordergroup_app_id_token` ON `ant_order_group` (`app_id`, `token`);
-- Create index "ordergroup_app_id_status" to table: "ant_order_group"
CREATE INDEX `ordergroup_app_id_status` ON `ant_order_group` (`app_id`, `status`);
-- Create "ant_order_product" table
CREATE TABLE `ant_order_product` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL, `product_id` integer NOT NULL, `product_name` text NOT NULL, `price` real NOT NULL, `quantity` integer NOT NULL, `attributes` json NOT NULL, `order_id` integer NOT NULL, CONSTRAINT `ant_order_product_ant_order_products` FOREIGN KEY (`order_id`) REFERENCES `ant_order` (`id`) ON DELETE NO ACTION);
-- Create index "orderproduct_order_id" to table: "ant_order_product"
CREATE INDEX `orderproduct_order_id` ON `ant_order_product` (`order_id`);
-- Create index "orderproduct_product_id" to table: "ant_order_product"
CREATE INDEX `orderproduct_product_id` ON `ant_order_product` (`product_id`);
-- Create "ant_product" table
CREATE TABLE `ant_product` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL, `app_id` integer NOT NULL, `user_id` integer NOT NULL, `name` text NOT NULL, `price` real NOT NULL DEFAULT (0), `status` integer NOT NULL DEFAULT (1));
-- Create index "product_app_id" to table: "ant_product"
CREATE INDEX `product_app_id` ON `ant_product` (`app_id`);
-- Create "ant_product_attribute" table
CREATE TABLE `ant_product_attribute` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL, `is_mandatory` bool NOT NULL DEFAULT (false), `options` json NOT NULL, `attribute_id` integer NOT NULL, `product_id` integer NOT NULL, CONSTRAINT `ant_product_attribute_ant_attribute_product_attributes` FOREIGN KEY (`attribute_id`) REFERENCES `ant_attribute` (`id`) ON DELETE NO ACTION, CONSTRAINT `ant_product_attribute_ant_product_attributes` FOREIGN KEY (`product_id`) REFERENCES `ant_product` (`id`) ON DELETE NO ACTION);
-- Create index "productattribute_product_id_attribute_id" to table: "ant_product_attribute"
CREATE UNIQUE INDEX `productattribute_product_id_attribute_id` ON `ant_product_attribute` (`product_id`, `attribute_id`);
-- Create index "productattribute_attribute_id" to table: "ant_product_attribute"
CREATE INDEX `productattribute_attribute_id` ON `ant_product_attribute` (`attribute_id`);
