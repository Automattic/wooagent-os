<?php
/**
 * WooAgent Companion ↔ WordPress auth bridge.
 *
 * The pairing handshake (pair-rest.php) mints a `device_token` that the
 * daemon sends as `Authorization: Bearer <token>` on every request —
 * including calls to the WP MCP Adapter at
 * /wp-json/mcp/mcp-adapter-default-server. Without this bridge those calls
 * return 401 rest_forbidden, because WordPress doesn't recognize the
 * bearer token as a known user identity.
 *
 * This file registers a `determine_current_user` filter for REST requests
 * that:
 *
 *   1. Reads the incoming Authorization header.
 *   2. Hashes the bearer with SHA-256.
 *   3. Looks the hash up in the wooagent_devices wp_option.
 *   4. If matched, returns the WP user_id that approved this device's
 *      pairing (captured at approve-time and stored on the device record).
 *      The MCP Adapter then sees an authenticated user with that user's
 *      capabilities; standard WP permission checks cascade.
 *   5. If unmatched, returns the existing $user_id unchanged so other auth
 *      mechanisms (cookies, Application Passwords, JWT) still apply.
 *
 * Security model: the bearer is identity-equivalent to a REST session for
 * the user who approved it. The pairing UI requires `manage_options` to
 * approve, and the user's current capabilities continue to govern each
 * ability. Legacy records without an approver and records whose approver
 * was deleted fail closed and must pair again. The pair-revoke route lets
 * the operator drop the device at any time, ending its access. See PRD §8.4.
 *
 * DSGWOO-1236 (related: 1275, 1276).
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

add_filter( 'determine_current_user', 'wooagent_companion_resolve_bearer_user', 20 );

/**
 * Filter callback. Returns a user_id for valid device-token bearers, or
 * the incoming $user_id unchanged for anything else. The filter runs on
 * every REST request; keep this path cheap.
 *
 * @param int|false $user_id The user_id already resolved by an earlier filter, or false if none.
 * @return int|false
 */
function wooagent_companion_resolve_bearer_user( $user_id ) {
	// Bail if an earlier filter already authenticated. Token bridge only
	// covers the "no auth yet" path so cookie / app-password / etc. wins
	// when present.
	if ( ! empty( $user_id ) ) {
		return $user_id;
	}
	if ( ! defined( 'REST_REQUEST' ) || ! REST_REQUEST ) {
		return $user_id;
	}

	$header = '';
	if ( isset( $_SERVER['HTTP_AUTHORIZATION'] ) ) {
		$header = (string) $_SERVER['HTTP_AUTHORIZATION'];
	} elseif ( isset( $_SERVER['REDIRECT_HTTP_AUTHORIZATION'] ) ) {
		// Some host setups (Apache + suexec) move the header here.
		$header = (string) $_SERVER['REDIRECT_HTTP_AUTHORIZATION'];
	} elseif ( function_exists( 'apache_request_headers' ) ) {
		$apache_headers = apache_request_headers();
		if ( ! empty( $apache_headers['Authorization'] ) ) {
			$header = (string) $apache_headers['Authorization'];
		}
	}

	if ( $header === '' || stripos( $header, 'Bearer ' ) !== 0 ) {
		return $user_id;
	}

	$token = trim( substr( $header, 7 ) );
	if ( $token === '' ) {
		return $user_id;
	}

	$hash = hash( 'sha256', $token );

	$devices = get_option( WOOAGENT_DEVICES_OPTION, array() );
	if ( ! is_array( $devices ) ) {
		return $user_id;
	}

	foreach ( $devices as $device ) {
		if ( ! is_array( $device ) ) {
			continue;
		}
		if ( empty( $device['token_hash'] ) ) {
			continue;
		}
		if ( ! hash_equals( (string) $device['token_hash'], $hash ) ) {
			continue;
		}

		$candidate = wooagent_companion_device_user_id( $device );
		return $candidate > 0 ? $candidate : $user_id;
	}

	return $user_id;
}
