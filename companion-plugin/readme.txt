=== WooAgent Companion ===
Contributors: automattic
Tags: woocommerce, ai, agents, mcp, abilities
Requires at least: 6.7
Tested up to: 6.8
Requires PHP: 7.4
Stable tag: 0.4.1
License: Apache-2.0

Registers the WooAgent OS ability surface on a WooCommerce store. Paired with the WooAgent OS daemon running on the operator's machine.

== Description ==

The WooAgent Companion plugin exposes WooCommerce store operations as WordPress Abilities. Once installed, the WooAgent OS daemon can discover and invoke these abilities through any MCP client — enabling a fleet of AI agents to read products, draft content, inspect orders, and support customers.

This plugin is part of the WooAgent OS project. It runs on the store side; the agent fleet and UI run on the operator's machine.

== v0.1 Ability Surface ==

Products:
* wooagent-products/list  (v0.2: accepts orderby + order; returns total_sales)
* wooagent-products/get
* wooagent-products/update
* wooagent-products/list-categories  (new in v0.2)

Orders:
* wooagent-orders/list
* wooagent-orders/get
* wooagent-orders/add-note

Customers:
* wooagent-customers/get

Device pairing (stub — shipping in v0.2):
* wooagent-device-pair/request
* wooagent-device-pair/confirm
* wooagent-device-pair/revoke

Until pairing ships, authenticate with a WordPress Application Password (Users → Profile → Application Passwords).

== Changelog ==

= 0.4.1 =
* Removes internal diagnostic REST endpoints, including the source-inspection endpoint.
* Restricts device bearer authentication to REST requests and valid approving WordPress users. Legacy pairings without an approver identity must pair again.
* Adds pairing input bounds, fixed-window rate limits, and non-destructive retry behavior.
* Hardens updater asset selection and plugin packaging, and adds repeatable security/release verification.

= 0.4.0 =
* Adds self-update via plugin-update-checker pointed at github.com/Automattic/wooagent-os releases. Installed copies will now check for new releases automatically and surface them in WP Admin → Plugins.
* Fixes the Plugin URI to point at github.com/Automattic/wooagent-os (the canonical public repo).

= 0.3.0 =
* (No public changelog entry — version was bumped during internal development; superseded by 0.4.0.)

= 0.2.0 =
* wooagent-products/list: new orderby (menu_order | date | date_modified | total_sales | title) + order (asc | desc) input args.
* wooagent-products/list: summary now includes total_sales.
* New ability wooagent-products/list-categories: returns the store's product category taxonomy as {id, name, slug, parent_id, count}.

= 0.1.0 =
* Initial release. Product, order, and customer CRUD abilities. Device-pair scaffold.
