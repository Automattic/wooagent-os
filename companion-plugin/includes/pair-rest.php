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
 * Four routes under /wp-json/wooagent/v1/*:
 *   POST /pair/request        — daemon registers a pending code (unauth)
 *   GET  /pair/poll?code=...  — daemon polls for approval (unauth: code IS auth)
 *   POST /pair/revoke         — daemon revokes a device (auth: device token)
 *   GET  /devices/me          — daemon probes that its bearer is still known
 *                               (DSGWOO-1275 pairing staleness)
 *
 * State:
 *   - Pending pairs: WP transients keyed `wooagent_pair_<code>`, 10 min TTL.
 *   - Approved devices: wp_option `wooagent_devices`, JSON array of
 *     {id, name, token_hash (sha256), paired_by_user_id, created_at}. We
 *     never store the plaintext token outside the transient; the daemon
 *     picks it up on the first /poll after approval and the transient
 *     expires shortly after.
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

const WOOAGENT_PAIR_TTL_SECONDS = 600;
const WOOAGENT_DEVICES_OPTION   = 'wooagent_devices';
const WOOAGENT_PAIR_DEVICE_NAME_MAX_BYTES = 100;
const WOOAGENT_PAIR_RATE_WINDOW_SECONDS   = 60;
const WOOAGENT_PAIR_REQUEST_RATE_LIMIT    = 10;
const WOOAGENT_PAIR_POLL_RATE_LIMIT       = 90;

add_action( 'rest_api_init', 'wooagent_companion_register_pair_routes' );

function wooagent_companion_register_pair_routes(): void {
	register_rest_route(
		'wooagent/v1',
		'/pair/request',
		array(
			'methods'             => 'POST',
			'permission_callback' => 'wooagent_companion_pair_request_permission',
			'callback'            => 'wooagent_companion_pair_request',
			'args'                => array(
				'code'        => array(
					'type'              => 'string',
					'required'          => true,
					'pattern'           => '^WOOA-[A-Z0-9]{4}-[A-Z0-9]{4}$',
					'sanitize_callback' => 'wooagent_companion_pair_sanitize_code',
				),
				'device_name' => array(
					'type'              => 'string',
					'required'          => false,
					'maxLength'         => WOOAGENT_PAIR_DEVICE_NAME_MAX_BYTES,
					'sanitize_callback' => 'sanitize_text_field',
				),
			),
		)
	);

	register_rest_route(
		'wooagent/v1',
		'/pair/poll',
		array(
			'methods'             => 'GET',
			'permission_callback' => 'wooagent_companion_pair_poll_permission',
			'callback'            => 'wooagent_companion_pair_poll',
			'args'                => array(
				'code' => array(
					'type'              => 'string',
					'required'          => true,
					'pattern'           => '^WOOA-[A-Z0-9]{4}-[A-Z0-9]{4}$',
					'sanitize_callback' => 'wooagent_companion_pair_sanitize_code',
				),
			),
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

	register_rest_route(
		'wooagent/v1',
		'/devices/me',
		array(
			'methods'             => 'GET',
			'permission_callback' => 'wooagent_companion_devices_me_permission',
			'callback'            => 'wooagent_companion_devices_me',
		)
	);
}

/**
 * Fixed-window rate gate for public pairing registration.
 */
function wooagent_companion_pair_request_permission() {
	return wooagent_companion_pair_rate_limit( 'request', WOOAGENT_PAIR_REQUEST_RATE_LIMIT );
}

/**
 * Fixed-window rate gate for public pairing polling. The higher threshold
 * leaves headroom above the daemon UI's normal two-second poll cadence.
 */
function wooagent_companion_pair_poll_permission() {
	return wooagent_companion_pair_rate_limit( 'poll', WOOAGENT_PAIR_POLL_RATE_LIMIT );
}

/**
 * Limits a public pairing action per remote address without persisting the
 * raw address. Keeping reset_at fixed avoids turning steady polling into a
 * sliding-window lockout.
 */
function wooagent_companion_pair_rate_limit( string $action, int $limit ) {
	$remote_addr = isset( $_SERVER['REMOTE_ADDR'] ) && is_scalar( $_SERVER['REMOTE_ADDR'] )
		? (string) $_SERVER['REMOTE_ADDR']
		: 'unknown';
	$key = 'wooagent_pair_rate_' . $action . '_' . substr( hash( 'sha256', $remote_addr ), 0, 32 );

	$now    = time();
	$bucket = get_transient( $key );
	if ( ! is_array( $bucket ) || (int) ( $bucket['reset_at'] ?? 0 ) <= $now ) {
		$bucket = array(
			'count'    => 0,
			'reset_at' => $now + WOOAGENT_PAIR_RATE_WINDOW_SECONDS,
		);
	}

	if ( (int) ( $bucket['count'] ?? 0 ) >= $limit ) {
		return new WP_Error(
			'pairing_rate_limited',
			__( 'too many pairing requests; try again shortly', 'wooagent-companion' ),
			array( 'status' => 429 )
		);
	}

	$bucket['count'] = (int) $bucket['count'] + 1;
	$remaining_ttl   = max( 1, (int) $bucket['reset_at'] - $now );
	set_transient( $key, $bucket, $remaining_ttl );
	return true;
}

/**
 * Records a pending pairing under the daemon-supplied code. Re-posting an
 * existing pending code is idempotent without extending or changing it;
 * terminal states can never be reset through the public endpoint.
 */
function wooagent_companion_pair_request( WP_REST_Request $request ) {
	$code = wooagent_companion_pair_sanitize_code( $request->get_param( 'code' ) );

	if ( ! wooagent_companion_pair_valid_code( $code ) ) {
		return new WP_Error(
			'invalid_code',
			__( 'pair/request requires a code in the form WOOA-XXXX-XXXX.', 'wooagent-companion' ),
			array( 'status' => 400 )
		);
	}
	$raw_device_name = $request->get_param( 'device_name' );
	if ( $raw_device_name !== null && ! is_scalar( $raw_device_name ) ) {
		return new WP_Error(
			'invalid_device_name',
			__( 'device_name must be a string no longer than 100 bytes.', 'wooagent-companion' ),
			array( 'status' => 400 )
		);
	}
	$device_name = sanitize_text_field( $raw_device_name === null ? '' : (string) $raw_device_name );
	if ( strlen( $device_name ) > WOOAGENT_PAIR_DEVICE_NAME_MAX_BYTES ) {
		return new WP_Error(
			'invalid_device_name',
			__( 'device_name must be a string no longer than 100 bytes.', 'wooagent-companion' ),
			array( 'status' => 400 )
		);
	}
	if ( $device_name === '' ) {
		$device_name = 'wooagent-device';
	}

	$key      = wooagent_companion_pair_transient_key( $code );
	$existing = get_transient( $key );
	if ( is_array( $existing ) ) {
		if ( ( $existing['status'] ?? '' ) === 'pending' ) {
			$existing_expires_at = (int) ( $existing['expires_at'] ?? 0 );
			return rest_ensure_response(
				array(
					'status'     => 'pending',
					'expires_at' => gmdate( 'c', $existing_expires_at > 0 ? $existing_expires_at : time() ),
				)
			);
		}

		return new WP_Error(
			'pairing_code_in_use',
			__( 'pairing code is already in use', 'wooagent-companion' ),
			array( 'status' => 409 )
		);
	}

	$expires_at = time() + WOOAGENT_PAIR_TTL_SECONDS;
	set_transient(
		$key,
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
	// Critical: this endpoint is GET and changes state (operator approves
	// in wp-admin → next poll must see status='approved'). Without
	// nocache_headers() WPCom/Pressable's Batcache caches the first
	// "pending" response for 5 minutes, leaving the daemon stuck on
	// stale pending status until the cache expires. Bug doc: the daemon
	// would otherwise report "Waiting for approval" even after wp-admin
	// has successfully approved.
	nocache_headers();

	$code = wooagent_companion_pair_sanitize_code( $request->get_param( 'code' ) );
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
 * token hash and valid approving user.
 */
function wooagent_companion_pair_revoke_permission( WP_REST_Request $request ): bool {
	return wooagent_companion_find_device_by_bearer( $request ) !== null;
}

/**
 * Permission check for /devices/me: same bearer-matches-device-hash gate
 * as /pair/revoke. We don't fall back to wp-admin auth here — the daemon
 * is the only caller and it always presents the device token.
 */
function wooagent_companion_devices_me_permission( WP_REST_Request $request ): bool {
	return wooagent_companion_find_device_by_bearer( $request ) !== null;
}

/**
 * Resolves a request's Authorization: Bearer header to a registered device
 * record, or null when no header is present, the token is unknown, or the
 * record no longer maps to an existing approving WordPress user.
 * Constant-time compare on token_hash so a timing oracle can't fingerprint
 * the device list. Shared by /pair/revoke + /devices/me.
 */
function wooagent_companion_find_device_by_bearer( WP_REST_Request $request ): ?array {
	$header = $request->get_header( 'Authorization' );
	if ( ! is_string( $header ) || stripos( $header, 'Bearer ' ) !== 0 ) {
		return null;
	}
	$token = trim( substr( $header, 7 ) );
	if ( $token === '' ) {
		return null;
	}
	$hash    = hash( 'sha256', $token );
	$devices = get_option( WOOAGENT_DEVICES_OPTION, array() );
	if ( ! is_array( $devices ) ) {
		return null;
	}
	foreach ( $devices as $d ) {
		if ( is_array( $d ) && isset( $d['token_hash'] ) && is_scalar( $d['token_hash'] ) && hash_equals( (string) $d['token_hash'], $hash ) ) {
			return wooagent_companion_device_user_id( $d ) > 0 ? $d : null;
		}
	}
	return null;
}

/**
 * Returns the existing WordPress user that approved a device, or 0 when the
 * record predates approver capture or that user has since been deleted.
 * Invalid records fail closed and must pair again instead of inheriting a
 * different administrator's identity.
 */
function wooagent_companion_device_user_id( array $device ): int {
	$user_id = isset( $device['paired_by_user_id'] ) ? (int) $device['paired_by_user_id'] : 0;
	if ( $user_id <= 0 || ! get_user_by( 'id', $user_id ) ) {
		return 0;
	}
	return $user_id;
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
				if ( ! is_array( $d ) || ! isset( $d['token_hash'] ) || ! is_scalar( $d['token_hash'] ) ) {
					return true;
				}
				return ! hash_equals( (string) $d['token_hash'], $hash );
			}
		)
	);
	update_option( WOOAGENT_DEVICES_OPTION, $kept );
	return rest_ensure_response( array( 'revoked' => true ) );
}

/**
 * Daemon-side staleness probe. The daemon hits this on every read of a
 * `paired` store row (throttled to ~30s per row) to confirm its bearer is
 * still known to wp-admin. A 401 here is the signal that the operator
 * clicked "Remove" on the device or the wooagent_devices option was
 * reset; the daemon flips its local row out of `paired` and drops the
 * keychain entry.
 *
 * Returns the minimal device record — id, name, created_at, and the
 * paired_by_user_id captured at approve time so the daemon can surface
 * "paired by <user>" if a future UI wants it. The token_hash is never
 * returned (would leak the equality of two bearers across observers).
 *
 * nocache_headers() so Batcache / WP supercache can't serve a stale 200
 * response after the operator clicks Remove — same fix as /pair/poll.
 */
function wooagent_companion_devices_me( WP_REST_Request $request ) {
	nocache_headers();
	$device = wooagent_companion_find_device_by_bearer( $request );
	if ( $device === null ) {
		// permission_callback would have already rejected, but defensive
		// so we never accidentally 200 with an empty body.
		return new WP_Error( 'unknown_device', __( 'unknown device', 'wooagent-companion' ), array( 'status' => 401 ) );
	}
	return rest_ensure_response(
		array(
			'id'                => $device['id'] ?? '',
			'name'              => $device['name'] ?? '',
			'paired_by_user_id' => isset( $device['paired_by_user_id'] ) ? (int) $device['paired_by_user_id'] : 0,
			'created_at'        => $device['created_at'] ?? '',
		)
	);
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
	// Capture the WP user_id of the admin clicking Approve so the auth-bridge
	// filter (auth-bridge.php) can map this device's bearer back to a
	// known WP user identity. Falls back to 0 when called from a context
	// without a logged-in user (shouldn't happen — the wp-admin pair
	// screen requires manage_options — but defensive).
	$approver = get_current_user_id();
	$devices[] = array(
		'id'                => $device_id,
		'name'              => $data['device_name'] ?? 'wooagent-device',
		'token_hash'        => hash( 'sha256', $device_token ),
		'paired_by_user_id' => $approver,
		'created_at'        => gmdate( 'c' ),
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

function wooagent_companion_pair_sanitize_code( $code ): string {
	return is_scalar( $code ) ? strtoupper( sanitize_text_field( (string) $code ) ) : '';
}

function wooagent_companion_pair_transient_key( string $code ): string {
	// Transient names have a 172-char limit; ours is 23. The literal code
	// is fine — anyone who can read the transient already has DB access.
	return 'wooagent_pair_' . $code;
}
