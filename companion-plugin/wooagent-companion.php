<?php
/**
 * Plugin Name:       WooAgent Companion
 * Plugin URI:        https://github.com/Automattic/wooagent-os
 * Description:       Registers the WooAgent OS ability surface on a WooCommerce store. Paired with the WooAgent OS daemon running on the operator's machine.
 * Version:           0.4.0
 * Requires at least: 6.7
 * Requires PHP:      7.4
 * Author:            Elizabeth Pizzuti
 * License:           Apache-2.0
 * Text Domain:       wooagent-companion
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

define( 'WOOAGENT_COMPANION_VERSION', '0.4.0' );
define( 'WOOAGENT_COMPANION_PATH', plugin_dir_path( __FILE__ ) );

require_once WOOAGENT_COMPANION_PATH . 'includes/abilities-products.php';
require_once WOOAGENT_COMPANION_PATH . 'includes/abilities-orders.php';
require_once WOOAGENT_COMPANION_PATH . 'includes/abilities-customers.php';
require_once WOOAGENT_COMPANION_PATH . 'includes/pair-rest.php';
require_once WOOAGENT_COMPANION_PATH . 'includes/auth-bridge.php';
require_once WOOAGENT_COMPANION_PATH . 'includes/admin-pair-screen.php';
require_once WOOAGENT_COMPANION_PATH . 'includes/update-checker.php';

add_action( 'wp_abilities_api_categories_init', 'wooagent_companion_register_categories' );
add_action( 'wp_abilities_api_init', 'wooagent_companion_register_abilities' );

function wooagent_companion_register_categories(): void {
	if ( ! function_exists( 'wp_register_ability_category' ) ) {
		return;
	}

	$cats = array(
		'wooagent-products'     => array( 'label' => __( 'WooAgent · Products', 'wooagent-companion' ),    'description' => __( 'Read and update WooCommerce products.', 'wooagent-companion' ) ),
		'wooagent-orders'       => array( 'label' => __( 'WooAgent · Orders', 'wooagent-companion' ),      'description' => __( 'Inspect WooCommerce orders and attach notes.', 'wooagent-companion' ) ),
		'wooagent-customers'    => array( 'label' => __( 'WooAgent · Customers', 'wooagent-companion' ),   'description' => __( 'Read WooCommerce customer records.', 'wooagent-companion' ) ),
	);

	foreach ( $cats as $slug => $args ) {
		wp_register_ability_category( $slug, $args );
	}
}

function wooagent_companion_register_abilities(): void {
	wooagent_companion_register_product_abilities();
	wooagent_companion_register_order_abilities();
	wooagent_companion_register_customer_abilities();
}

add_action( 'admin_notices', 'wooagent_companion_dependency_notice' );

function wooagent_companion_dependency_notice(): void {
	if ( ! current_user_can( 'activate_plugins' ) ) {
		return;
	}

	$missing = array();

	if ( ! function_exists( 'wp_register_ability' ) ) {
		$missing[] = 'WordPress Abilities API';
	}
	if ( ! class_exists( 'WooCommerce' ) ) {
		$missing[] = 'WooCommerce';
	}

	if ( empty( $missing ) ) {
		return;
	}

	printf(
		'<div class="notice notice-error"><p><strong>WooAgent Companion</strong> requires: %s.</p></div>',
		esc_html( implode( ', ', $missing ) )
	);
}
