<?php

require_once __DIR__ . '/bootstrap.php';
require_once WOOAGENT_TEST_ROOT . '/companion-plugin/includes/pair-rest.php';

function wooagent_test_error_status( WP_Error $error ): int {
	$data = $error->get_error_data();
	return is_array( $data ) ? (int) ( $data['status'] ?? 0 ) : 0;
}

wooagent_test_run(
	'rejects malformed pairing codes',
	static function (): void {
		wooagent_test_reset();
		$result = wooagent_companion_pair_request( new WP_REST_Request( array( 'code' => 'not-a-code' ) ) );
		wooagent_test_expect_instance_of( WP_Error::class, $result, 'Malformed codes must return WP_Error.' );
		wooagent_test_expect_same( 'invalid_code', $result->get_error_code(), 'Malformed codes need the stable invalid_code error.' );
		wooagent_test_expect_same( 400, wooagent_test_error_status( $result ), 'Malformed codes must return HTTP 400.' );
	}
);

wooagent_test_run(
	'sanitizes, defaults, and bounds device names',
	static function (): void {
		wooagent_test_reset();
		$code = 'WOOA-ABCD-1234';
		$result = wooagent_companion_pair_request(
			new WP_REST_Request(
				array(
					'code'        => $code,
					'device_name' => "  <b>My\nDevice</b>  ",
				)
			)
		);
		wooagent_test_expect_instance_of( WP_REST_Response::class, $result, 'A valid request must return a REST response.' );
		$stored = get_transient( wooagent_companion_pair_transient_key( $code ) );
		wooagent_test_expect_same( 'My Device', $stored['device_name'] ?? null, 'Device names must be sanitized before storage.' );

		$default_code = 'WOOA-BCDE-2345';
		wooagent_companion_pair_request( new WP_REST_Request( array( 'code' => $default_code, 'device_name' => '' ) ) );
		$default = get_transient( wooagent_companion_pair_transient_key( $default_code ) );
		wooagent_test_expect_same( 'wooagent-device', $default['device_name'] ?? null, 'Blank device names must use the safe default.' );

		$long_result = wooagent_companion_pair_request(
			new WP_REST_Request(
				array(
					'code'        => 'WOOA-CDEF-3456',
					'device_name' => str_repeat( 'x', 101 ),
				)
			)
		);
		wooagent_test_expect_instance_of( WP_Error::class, $long_result, 'Overlong device names must be rejected.' );
		wooagent_test_expect_same( 'invalid_device_name', $long_result->get_error_code(), 'Overlong names need a stable error code.' );
		wooagent_test_expect_same( 400, wooagent_test_error_status( $long_result ), 'Overlong names must return HTTP 400.' );
	}
);

wooagent_test_run(
	'keeps an existing pending pairing unchanged on retry',
	static function (): void {
		wooagent_test_reset();
		$code = 'WOOA-DEFG-4567';
		$key  = wooagent_companion_pair_transient_key( $code );
		set_transient(
			$key,
			array(
				'status'      => 'pending',
				'device_name' => 'Original device',
				'expires_at'  => 2000000000,
			),
			321
		);
		$before = $GLOBALS['wooagent_test_transients'][ $key ];

		$result = wooagent_companion_pair_request(
			new WP_REST_Request( array( 'code' => $code, 'device_name' => 'Replacement device' ) )
		);
		wooagent_test_expect_instance_of( WP_REST_Response::class, $result, 'Pending retry must remain successful.' );
		wooagent_test_expect_same( $before, $GLOBALS['wooagent_test_transients'][ $key ], 'Pending retry must not change name, expiry, or TTL.' );
	}
);

wooagent_test_run(
	'never overwrites terminal pairing state',
	static function (): void {
		foreach ( array( 'approved', 'approved_delivered', 'rejected' ) as $index => $status ) {
			wooagent_test_reset();
			$code = 'WOOA-EFG' . $index . '-5678';
			$key  = wooagent_companion_pair_transient_key( $code );
			set_transient(
				$key,
				array(
					'status'      => $status,
					'device_name' => 'Terminal device',
					'expires_at'  => 2000000000,
				),
				321
			);
			$before = $GLOBALS['wooagent_test_transients'][ $key ];

			$result = wooagent_companion_pair_request( new WP_REST_Request( array( 'code' => $code ) ) );
			wooagent_test_expect_instance_of( WP_Error::class, $result, 'Terminal retries must return WP_Error.' );
			wooagent_test_expect_same( 'pairing_code_in_use', $result->get_error_code(), 'Terminal retries need the stable conflict error.' );
			wooagent_test_expect_same( 409, wooagent_test_error_status( $result ), 'Terminal retries must return HTTP 409.' );
			wooagent_test_expect_same( $before, $GLOBALS['wooagent_test_transients'][ $key ], 'Terminal retries must not overwrite state.' );
		}
	}
);

wooagent_test_run(
	'rate limits pairing requests without storing the raw client address',
	static function (): void {
		wooagent_test_reset();
		$_SERVER['REMOTE_ADDR'] = '192.0.2.10';

		for ( $attempt = 0; $attempt < 10; $attempt++ ) {
			wooagent_test_expect_true( wooagent_companion_pair_request_permission(), 'The first ten request attempts must be allowed.' );
		}
		$limited = wooagent_companion_pair_request_permission();
		wooagent_test_expect_instance_of( WP_Error::class, $limited, 'The eleventh request attempt must be rate limited.' );
		wooagent_test_expect_same( 'pairing_rate_limited', $limited->get_error_code(), 'Request rate limit needs a stable error code.' );
		wooagent_test_expect_same( 429, wooagent_test_error_status( $limited ), 'Request rate limit must return HTTP 429.' );
		wooagent_test_expect_false(
			strpos( serialize( $GLOBALS['wooagent_test_transients'] ), '192.0.2.10' ) !== false,
			'Rate-limit storage must not contain a raw client address.'
		);
	}
);

wooagent_test_run(
	'uses a separate higher rate limit for pairing polls',
	static function (): void {
		wooagent_test_reset();
		$_SERVER['REMOTE_ADDR'] = '198.51.100.25';

		for ( $attempt = 0; $attempt < 90; $attempt++ ) {
			wooagent_test_expect_true( wooagent_companion_pair_poll_permission(), 'The first ninety poll attempts must be allowed.' );
		}
		$limited = wooagent_companion_pair_poll_permission();
		wooagent_test_expect_instance_of( WP_Error::class, $limited, 'The ninety-first poll attempt must be rate limited.' );
		wooagent_test_expect_same( 429, wooagent_test_error_status( $limited ), 'Poll rate limit must return HTTP 429.' );
	}
);

wooagent_test_run(
	'registers bounded pairing arguments and concrete public permission callbacks',
	static function (): void {
		wooagent_test_reset();
		wooagent_companion_register_pair_routes();
		$request_route = $GLOBALS['wooagent_test_routes']['wooagent/v1/pair/request'];
		$poll_route    = $GLOBALS['wooagent_test_routes']['wooagent/v1/pair/poll'];

		wooagent_test_expect_same( 'wooagent_companion_pair_request_permission', $request_route['permission_callback'] ?? null, 'Pair request needs its rate-limit permission callback.' );
		wooagent_test_expect_same( 'wooagent_companion_pair_poll_permission', $poll_route['permission_callback'] ?? null, 'Pair poll needs its rate-limit permission callback.' );
		wooagent_test_expect_same( true, $request_route['args']['code']['required'] ?? null, 'Pair request code must be required.' );
		wooagent_test_expect_same( 'string', $request_route['args']['code']['type'] ?? null, 'Pair request code must be a string.' );
		wooagent_test_expect_same( 100, $request_route['args']['device_name']['maxLength'] ?? null, 'Device names must be bounded in the REST schema.' );
		wooagent_test_expect_same( true, $poll_route['args']['code']['required'] ?? null, 'Pair poll code must be required.' );
	}
);

wooagent_test_run(
	'ignores malformed stored rows while revoking the matching device',
	static function (): void {
		wooagent_test_reset();
		$token = 'fixture-revoke-device-token';
		$GLOBALS['wooagent_test_options'][ WOOAGENT_DEVICES_OPTION ] = array(
			'malformed-row',
			array( 'id' => 'bad_hash', 'token_hash' => array( 'unexpected' ) ),
			array( 'id' => 'dev_matching', 'token_hash' => hash( 'sha256', $token ) ),
		);

		$result = wooagent_companion_pair_revoke(
			new WP_REST_Request( array(), array( 'Authorization' => 'Bearer ' . $token ) )
		);
		wooagent_test_expect_instance_of( WP_REST_Response::class, $result, 'Malformed rows must not crash revoke.' );
		wooagent_test_expect_same( array( 'revoked' => true ), $result->get_data(), 'Revoke must report success.' );
		$remaining = get_option( WOOAGENT_DEVICES_OPTION, array() );
		wooagent_test_expect_same( 2, count( $remaining ), 'Only the matching valid row should be removed.' );
	}
);

wooagent_test_finish();
