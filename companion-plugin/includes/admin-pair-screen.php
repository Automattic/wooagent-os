<?php
/**
 * wp-admin "WooAgent → Pair device" screen.
 *
 * Operator types the pairing code shown by the WooAgent OS daemon and
 * clicks Approve (or Reject). Approval mints a device token and stashes
 * the plaintext on the transient so the daemon's next /poll picks it up.
 *
 * The WooAgent OS daemon deep-links here with `?page=wooagent&code=XXX`,
 * which prefills the input. Operators arriving without a query param see
 * an empty input — they paste the code from the daemon UI manually.
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

add_action( 'admin_menu', 'wooagent_companion_register_admin_menu' );
add_action( 'admin_post_wooagent_pair_approve', 'wooagent_companion_handle_pair_approve' );
add_action( 'admin_post_wooagent_pair_reject', 'wooagent_companion_handle_pair_reject' );

function wooagent_companion_register_admin_menu(): void {
	add_menu_page(
		__( 'WooAgent', 'wooagent-companion' ),
		__( 'WooAgent', 'wooagent-companion' ),
		'manage_options',
		'wooagent',
		'wooagent_companion_render_pair_screen',
		'dashicons-superhero',
		58
	);
	add_submenu_page(
		'wooagent',
		__( 'Pair device', 'wooagent-companion' ),
		__( 'Pair device', 'wooagent-companion' ),
		'manage_options',
		'wooagent',
		'wooagent_companion_render_pair_screen'
	);
}

function wooagent_companion_render_pair_screen(): void {
	if ( ! current_user_can( 'manage_options' ) ) {
		return;
	}

	$prefill = isset( $_GET['code'] ) ? strtoupper( sanitize_text_field( wp_unslash( (string) $_GET['code'] ) ) ) : '';
	$notice  = isset( $_GET['notice'] ) ? sanitize_key( wp_unslash( (string) $_GET['notice'] ) ) : '';
	$device  = isset( $_GET['device'] ) ? sanitize_text_field( wp_unslash( (string) $_GET['device'] ) ) : '';

	?>
	<div class="wrap">
		<h1><?php esc_html_e( 'Pair a WooAgent device', 'wooagent-companion' ); ?></h1>

		<?php if ( $notice === 'approved' ) : ?>
			<div class="notice notice-success"><p>
				<?php
				printf(
					/* translators: %s: device name */
					esc_html__( 'Approved %s. The WooAgent OS daemon will pick this up on its next poll.', 'wooagent-companion' ),
					'<strong>' . esc_html( $device !== '' ? $device : 'this device' ) . '</strong>'
				);
				?>
			</p></div>
		<?php elseif ( $notice === 'rejected' ) : ?>
			<div class="notice notice-warning"><p>
				<?php esc_html_e( 'Pairing rejected. The WooAgent OS daemon will surface a clear error.', 'wooagent-companion' ); ?>
			</p></div>
		<?php elseif ( $notice === 'not_found' ) : ?>
			<div class="notice notice-error"><p>
				<?php esc_html_e( 'No pending pairing for that code. It may have already been used or expired — return to the WooAgent OS daemon and click Try again.', 'wooagent-companion' ); ?>
			</p></div>
		<?php endif; ?>

		<p><?php esc_html_e( 'Enter the pairing code shown by your WooAgent OS daemon and click Approve. The connection lasts until you revoke this device.', 'wooagent-companion' ); ?></p>

		<form method="post" action="<?php echo esc_url( admin_url( 'admin-post.php' ) ); ?>" style="margin-top:1em;">
			<?php wp_nonce_field( 'wooagent_pair' ); ?>

			<table class="form-table" role="presentation">
				<tr>
					<th scope="row">
						<label for="wooagent-pairing-code"><?php esc_html_e( 'Pairing code', 'wooagent-companion' ); ?></label>
					</th>
					<td>
						<input
							type="text"
							id="wooagent-pairing-code"
							name="code"
							value="<?php echo esc_attr( $prefill ); ?>"
							placeholder="WOOA-XXXX-XXXX"
							class="regular-text code"
							required
							pattern="WOOA-[A-Z0-9]{4}-[A-Z0-9]{4}"
							autofocus
						>
						<p class="description"><?php esc_html_e( 'Format: WOOA-XXXX-XXXX', 'wooagent-companion' ); ?></p>
					</td>
				</tr>
			</table>

			<p class="submit">
				<button type="submit" name="action" value="wooagent_pair_approve" class="button button-primary">
					<?php esc_html_e( 'Approve', 'wooagent-companion' ); ?>
				</button>
				<button type="submit" name="action" value="wooagent_pair_reject" class="button">
					<?php esc_html_e( 'Reject', 'wooagent-companion' ); ?>
				</button>
			</p>
		</form>
	</div>
	<?php
}

function wooagent_companion_handle_pair_approve(): void {
	wooagent_companion_handle_pair_action( 'approve' );
}

function wooagent_companion_handle_pair_reject(): void {
	wooagent_companion_handle_pair_action( 'reject' );
}

function wooagent_companion_handle_pair_action( string $action ): void {
	if ( ! current_user_can( 'manage_options' ) ) {
		wp_die( esc_html__( 'You are not allowed to pair devices.', 'wooagent-companion' ), '', array( 'response' => 403 ) );
	}
	check_admin_referer( 'wooagent_pair' );

	$code = strtoupper( sanitize_text_field( wp_unslash( $_POST['code'] ?? '' ) ) );
	if ( ! wooagent_companion_pair_valid_code( $code ) ) {
		wooagent_companion_pair_redirect( 'not_found' );
	}

	$ok    = false;
	$name  = '';
	$key   = wooagent_companion_pair_transient_key( $code );
	$data  = get_transient( $key );
	if ( is_array( $data ) ) {
		$name = (string) ( $data['device_name'] ?? '' );
	}

	if ( $action === 'approve' ) {
		$ok = wooagent_companion_pair_approve_code( $code );
	} else {
		$ok = wooagent_companion_pair_reject_code( $code );
	}

	if ( ! $ok ) {
		wooagent_companion_pair_redirect( 'not_found' );
	}

	wooagent_companion_pair_redirect( $action === 'approve' ? 'approved' : 'rejected', $name );
}

function wooagent_companion_pair_redirect( string $notice, string $device = '' ): void {
	$args = array( 'page' => 'wooagent', 'notice' => $notice );
	if ( $device !== '' ) {
		$args['device'] = $device;
	}
	wp_safe_redirect( add_query_arg( $args, admin_url( 'admin.php' ) ) );
	exit;
}
