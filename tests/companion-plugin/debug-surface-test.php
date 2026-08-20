<?php

require_once __DIR__ . '/bootstrap.php';

$plugin_file = WOOAGENT_TEST_ROOT . '/companion-plugin/wooagent-companion.php';
$source      = file_get_contents( $plugin_file );
$updater     = file_get_contents( WOOAGENT_TEST_ROOT . '/companion-plugin/includes/update-checker.php' );

if ( $source === false || $updater === false ) {
	throw new RuntimeException( 'Unable to read the production plugin files.' );
}

wooagent_test_run(
	'does not ship the internal debug or source-inspection surface',
	static function () use ( $source ): void {
		$forbidden = array(
			'wooagent_companion_register_debug_route',
			"'/source'",
			"'/selftest'",
			'ReflectionClass',
			'ReflectionFunction',
			'file_get_contents',
			'set_error_handler',
			'wooagent_companion_register_results',
			'wooagent_companion_register_errors',
		);

		foreach ( $forbidden as $marker ) {
			if ( strpos( $source, $marker ) !== false ) {
				throw new RuntimeException( 'Forbidden production marker remains: ' . $marker );
			}
		}
	}
);

wooagent_test_run(
	'keeps all production ability registration calls',
	static function () use ( $source ): void {
		$required = array(
			'wooagent_companion_register_product_abilities();',
			'wooagent_companion_register_order_abilities();',
			'wooagent_companion_register_customer_abilities();',
		);

		foreach ( $required as $marker ) {
			if ( strpos( $source, $marker ) === false ) {
				throw new RuntimeException( 'Required production registration call is missing: ' . $marker );
			}
		}
	}
);

wooagent_test_run(
	'requires the exact Companion release asset without source-zip fallback',
	static function () use ( $updater ): void {
		$required = array(
			"'/^wooagent-companion\\.zip$/'",
			'Api::REQUIRE_RELEASE_ASSETS',
		);

		foreach ( $required as $marker ) {
			if ( strpos( $updater, $marker ) === false ) {
				throw new RuntimeException( 'Required updater constraint is missing: ' . $marker );
			}
		}
	}
);

wooagent_test_finish();
