<?php

require_once __DIR__ . '/bootstrap.php';
require_once WOOAGENT_TEST_ROOT . '/companion-plugin/includes/abilities-products.php';
require_once WOOAGENT_TEST_ROOT . '/companion-plugin/includes/abilities-orders.php';
require_once WOOAGENT_TEST_ROOT . '/companion-plugin/includes/abilities-customers.php';

wooagent_companion_register_product_abilities();
wooagent_companion_register_order_abilities();
wooagent_companion_register_customer_abilities();

wooagent_test_run(
	'registers the complete ability surface with closed input schemas',
	static function (): void {
		$expected = array(
			'wooagent-products/list',
			'wooagent-products/get',
			'wooagent-products/list-categories',
			'wooagent-products/update',
			'wooagent-products/variations-list',
			'wooagent-orders/list',
			'wooagent-orders/get',
			'wooagent-orders/add-note',
			'wooagent-customers/get',
		);
		$actual = array_keys( $GLOBALS['wooagent_test_abilities'] );
		sort( $expected );
		sort( $actual );

		wooagent_test_expect_same( $expected, $actual, 'The registered ability names changed.' );

		foreach ( $GLOBALS['wooagent_test_abilities'] as $ability ) {
			wooagent_test_expect_true( is_callable( $ability['permission_callback'] ?? null ), 'Every ability needs a callable permission callback.' );
			wooagent_test_expect_same( false, $ability['input_schema']['additionalProperties'] ?? null, 'Every input schema must reject unknown properties.' );
		}
	}
);

wooagent_test_run(
	'denies every ability without WordPress capabilities',
	static function (): void {
		$GLOBALS['wooagent_test_caps'] = array();
		foreach ( $GLOBALS['wooagent_test_abilities'] as $ability ) {
			wooagent_test_expect_false( call_user_func( $ability['permission_callback'] ), 'An ability was available without a required capability.' );
		}
	}
);

wooagent_test_run(
	'allows product reads and writes only through their existing capabilities',
	static function (): void {
		$GLOBALS['wooagent_test_caps'] = array( 'read_private_products' => true );
		wooagent_test_expect_true( wooagent_products_read_permission(), 'read_private_products must allow product reads.' );
		wooagent_test_expect_false( wooagent_products_write_permission(), 'read_private_products must not allow product writes.' );

		$GLOBALS['wooagent_test_caps'] = array( 'edit_products' => true );
		wooagent_test_expect_true( wooagent_products_read_permission(), 'edit_products must allow product reads.' );
		wooagent_test_expect_true( wooagent_products_write_permission(), 'edit_products must allow product writes.' );
	}
);

wooagent_test_run(
	'allows order and customer access only through their existing capabilities',
	static function (): void {
		$GLOBALS['wooagent_test_caps'] = array( 'read_private_shop_orders' => true );
		wooagent_test_expect_true( wooagent_orders_read_permission(), 'read_private_shop_orders must allow order reads.' );
		wooagent_test_expect_false( wooagent_orders_write_permission(), 'read_private_shop_orders must not allow order writes.' );

		$GLOBALS['wooagent_test_caps'] = array( 'edit_shop_orders' => true );
		wooagent_test_expect_true( wooagent_orders_read_permission(), 'edit_shop_orders must allow order reads.' );
		wooagent_test_expect_true( wooagent_orders_write_permission(), 'edit_shop_orders must allow order-note writes.' );
		wooagent_test_expect_true( wooagent_customers_read_permission(), 'edit_shop_orders must allow customer reads.' );

		$GLOBALS['wooagent_test_caps'] = array( 'list_users' => true );
		wooagent_test_expect_true( wooagent_customers_read_permission(), 'list_users must allow customer reads.' );
	}
);

wooagent_test_finish();
