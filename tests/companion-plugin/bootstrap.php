<?php
/**
 * Dependency-free WordPress doubles for Companion Plugin regression tests.
 */

define( 'WOOAGENT_TEST_ROOT', dirname( __DIR__, 2 ) );

if ( ! defined( 'ABSPATH' ) ) {
	define( 'ABSPATH', WOOAGENT_TEST_ROOT . '/' );
}

$GLOBALS['wooagent_test_options']         = array();
$GLOBALS['wooagent_test_transients']      = array();
$GLOBALS['wooagent_test_users']           = array();
$GLOBALS['wooagent_test_routes']          = array();
$GLOBALS['wooagent_test_abilities']       = array();
$GLOBALS['wooagent_test_caps']            = array();
$GLOBALS['wooagent_test_current_user_id'] = 0;
$GLOBALS['wooagent_test_failures']        = 0;

final class WP_Error {
	private $code;
	private $message;
	private $data;

	public function __construct( $code = '', $message = '', $data = null ) {
		$this->code    = $code;
		$this->message = $message;
		$this->data    = $data;
	}

	public function get_error_code() {
		return $this->code;
	}

	public function get_error_message() {
		return $this->message;
	}

	public function get_error_data() {
		return $this->data;
	}
}

final class WP_REST_Request {
	private $params;
	private $headers;

	public function __construct( array $params = array(), array $headers = array() ) {
		$this->params  = $params;
		$this->headers = array_change_key_case( $headers, CASE_LOWER );
	}

	public function get_param( $name ) {
		return $this->params[ $name ] ?? null;
	}

	public function get_header( $name ) {
		return $this->headers[ strtolower( $name ) ] ?? '';
	}
}

final class WP_REST_Response {
	private $data;

	public function __construct( $data ) {
		$this->data = $data;
	}

	public function get_data() {
		return $this->data;
	}
}

function wooagent_test_reset(): void {
	$GLOBALS['wooagent_test_options']         = array();
	$GLOBALS['wooagent_test_transients']      = array();
	$GLOBALS['wooagent_test_users']           = array();
	$GLOBALS['wooagent_test_routes']          = array();
	$GLOBALS['wooagent_test_abilities']       = array();
	$GLOBALS['wooagent_test_caps']            = array();
	$GLOBALS['wooagent_test_current_user_id'] = 0;
	unset( $_SERVER['REMOTE_ADDR'], $_SERVER['HTTP_AUTHORIZATION'], $_SERVER['REDIRECT_HTTP_AUTHORIZATION'] );
}

function wooagent_test_run( string $name, callable $test ): void {
	try {
		$test();
		echo '[PASS] ' . $name . PHP_EOL;
	} catch ( Throwable $error ) {
		$GLOBALS['wooagent_test_failures']++;
		echo '[FAIL] ' . $name . ': ' . $error->getMessage() . PHP_EOL;
	}
}

function wooagent_test_finish(): void {
	if ( $GLOBALS['wooagent_test_failures'] > 0 ) {
		exit( 1 );
	}
}

function wooagent_test_expect_true( $actual, string $message ): void {
	if ( $actual !== true ) {
		throw new RuntimeException( $message );
	}
}

function wooagent_test_expect_false( $actual, string $message ): void {
	if ( $actual !== false ) {
		throw new RuntimeException( $message );
	}
}

function wooagent_test_expect_same( $expected, $actual, string $message ): void {
	if ( $actual !== $expected ) {
		throw new RuntimeException( $message );
	}
}

function wooagent_test_expect_instance_of( string $class_name, $actual, string $message ): void {
	if ( ! ( $actual instanceof $class_name ) ) {
		throw new RuntimeException( $message );
	}
}

function add_action( $hook_name, $callback, $priority = 10, $accepted_args = 1 ): bool {
	return true;
}

function add_filter( $hook_name, $callback, $priority = 10, $accepted_args = 1 ): bool {
	return true;
}

function register_rest_route( $namespace, $route, $args, $override = false ): bool {
	$GLOBALS['wooagent_test_routes'][ $namespace . $route ] = $args;
	return true;
}

function wp_register_ability( $name, $args ): bool {
	$GLOBALS['wooagent_test_abilities'][ $name ] = $args;
	return true;
}

function get_option( $name, $default = false ) {
	return array_key_exists( $name, $GLOBALS['wooagent_test_options'] )
		? $GLOBALS['wooagent_test_options'][ $name ]
		: $default;
}

function update_option( $name, $value, $autoload = null ): bool {
	$GLOBALS['wooagent_test_options'][ $name ] = $value;
	return true;
}

function get_transient( $name ) {
	if ( ! isset( $GLOBALS['wooagent_test_transients'][ $name ] ) ) {
		return false;
	}

	$record = $GLOBALS['wooagent_test_transients'][ $name ];
	if ( $record['expires_at'] <= time() ) {
		unset( $GLOBALS['wooagent_test_transients'][ $name ] );
		return false;
	}

	return $record['value'];
}

function set_transient( $name, $value, $expiration ): bool {
	$GLOBALS['wooagent_test_transients'][ $name ] = array(
		'value'      => $value,
		'expires_at' => time() + (int) $expiration,
	);
	return true;
}

function get_user_by( $field, $value ) {
	$user_id = (int) $value;
	return $GLOBALS['wooagent_test_users'][ $user_id ] ?? false;
}

function current_user_can( $capability ): bool {
	return ! empty( $GLOBALS['wooagent_test_caps'][ $capability ] );
}

function sanitize_text_field( $value ): string {
	$value = is_scalar( $value ) ? (string) $value : '';
	return trim( preg_replace( '/[\r\n\t ]+/', ' ', strip_tags( $value ) ) );
}

function rest_ensure_response( $value ): WP_REST_Response {
	return $value instanceof WP_REST_Response ? $value : new WP_REST_Response( $value );
}

function nocache_headers(): void {}

function wp_generate_uuid4(): string {
	return '11111111-2222-4333-8444-555555555555';
}

function wp_generate_password( $length = 12, $special_chars = true, $extra_special_chars = false ): string {
	return str_repeat( 't', (int) $length );
}

function get_current_user_id(): int {
	return (int) $GLOBALS['wooagent_test_current_user_id'];
}

function __( $text, $domain = 'default' ): string {
	return (string) $text;
}

function is_wp_error( $value ): bool {
	return $value instanceof WP_Error;
}
