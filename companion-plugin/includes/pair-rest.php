<?php
/**
 * Device pairing — REST endpoints for the WooAgent OS daemon.
 *
 * Why REST and not MCP: pair handshake is a one-time bootstrap that has to
 * succeed BEFORE the daemon has any auth credential. The MCP Adapter
 * generally requires authentication, so a chicken-and-egg makes MCP a poor
 * fit for the very first round-trip. After pairing, the device token
 * minted here is the credential the daemon uses for every subsequent MCP
 * call (v0.2 enforcement; v0.1 records the token but Application Password
 * still drives ability invocations).
 *
 * Three routes under /wp-json/wooagent/v1/pair/*:
 *   POST /pair/request        — daemon registers a pending code (unauth)
 *   GET  /pair/poll?code=...  — daemon polls for approval (unauth: code IS auth)
 *   POST /pair/revoke         — daemon revokes a device (auth: device token)
 *
 * State:
 *   - Pending pairs: WP transients keyed `wooagent_pair_<code>`, 10 min TTL.
 *   - Approved devices: wp_option `wooagent_devices`, JSON array of
 *     {id, name, token_hash (sha256), created_at}. We never store the
 *     plaintext token outside the transient; the daemon picks it up on the
 *     first /poll after approval and the transient expires shortly after.
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

const WOOAGENT_PAIR_TTL_SECONDS = 600;
const WOOAGENT_DEVICES_OPTION   = 'wooagent_devices';

add_action( 'rest_api_init', 'wooagent_companion_register_pair_routes' );

function wooagent_companion_register_pair_routes(): void {
	register_rest_route(
		'wooagent/v1',
		'/pair/request',
		array(
			'methods'             => 'POST',
			'permission_callback' => '__return_true',
			'callback'            => 'wooagent_companion_pair_request',
		)
	);

	register_rest_route(
		'wooagent/v1',
		'/pair/poll',
		array(
			'methods'             => 'GET',
			'permission_callback' => '__return_true',
			'callback'            => 'wooagent_companion_pair_poll',
		)
	);

	register_rest_route(
		'wooagent/v1',
		'/pair/revoke',
		array(
			'methods'             => 'POST',
			'permission_callback' => 'wooagent_companion_pair_revoke_permission',
			'callback'            => 'wooagent_companion_pair_revoke',
		)
	);
}

/**
 * Records a pending pairing under the daemon-supplied code. Idempotent
 * within the TTL window: re-posting the same code overwrites the
 * transient (the daemon's POST /v1/stores rotates codes the same way).
 */
function wooagent_companion_pair_request( WP_REST_Request $request ) {
	$code        = (string) $request->get_param( 'code' );
	$device_name = (string) $request->get_param( 'device_name' );

	if ( ! wooagent_companion_pair_valid_code( $code ) ) {
		return new WP_Error(
			'invalid_code',
			__( 'pair/request requires a code in the form WOOA-XXXX-XXXX.', 'wooagent-companion' ),
			array( 'status' => 400 )
		);
	}
	if ( $device_name === '' ) {
		$device_name = 'wooagent-device';
	}

	$expires_at = time() + WOOAGENT_PAIR_TTL_SECONDS;
	set_transient(
		wooagent_companion_pair_transient_key( $code ),
		array(
			'status'      => 'pending',
			'device_name' => $device_name,
			'expires_at'  => $expires_at,
		),
		WOOAGENT_PAIR_TTL_SECONDS
	);

	return rest_ensure_response(
		array(
			'status'     => 'pending',
			'expires_at' => gmdate( 'c', $expires_at ),
		)
	);
}

/**
 * Daemon polls this with the code it gave the operator. Three terminal
 * states: pending (operator hasn't acted), approved (operator clicked
 * Approve in wp-admin → mint a device_token + return it once), rejected
 * (operator clicked Reject). 404 on missing/expired so the daemon can
 * distinguish "still pending" from "the window closed."
 *
 * Approved-token delivery is single-use: once we return the token, we
 * clear it from the transient and update status to 'approved_delivered'
 * so a second /poll returns approved without re-delivering the secret.
 * Polling-after-delivery is treated as success — re-deliveries would
 * leak the token if a poll log got intercepted.
 */
function wooagent_companion_pair_poll( WP_REST_Request $request ) {
	$code = (string) $request->get_param( 'code' );
	if ( ! wooagent_companion_pair_valid_code( $code ) ) {
		return new WP_Error( 'invalid_code', __( 'invalid code', 'wooagent-companion' ), array( 'status' => 400 ) );
	}

	$key  = wooagent_companion_pair_transient_key( $code );
	$data = get_transient( $key );
	if ( ! is_array( $data ) ) {
		return new WP_Error( 'not_found', __( 'pairing not found or expired', 'wooagent-companion' ), array( 'status' => 404 ) );
	}

	switch ( $data['status'] ) {
		case 'pending':
			return rest_ensure_response( array( 'status' => 'pending' ) );

		case 'approved':
			$response = array(
				'status'       => 'approved',
				'device_id'    => $data['device_id'] ?? '',
				'device_name'  => $data['device_name'] ?? '',
				'device_token' => $data['device_token'] ?? '',
			);
			$data['status'] = 'approved_delivered';
			unset( $data['device_token'] );
			set_transient( $key, $data, WOOAGENT_PAIR_TTL_SECONDS );
			return rest_ensure_response( $response );

		case 'approved_delivered':
			return rest_ensure_response(
				array(
					'status'      => 'approved',
					'device_id'   => $data['device_id'] ?? '',
					'device_name' => $data['device_name'] ?? '',
				)
			);

		case 'rejected':
			return rest_ensure_response( array( 'status' => 'rejected' ) );
	}

	return new WP_Error( 'invalid_state', 'unknown pairing state', array( 'status' => 500 ) );
}

/**
 * Permission check for /pair/revoke: bearer must match a known device's
 * token hash. Any registered device may revoke any device — multi-device
 * scoping is post-v0.1 and the test store is single-tenant.
 */
function wooagent_companion_pair_revoke_permission( WP_REST_Request $request ): bool {
	$header = $request->get_header( 'Authorization' );
	if ( ! is_string( $header ) || stripos( $header, 'Bearer ' ) !== 0 ) {
		return false;
	}
	$token = substr( $header, 7 );
	if ( $token === '' ) {
		return false;
	}
	$hash    = hash( 'sha256', $token );
	$devices = get_option( WOOAGENT_DEVICES_OPTION, array() );
	if ( ! is_array( $devices ) ) {
		return false;
	}
	foreach ( $devices as $d ) {
		if ( isset( $d['token_hash'] ) && hash_equals( $d['token_hash'], $hash ) ) {
			return true;
		}
	}
	return false;
}

/**
 * Revokes the device whose token the bearer presented. We use the bearer
 * as the identity rather than an explicit device_id in the body — that
 * keeps the daemon from having to remember a separate id alongside the
 * token. The permission_callback already verified the bearer matches
 * SOME registered device; we drop that one.
 */
function wooagent_companion_pair_revoke( WP_REST_Request $request ) {
	$header = (string) $request->get_header( 'Authorization' );
	$token  = substr( $header, 7 ); // permission_callback enforced "Bearer "
	$hash   = hash( 'sha256', $token );

	$devices = get_option( WOOAGENT_DEVICES_OPTION, array() );
	if ( ! is_array( $devices ) ) {
		$devices = array();
	}
	$kept = array_values(
		array_filter(
			$devices,
			static function ( $d ) use ( $hash ) {
				return ! ( isset( $d['token_hash'] ) && hash_equals( $d['token_hash'], $hash ) );
			}
		)
	);
	update_option( WOOAGENT_DEVICES_OPTION, $kept );
	return rest_ensure_response( array( 'revoked' => true ) );
}

/**
 * Internal helper used by the wp-admin Approve handler. Mints a token,
 * persists the device, and stashes the plaintext token onto the
 * transient so /poll can deliver it once.
 *
 * Returns true on success; false if the code's transient is missing,
 * expired, or already terminal.
 */
function wooagent_companion_pair_approve_code( string $code ): bool {
	$key  = wooagent_companion_pair_transient_key( $code );
	$data = get_transient( $key );
	if ( ! is_array( $data ) || ( $data['status'] ?? '' ) !== 'pending' ) {
		return false;
	}

	$device_id    = 'dev_' . wp_generate_uuid4();
	$device_token = wp_generate_password( 48, false, false );

	$devices = get_option( WOOAGENT_DEVICES_OPTION, array() );
	if ( ! is_array( $devices ) ) {
		$devices = array();
	}
	$devices[] = array(
		'id'         => $device_id,
		'name'       => $data['device_name'] ?? 'wooagent-device',
		'token_hash' => hash( 'sha256', $device_token ),
		'created_at' => gmdate( 'c' ),
	);
	update_option( WOOAGENT_DEVICES_OPTION, $devices );

	$data['status']       = 'approved';
	$data['device_id']    = $device_id;
	$data['device_token'] = $device_token;
	set_transient( $key, $data, WOOAGENT_PAIR_TTL_SECONDS );
	return true;
}

function wooagent_companion_pair_reject_code( string $code ): bool {
	$key  = wooagent_companion_pair_transient_key( $code );
	$data = get_transient( $key );
	if ( ! is_array( $data ) || ( $data['status'] ?? '' ) !== 'pending' ) {
		return false;
	}
	$data['status'] = 'rejected';
	unset( $data['device_token'] );
	set_transient( $key, $data, WOOAGENT_PAIR_TTL_SECONDS );
	return true;
}

function wooagent_companion_pair_valid_code( string $code ): bool {
	return (bool) preg_match( '/^WOOA-[A-Z0-9]{4}-[A-Z0-9]{4}$/', $code );
}

function wooagent_companion_pair_transient_key( string $code ): string {
	// Transient names have a 172-char limit; ours is 23. The literal code
	// is fine — anyone who can read the transient already has DB access.
	return 'wooagent_pair_' . $code;
}
