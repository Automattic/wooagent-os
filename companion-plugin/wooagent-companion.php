<?php
/**
 * Plugin Name:       WooAgent Companion
 * Plugin URI:        https://github.com/automattic/wooagent-os
 * Description:       Registers the WooAgent OS ability surface on a WooCommerce store. Paired with the WooAgent OS daemon running on the operator's machine.
 * Version:           0.1.0
 * Requires at least: 6.7
 * Requires PHP:      7.4
 * Author:            Elizabeth Pizzuti
 * License:           Apache-2.0
 * Text Domain:       wooagent-companion
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

define( 'WOOAGENT_COMPANION_VERSION', '0.1.0' );
define( 'WOOAGENT_COMPANION_PATH', plugin_dir_path( __FILE__ ) );

require_once WOOAGENT_COMPANION_PATH . 'includes/abilities-products.php';
require_once WOOAGENT_COMPANION_PATH . 'includes/abilities-orders.php';
require_once WOOAGENT_COMPANION_PATH . 'includes/abilities-customers.php';
require_once WOOAGENT_COMPANION_PATH . 'includes/abilities-device-pair.php';
require_once WOOAGENT_COMPANION_PATH . 'includes/admin-pair-screen.php';

add_action( 'wp_abilities_api_categories_init', 'wooagent_companion_register_categories' );
add_action( 'wp_abilities_api_init', 'wooagent_companion_register_abilities' );

function wooagent_companion_register_categories(): void {
	if ( ! function_exists( 'wp_register_ability_category' ) ) {
		return;
	}

	$results = array();
	$cats = array(
		'wooagent-products'     => array( 'label' => __( 'WooAgent · Products', 'wooagent-companion' ),    'description' => __( 'Read and update WooCommerce products.', 'wooagent-companion' ) ),
		'wooagent-orders'       => array( 'label' => __( 'WooAgent · Orders', 'wooagent-companion' ),      'description' => __( 'Inspect WooCommerce orders and attach notes.', 'wooagent-companion' ) ),
		'wooagent-customers'    => array( 'label' => __( 'WooAgent · Customers', 'wooagent-companion' ),   'description' => __( 'Read WooCommerce customer records.', 'wooagent-companion' ) ),
		'wooagent-device-pair'  => array( 'label' => __( 'WooAgent · Device pairing', 'wooagent-companion' ), 'description' => __( 'Daemon device pairing flow (scaffolded; ships in v0.2).', 'wooagent-companion' ) ),
	);

	foreach ( $cats as $slug => $args ) {
		$return = wp_register_ability_category( $slug, $args );
		$exists = wp_has_ability_category( $slug );
		$results[ $slug ] = array(
			'return_type' => is_null( $return ) ? 'null' : ( is_wp_error( $return ) ? 'WP_Error' : get_class( $return ) ),
			'exists_after' => $exists,
		);
	}

	update_option( 'wooagent_companion_category_results', $results );
}

function wooagent_companion_register_abilities(): void {
	$results = array();
	$names = array(
		'wooagent-products/list',
		'wooagent-products/get',
		'wooagent-products/update',
		'wooagent-orders/list',
		'wooagent-orders/get',
		'wooagent-orders/add-note',
		'wooagent-customers/get',
		'wooagent-device-pair/request',
		'wooagent-device-pair/confirm',
		'wooagent-device-pair/revoke',
	);

	// Verify categories exist before we register abilities referencing them.
	$category_check = array(
		'wooagent-products'    => wp_has_ability_category( 'wooagent-products' ),
		'wooagent-orders'      => wp_has_ability_category( 'wooagent-orders' ),
		'wooagent-customers'   => wp_has_ability_category( 'wooagent-customers' ),
		'wooagent-device-pair' => wp_has_ability_category( 'wooagent-device-pair' ),
	);
	update_option( 'wooagent_companion_category_check_at_register', $category_check );

	// Capture PHP errors/notices fired during registration.
	$captured_errors = array();
	$prev_handler = set_error_handler( static function ( $errno, $errstr, $errfile = '', $errline = 0 ) use ( &$captured_errors ) {
		$captured_errors[] = array(
			'errno'   => $errno,
			'message' => $errstr,
			'file'    => basename( (string) $errfile ),
			'line'    => $errline,
		);
		return false; // let PHP default handling continue
	} );

	wooagent_companion_register_product_abilities();
	wooagent_companion_register_order_abilities();
	wooagent_companion_register_customer_abilities();
	wooagent_companion_register_device_pair_abilities();

	set_error_handler( $prev_handler );

	foreach ( $names as $name ) {
		$ability           = wp_get_ability( $name );
		$results[ $name ] = $ability ? 'ok' : 'missing';
	}

	update_option( 'wooagent_companion_last_register', time() );
	update_option( 'wooagent_companion_register_results', $results );
	update_option( 'wooagent_companion_register_errors', $captured_errors );
}

function wooagent_companion_describe_value( $value ): string {
	if ( is_wp_error( $value ) ) {
		$data = $value->get_error_data();
		return sprintf( 'WP_Error(code=%s, message=%s, data=%s)', $value->get_error_code(), $value->get_error_message(), wp_json_encode( $data ) );
	}
	if ( is_bool( $value ) ) {
		return $value ? 'true' : 'false';
	}
	if ( is_null( $value ) ) {
		return 'null';
	}
	if ( is_object( $value ) ) {
		return 'object(' . get_class( $value ) . ')';
	}
	if ( is_array( $value ) ) {
		return 'array(' . count( $value ) . ')';
	}
	return gettype( $value ) . ':' . (string) $value;
}

add_action( 'rest_api_init', 'wooagent_companion_register_debug_route' );

function wooagent_companion_register_debug_route(): void {
	register_rest_route(
		'wooagent-companion/v1',
		'/source',
		array(
			'methods'             => 'GET',
			'permission_callback' => static function () {
				return current_user_can( 'manage_options' );
			},
			'callback'            => static function ( $request ) {
				$target = $request->get_param( 'class' ) ?: 'wp_register_ability';
				if ( $target === 'wp_register_ability' || $target === 'wp_register_ability_category' ) {
					if ( ! function_exists( $target ) ) {
						return new WP_Error( 'no_function', $target . ' does not exist' );
					}
					$reflect = new ReflectionFunction( $target );
				} else {
					if ( ! class_exists( $target ) ) {
						return new WP_Error( 'no_class', $target . ' does not exist' );
					}
					$reflect = new ReflectionClass( $target );
				}
				$file = $reflect->getFileName();
				if ( ! $file || ! is_readable( $file ) ) {
					return new WP_Error( 'unreadable', 'Source not readable', array( 'file' => $file ) );
				}
				$source = file_get_contents( $file );
				return array(
					'file'   => $file,
					'length' => strlen( $source ),
					'source' => $source,
				);
			},
		)
	);

	register_rest_route(
		'wooagent-companion/v1',
		'/selftest',
		array(
			'methods'             => 'GET',
			'permission_callback' => '__return_true',
			'callback'            => static function () {
				$all_abilities = array();
				if ( function_exists( 'wp_get_abilities' ) ) {
					foreach ( wp_get_abilities() as $a ) {
						if ( is_object( $a ) && method_exists( $a, 'get_name' ) ) {
							$all_abilities[] = $a->get_name();
						} elseif ( is_array( $a ) && isset( $a['name'] ) ) {
							$all_abilities[] = $a['name'];
						}
					}
				}

				// Reflect on wp_register_ability to understand its signature.
				$signature = null;
				if ( function_exists( 'wp_register_ability' ) ) {
					$reflect = new ReflectionFunction( 'wp_register_ability' );
					$params = array();
					foreach ( $reflect->getParameters() as $p ) {
						$type = $p->getType();
						$params[] = array(
							'name' => $p->getName(),
							'type' => $type ? (string) $type : null,
							'optional' => $p->isOptional(),
						);
					}
					$signature = array(
						'file'   => $reflect->getFileName(),
						'line'   => $reflect->getStartLine(),
						'params' => $params,
					);
				}

				return array(
					'plugin_loaded'              => true,
					'version'                    => defined( 'WOOAGENT_COMPANION_VERSION' ) ? WOOAGENT_COMPANION_VERSION : null,
					'wp_register_ability_exists' => function_exists( 'wp_register_ability' ),
					'wp_get_ability_exists'      => function_exists( 'wp_get_ability' ),
					'wp_get_abilities_exists'    => function_exists( 'wp_get_abilities' ),
					'woocommerce_active'         => class_exists( 'WooCommerce' ),
					'abilities_api_init_did'     => did_action( 'abilities_api_init' ),
					'init_did'                   => did_action( 'init' ),
					'registered_marker'          => get_option( 'wooagent_companion_last_register', 0 ),
					'register_results'           => get_option( 'wooagent_companion_register_results', array() ),
					'register_errors'            => get_option( 'wooagent_companion_register_errors', array() ),
					'category_results'           => get_option( 'wooagent_companion_category_results', array() ),
					'category_check_at_register' => get_option( 'wooagent_companion_category_check_at_register', array() ),
					'categories_init_did'        => did_action( 'wp_abilities_api_categories_init' ),
					'abilities_init_did'         => did_action( 'wp_abilities_api_init' ),
					'runtime_all_abilities'      => $all_abilities,
					'runtime_all_abilities_count' => count( $all_abilities ),
				);
			},
		)
	);
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
