<?php
/**
 * Customer abilities for WooAgent.
 *
 * wooagent-customers/get
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

function wooagent_companion_register_customer_abilities(): void {
	wp_register_ability(
		'wooagent-customers/get',
		array(
			'label'               => __( 'Get customer', 'wooagent-companion' ),
			'description'         => __( 'Read a single WooCommerce customer including lifetime spend and order count.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'id' => array( 'type' => 'integer', 'minimum' => 1 ),
				),
				'required'   => array( 'id' ),
				'additionalProperties' => false,
			),
			'output_schema'       => array(
				'type'       => 'object',
				'properties' => array(
					'id'             => array( 'type' => 'integer' ),
					'email'          => array( 'type' => 'string' ),
					'first_name'     => array( 'type' => 'string' ),
					'last_name'      => array( 'type' => 'string' ),
					'username'       => array( 'type' => 'string' ),
					'date_created'   => array( 'type' => 'string' ),
					'order_count'    => array( 'type' => 'integer' ),
					'total_spent'    => array( 'type' => 'string' ),
					'is_paying_customer' => array( 'type' => 'boolean' ),
				),
				'required'   => array( 'id', 'email' ),
			),
			'category'            => 'wooagent-customers',
			'execute_callback'    => 'wooagent_customers_get_execute',
			'permission_callback' => 'wooagent_customers_read_permission',
			'meta'                => array(
				'show_in_rest' => true,
				'mcp'          => array( 'public' => true ),
				'annotations'  => array( 'readonly' => true, 'idempotent' => true ),
			),
		)
	);
}

function wooagent_customers_read_permission(): bool {
	return current_user_can( 'list_users' ) || current_user_can( 'edit_shop_orders' );
}

function wooagent_customers_get_execute( array $args ) {
	if ( ! class_exists( 'WC_Customer' ) ) {
		return new WP_Error( 'wooagent_woocommerce_missing', __( 'WooCommerce is not active.', 'wooagent-companion' ) );
	}

	try {
		$customer = new WC_Customer( (int) $args['id'] );
	} catch ( Exception $e ) {
		return new WP_Error( 'wooagent_customer_not_found', __( 'Customer not found.', 'wooagent-companion' ), array( 'status' => 404 ) );
	}

	if ( ! $customer->get_id() ) {
		return new WP_Error( 'wooagent_customer_not_found', __( 'Customer not found.', 'wooagent-companion' ), array( 'status' => 404 ) );
	}

	return array(
		'id'                 => $customer->get_id(),
		'email'              => $customer->get_email(),
		'first_name'         => $customer->get_first_name(),
		'last_name'          => $customer->get_last_name(),
		'username'           => $customer->get_username(),
		'date_created'       => $customer->get_date_created() ? $customer->get_date_created()->date( 'c' ) : '',
		'order_count'        => (int) $customer->get_order_count(),
		'total_spent'        => (string) $customer->get_total_spent(),
		'is_paying_customer' => (bool) $customer->get_is_paying_customer(),
	);
}
