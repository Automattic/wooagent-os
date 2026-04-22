=== WooAgent Companion ===
Contributors: elizaan36
Tags: woocommerce, ai, agents, mcp, abilities
Requires at least: 6.7
Tested up to: 6.8
Requires PHP: 7.4
Stable tag: 0.1.0
License: Apache-2.0

Registers the WooAgent OS ability surface on a WooCommerce store. Paired with the WooAgent OS daemon running on the operator's machine.

== Description ==

The WooAgent Companion plugin exposes WooCommerce store operations as WordPress Abilities. Once installed, the WooAgent OS daemon can discover and invoke these abilities through any MCP client — enabling a fleet of AI agents to read products, draft content, inspect orders, and support customers.

This plugin is part of the WooAgent OS project. It runs on the store side; the agent fleet and UI run on the operator's machine.

== v0.1 Ability Surface ==

Products:
* wooagent-products/list
* wooagent-products/get
* wooagent-products/update

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

= 0.1.0 =
* Initial release. Product, order, and customer CRUD abilities. Device-pair scaffold.
