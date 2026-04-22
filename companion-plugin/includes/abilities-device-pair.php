<?php
/**
 * Device pairing abilities for WooAgent — v0.1 scaffold.
 *
 * Ships as registered stubs so the ability surface is stable. Full token flow lands in v0.2.
 * Until then, operators authenticate with a WordPress Application Password.
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

function wooagent_companion_register_device_pair_abilities(): void {
	$not_implemented = static function () {
		return new WP_Error(
			'wooagent_device_pair_not_implemented',
			__( 'Device pairing ships in WooAgent Companion v0.2. Use a WordPress Application Password to authenticate the daemon for now.', 'wooagent-companion' ),
			array( 'status' => 501 )
		);
	};

	$empty_object_schema = array(
		'type'                 => 'object',
		'properties'           => new stdClass(),
		'additionalProperties' => false,
	);

	wp_register_ability(
		'wooagent-device-pair/request',
		array(
			'label'               => __( 'Request device pairing code', 'wooagent-companion' ),
			'description'         => __( 'Daemon-initiated: requests a short pairing code the operator can enter in wp-admin. v0.2.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'device_name' => array( 'type' => 'string', 'description' => 'Human-readable name for the device, e.g., "mystore-laptop".' ),
				),
				'additionalProperties' => false,
			),
			'output_schema'       => $empty_object_schema,
			'execute_callback'    => $not_implemented,
			'permission_callback' => '__return_true',
			'category'            => 'wooagent-device-pair',
			'meta'                => array( 'show_in_rest' => true, 'mcp' => array( 'public' => true ), 'status' => 'stub' ),
		)
	);

	wp_register_ability(
		'wooagent-device-pair/confirm',
		array(
			'label'               => __( 'Confirm device pairing', 'wooagent-companion' ),
			'description'         => __( 'Operator-facing: approves a pending pairing code and mints a scoped device token. v0.2.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'code' => array( 'type' => 'string' ),
				),
				'required'   => array( 'code' ),
				'additionalProperties' => false,
			),
			'output_schema'       => $empty_object_schema,
			'execute_callback'    => $not_implemented,
			'permission_callback' => static function () {
				return current_user_can( 'manage_options' );
			},
			'category'            => 'wooagent-device-pair',
			'meta'                => array( 'show_in_rest' => true, 'mcp' => array( 'public' => true ), 'status' => 'stub' ),
		)
	);

	wp_register_ability(
		'wooagent-device-pair/revoke',
		array(
			'label'               => __( 'Revoke paired device', 'wooagent-companion' ),
			'description'         => __( 'Revoke a previously-paired device token. v0.2.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'device_id' => array( 'type' => 'string' ),
				),
				'required'   => array( 'device_id' ),
				'additionalProperties' => false,
			),
			'output_schema'       => $empty_object_schema,
			'execute_callback'    => $not_implemented,
			'permission_callback' => static function () {
				return current_user_can( 'manage_options' );
			},
			'category'            => 'wooagent-device-pair',
			'meta'                => array( 'show_in_rest' => true, 'mcp' => array( 'public' => true ), 'status' => 'stub' ),
		)
	);
}
