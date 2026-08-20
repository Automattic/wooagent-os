<?php

define( 'REST_REQUEST', false );

require_once __DIR__ . '/bootstrap.php';
require_once WOOAGENT_TEST_ROOT . '/companion-plugin/includes/pair-rest.php';
require_once WOOAGENT_TEST_ROOT . '/companion-plugin/includes/auth-bridge.php';

wooagent_test_run(
	'ignores device bearers outside WordPress REST requests',
	static function (): void {
		$token = 'fixture-non-rest-device-token';
		$GLOBALS['wooagent_test_users'][42] = (object) array( 'ID' => 42 );
		$GLOBALS['wooagent_test_options'][ WOOAGENT_DEVICES_OPTION ] = array(
			array(
				'id'                => 'dev_fixture',
				'token_hash'        => hash( 'sha256', $token ),
				'paired_by_user_id' => 42,
			),
		);
		$_SERVER['HTTP_AUTHORIZATION'] = 'Bearer ' . $token;

		wooagent_test_expect_false(
			wooagent_companion_resolve_bearer_user( false ),
			'Bearer authentication must not run outside REST requests.'
		);
	}
);

wooagent_test_finish();
