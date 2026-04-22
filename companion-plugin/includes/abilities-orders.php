<?php
/**
 * Order abilities for WooAgent.
 *
 * wooagent-orders/list, wooagent-orders/get, wooagent-orders/add-note
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

function wooagent_companion_register_order_abilities(): void {
	wp_register_ability(
		'wooagent-orders/list',
		array(
			'label'               => __( 'List orders', 'wooagent-companion' ),
			'description'         => __( 'List WooCommerce orders, filterable by status and date.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'status'   => array(
						'type'        => 'array',
						'items'       => array( 'type' => 'string' ),
						'description' => 'WooCommerce order statuses (e.g., processing, completed). Omit for all.',
					),
					'per_page' => array( 'type' => 'integer', 'minimum' => 1, 'maximum' => 100, 'default' => 25 ),
					'page'     => array( 'type' => 'integer', 'minimum' => 1, 'default' => 1 ),
					'after'    => array(
						'type'        => 'string',
						'description' => 'ISO-8601 date. Only return orders modified after this date.',
					),
				),
				'additionalProperties' => false,
			),
			'output_schema'       => array(
				'type'       => 'object',
				'properties' => array(
					'orders' => array(
						'type'  => 'array',
						'items' => array(
							'type'       => 'object',
							'properties' => array(
								'id'           => array( 'type' => 'integer' ),
								'number'       => array( 'type' => 'string' ),
								'status'       => array( 'type' => 'string' ),
								'total'        => array( 'type' => 'string' ),
								'currency'     => array( 'type' => 'string' ),
								'customer_id'  => array( 'type' => 'integer' ),
								'line_item_count' => array( 'type' => 'integer' ),
								'date_created' => array( 'type' => 'string' ),
							),
						),
					),
					'total'  => array( 'type' => 'integer' ),
				),
				'required'   => array( 'orders', 'total' ),
			),
			'category'            => 'wooagent-orders',
			'execute_callback'    => 'wooagent_orders_list_execute',
			'permission_callback' => 'wooagent_orders_read_permission',
			'meta'                => array(
				'show_in_rest' => true,
				'mcp'          => array( 'public' => true ),
				'annotations'  => array( 'readonly' => true, 'idempotent' => true ),
			),
		)
	);

	wp_register_ability(
		'wooagent-orders/get',
		array(
			'label'               => __( 'Get order', 'wooagent-companion' ),
			'description'         => __( 'Read a single order with full line items, billing, and shipping details.', 'wooagent-companion' ),
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
					'id'              => array( 'type' => 'integer' ),
					'number'          => array( 'type' => 'string' ),
					'status'          => array( 'type' => 'string' ),
					'total'           => array( 'type' => 'string' ),
					'currency'        => array( 'type' => 'string' ),
					'customer_id'     => array( 'type' => 'integer' ),
					'customer_email'  => array( 'type' => 'string' ),
					'billing_name'    => array( 'type' => 'string' ),
					'shipping_name'   => array( 'type' => 'string' ),
					'payment_method'  => array( 'type' => 'string' ),
					'line_items'      => array(
						'type'  => 'array',
						'items' => array(
							'type'       => 'object',
							'properties' => array(
								'product_id' => array( 'type' => 'integer' ),
								'name'       => array( 'type' => 'string' ),
								'quantity'   => array( 'type' => 'integer' ),
								'total'      => array( 'type' => 'string' ),
								'sku'        => array( 'type' => 'string' ),
							),
						),
					),
					'date_created'    => array( 'type' => 'string' ),
					'date_modified'   => array( 'type' => 'string' ),
				),
				'required'   => array( 'id', 'status' ),
			),
			'category'            => 'wooagent-orders',
			'execute_callback'    => 'wooagent_orders_get_execute',
			'permission_callback' => 'wooagent_orders_read_permission',
			'meta'                => array(
				'show_in_rest' => true,
				'mcp'          => array( 'public' => true ),
				'annotations'  => array( 'readonly' => true, 'idempotent' => true ),
			),
		)
	);

	wp_register_ability(
		'wooagent-orders/add-note',
		array(
			'label'               => __( 'Add order note', 'wooagent-companion' ),
			'description'         => __( 'Add a note to an order. Customer-facing notes are emailed to the customer; internal notes are visible only in wp-admin.', 'wooagent-companion' ),
			'input_schema'        => array(
				'type'       => 'object',
				'properties' => array(
					'id'         => array( 'type' => 'integer', 'minimum' => 1 ),
					'note'       => array( 'type' => 'string', 'minLength' => 1 ),
					'is_customer_note' => array(
						'type'    => 'boolean',
						'default' => false,
						'description' => 'If true, note is visible to the customer and emailed.',
					),
				),
				'required'   => array( 'id', 'note' ),
				'additionalProperties' => false,
			),
			'output_schema'       => array(
				'type'       => 'object',
				'properties' => array(
					'note_id' => array( 'type' => 'integer' ),
				),
				'required'   => array( 'note_id' ),
			),
			'category'            => 'wooagent-orders',
			'execute_callback'    => 'wooagent_orders_add_note_execute',
			'permission_callback' => 'wooagent_orders_write_permission',
			'meta'                => array(
				'show_in_rest' => true,
				'mcp'          => array( 'public' => true ),
				'annotations'  => array( 'readonly' => false, 'destructive' => false, 'idempotent' => false ),
			),
		)
	);
}

function wooagent_orders_read_permission(): bool {
	return current_user_can( 'read_private_shop_orders' ) || current_user_can( 'edit_shop_orders' );
}

function wooagent_orders_write_permission(): bool {
	return current_user_can( 'edit_shop_orders' );
}

function wooagent_orders_list_execute( array $args ) {
	$query_args = array(
		'limit'    => $args['per_page'] ?? 25,
		'page'     => $args['page'] ?? 1,
		'paginate' => true,
		'orderby'  => 'date',
		'order'    => 'DESC',
	);

	if ( ! empty( $args['status'] ) ) {
		$query_args['status'] = $args['status'];
	}
	if ( ! empty( $args['after'] ) ) {
		$query_args['date_created'] = '>' . $args['after'];
	}

	$result = wc_get_orders( $query_args );

	$orders = array();
	foreach ( $result->orders as $order ) {
		$orders[] = array(
			'id'              => $order->get_id(),
			'number'          => (string) $order->get_order_number(),
			'status'          => $order->get_status(),
			'total'           => (string) $order->get_total(),
			'currency'        => $order->get_currency(),
			'customer_id'     => (int) $order->get_customer_id(),
			'line_item_count' => count( $order->get_items() ),
			'date_created'    => $order->get_date_created() ? $order->get_date_created()->date( 'c' ) : '',
		);
	}

	return array(
		'orders' => $orders,
		'total'  => (int) $result->total,
	);
}

function wooagent_orders_get_execute( array $args ) {
	$order = wc_get_order( (int) $args['id'] );
	if ( ! $order ) {
		return new WP_Error( 'wooagent_order_not_found', __( 'Order not found.', 'wooagent-companion' ), array( 'status' => 404 ) );
	}

	$line_items = array();
	foreach ( $order->get_items() as $item ) {
		$product = $item->get_product();
		$line_items[] = array(
			'product_id' => (int) $item->get_product_id(),
			'name'       => $item->get_name(),
			'quantity'   => (int) $item->get_quantity(),
			'total'      => (string) $item->get_total(),
			'sku'        => $product ? (string) $product->get_sku() : '',
		);
	}

	return array(
		'id'              => $order->get_id(),
		'number'          => (string) $order->get_order_number(),
		'status'          => $order->get_status(),
		'total'           => (string) $order->get_total(),
		'currency'        => $order->get_currency(),
		'customer_id'     => (int) $order->get_customer_id(),
		'customer_email'  => (string) $order->get_billing_email(),
		'billing_name'    => trim( $order->get_billing_first_name() . ' ' . $order->get_billing_last_name() ),
		'shipping_name'   => trim( $order->get_shipping_first_name() . ' ' . $order->get_shipping_last_name() ),
		'payment_method'  => (string) $order->get_payment_method_title(),
		'line_items'      => $line_items,
		'date_created'    => $order->get_date_created() ? $order->get_date_created()->date( 'c' ) : '',
		'date_modified'   => $order->get_date_modified() ? $order->get_date_modified()->date( 'c' ) : '',
	);
}

function wooagent_orders_add_note_execute( array $args ) {
	$order = wc_get_order( (int) $args['id'] );
	if ( ! $order ) {
		return new WP_Error( 'wooagent_order_not_found', __( 'Order not found.', 'wooagent-companion' ), array( 'status' => 404 ) );
	}

	$note_id = $order->add_order_note(
		(string) $args['note'],
		! empty( $args['is_customer_note'] ) ? 1 : 0,
		true
	);

	return array( 'note_id' => (int) $note_id );
}
