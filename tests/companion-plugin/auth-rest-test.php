<?php

define( 'REST_REQUEST', true );

require_once __DIR__ . '/bootstrap.php';
require_once WOOAGENT_TEST_ROOT . '/companion-plugin/includes/pair-rest.php';
require_once WOOAGENT_TEST_ROOT . '/companion-plugin/includes/auth-bridge.php';

function wooagent_test_install_device( string $token, array $overrides = array() ): void {
	$device = array_merge(
		array(
			'id'                => 'dev_fixture',
			'name'              => 'Fixture device',
			'token_hash'        => hash( 'sha256', $token ),
			'paired_by_user_id' => 42,
			'created_at'        => '2026-08-20T00:00:00+00:00',
		),
		$overrides
	);
	$GLOBALS['wooagent_test_options'][ WOOAGENT_DEVICES_OPTION ] = array( $device );
}

function wooagent_test_set_server_bearer( string $token ): void {
	$_SERVER['HTTP_AUTHORIZATION'] = 'Bearer ' . $token;
}

wooagent_test_run(
	'preserves an identity resolved by an earlier authentication method',
	static function (): void {
		wooagent_test_reset();
		wooagent_test_set_server_bearer( 'fixture-preauthenticated-token' );
		wooagent_test_expect_same( 17, wooagent_companion_resolve_bearer_user( 17 ), 'Existing WordPress authentication must win.' );
	}
);

wooagent_test_run(
	'resolves a valid REST bearer to its existing approving user',
	static function (): void {
		wooagent_test_reset();
		$token = 'fixture-valid-device-token';
		$GLOBALS['wooagent_test_users'][42] = (object) array( 'ID' => 42 );
		wooagent_test_install_device( $token );
		wooagent_test_set_server_bearer( $token );

		wooagent_test_expect_same( 42, wooagent_companion_resolve_bearer_user( false ), 'A valid device must resolve to its approving user.' );

		$request = new WP_REST_Request( array(), array( 'Authorization' => 'Bearer ' . $token ) );
		$device  = wooagent_companion_find_device_by_bearer( $request );
		wooagent_test_expect_same( 'dev_fixture', $device['id'] ?? null, 'The bearer helper must return the valid device.' );
	}
);

wooagent_test_run(
	'leaves an unknown bearer unauthenticated',
	static function (): void {
		wooagent_test_reset();
		$GLOBALS['wooagent_test_users'][42] = (object) array( 'ID' => 42 );
		wooagent_test_install_device( 'fixture-known-device-token' );
		wooagent_test_set_server_bearer( 'fixture-unknown-device-token' );

		wooagent_test_expect_false( wooagent_companion_resolve_bearer_user( false ), 'Unknown bearers must not authenticate.' );
	}
);

wooagent_test_run(
	'rejects legacy device records without an approving user identity',
	static function (): void {
		wooagent_test_reset();
		$token = 'fixture-legacy-device-token';
		$GLOBALS['wooagent_test_users'][42] = (object) array( 'ID' => 42 );
		wooagent_test_install_device( $token, array( 'paired_by_user_id' => 0 ) );
		wooagent_test_set_server_bearer( $token );

		wooagent_test_expect_false( wooagent_companion_resolve_bearer_user( false ), 'Legacy records must fail closed instead of selecting an administrator.' );

		$request = new WP_REST_Request( array(), array( 'Authorization' => 'Bearer ' . $token ) );
		wooagent_test_expect_same( null, wooagent_companion_find_device_by_bearer( $request ), 'Legacy records must fail the device probe and revoke permission gate.' );
	}
);

wooagent_test_run(
	'rejects device records whose approving user was deleted',
	static function (): void {
		wooagent_test_reset();
		$token = 'fixture-deleted-user-device-token';
		wooagent_test_install_device( $token, array( 'paired_by_user_id' => 99 ) );
		wooagent_test_set_server_bearer( $token );

		wooagent_test_expect_false( wooagent_companion_resolve_bearer_user( false ), 'Deleted approvers must not remain valid identities.' );

		$request = new WP_REST_Request( array(), array( 'Authorization' => 'Bearer ' . $token ) );
		wooagent_test_expect_same( null, wooagent_companion_find_device_by_bearer( $request ), 'Deleted approvers must fail the device probe and revoke permission gate.' );
	}
);

wooagent_test_finish();
