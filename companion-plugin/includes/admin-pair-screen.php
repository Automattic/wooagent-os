<?php
/**
 * wp-admin "WooAgent → Pair device" screen.
 *
 * Two display modes driven by the wooagent_devices wp_option:
 *   - At least one device paired → render a read-only "Currently paired"
 *     list (name + paired-at + Remove). The pair form is collapsed inside
 *     a <details> labeled "Pair a different device".
 *   - No devices paired → render the pair form prominently. This is the
 *     first-run state.
 *
 * Operator types the pairing code shown by the WooAgent OS daemon and
 * clicks Approve (or Reject). Approval mints a device token and stashes
 * the plaintext on the transient so the daemon's next /poll picks it up.
 *
 * The WooAgent OS daemon deep-links here with `?page=wooagent&code=XXX`,
 * which prefills the input.
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

add_action( 'admin_menu', 'wooagent_companion_register_admin_menu' );
add_action( 'admin_post_wooagent_pair_approve', 'wooagent_companion_handle_pair_approve' );
add_action( 'admin_post_wooagent_pair_reject', 'wooagent_companion_handle_pair_reject' );
add_action( 'admin_post_wooagent_pair_admin_remove', 'wooagent_companion_handle_admin_remove' );

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

	$devices = get_option( WOOAGENT_DEVICES_OPTION, array() );
	if ( ! is_array( $devices ) ) {
		$devices = array();
	}
	$has_devices = ! empty( $devices );

	?>
	<div class="wrap">
		<h1><?php esc_html_e( 'WooAgent device pairing', 'wooagent-companion' ); ?></h1>

		<?php if ( $notice === 'approved' ) : ?>
			<div class="notice notice-success is-dismissible"><p>
				<?php
				printf(
					/* translators: %s: device name */
					esc_html__( 'Approved %s. The WooAgent OS daemon will pick this up on its next poll.', 'wooagent-companion' ),
					'<strong>' . esc_html( $device !== '' ? $device : 'this device' ) . '</strong>'
				);
				?>
			</p></div>
		<?php elseif ( $notice === 'rejected' ) : ?>
			<div class="notice notice-warning is-dismissible"><p>
				<?php esc_html_e( 'Pairing rejected. The WooAgent OS daemon will surface a clear error.', 'wooagent-companion' ); ?>
			</p></div>
		<?php elseif ( $notice === 'not_found' ) : ?>
			<div class="notice notice-error is-dismissible"><p>
				<?php esc_html_e( 'No pending pairing for that code. It may have already been used or expired — return to the WooAgent OS daemon and click Try again.', 'wooagent-companion' ); ?>
			</p></div>
		<?php elseif ( $notice === 'removed' ) : ?>
			<div class="notice notice-success is-dismissible"><p>
				<?php
				printf(
					/* translators: %s: device name */
					esc_html__( 'Removed %s. That daemon can no longer call the store; the operator can re-pair from the WooAgent OS UI.', 'wooagent-companion' ),
					'<strong>' . esc_html( $device !== '' ? $device : 'this device' ) . '</strong>'
				);
				?>
			</p></div>
		<?php endif; ?>

		<?php if ( $has_devices ) : ?>
			<h2><?php esc_html_e( 'Currently paired', 'wooagent-companion' ); ?></h2>
			<table class="wp-list-table widefat striped" style="max-width:760px;">
				<thead>
					<tr>
						<th scope="col"><?php esc_html_e( 'Device', 'wooagent-companion' ); ?></th>
						<th scope="col"><?php esc_html_e( 'Paired', 'wooagent-companion' ); ?></th>
						<th scope="col" style="width:1%;"></th>
					</tr>
				</thead>
				<tbody>
					<?php foreach ( $devices as $d ) :
						$id   = isset( $d['id'] ) ? (string) $d['id'] : '';
						$name = isset( $d['name'] ) ? (string) $d['name'] : __( 'Unnamed device', 'wooagent-companion' );
						$when = isset( $d['created_at'] ) ? wooagent_companion_format_paired_at( (string) $d['created_at'] ) : '—';
					?>
						<tr>
							<td><strong><?php echo esc_html( $name ); ?></strong></td>
							<td><?php echo esc_html( $when ); ?></td>
							<td>
								<form method="post" action="<?php echo esc_url( admin_url( 'admin-post.php' ) ); ?>" style="margin:0;">
									<?php wp_nonce_field( 'wooagent_pair_admin_remove' ); ?>
									<input type="hidden" name="action" value="wooagent_pair_admin_remove">
									<input type="hidden" name="device_id" value="<?php echo esc_attr( $id ); ?>">
									<input type="hidden" name="device_name" value="<?php echo esc_attr( $name ); ?>">
									<button
										type="submit"
										class="button button-link-delete"
										onclick="return confirm(<?php echo wp_json_encode( __( 'Remove this device? Its daemon will lose access until re-paired.', 'wooagent-companion' ) ); ?>);"
									><?php esc_html_e( 'Remove', 'wooagent-companion' ); ?></button>
								</form>
							</td>
						</tr>
					<?php endforeach; ?>
				</tbody>
			</table>
		<?php endif; ?>

		<?php
		// Form is the headline content when nothing's paired; collapsed
		// behind a <details> when one or more devices already exist.
		if ( $has_devices ) {
			echo '<details style="margin-top:2em;"><summary style="cursor:pointer;font-weight:600;font-size:1.1em;">'
				. esc_html__( 'Pair a different device', 'wooagent-companion' )
				. '</summary>';
		} else {
			echo '<p>' . esc_html__( 'Enter the pairing code shown by your WooAgent OS daemon and click Approve. The connection lasts until you remove this device.', 'wooagent-companion' ) . '</p>';
		}
		?>

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
							<?php echo $has_devices ? '' : 'autofocus'; ?>
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

		<?php if ( $has_devices ) : ?>
			</details>
		<?php endif; ?>
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

	$name  = '';
	$key   = wooagent_companion_pair_transient_key( $code );
	$data  = get_transient( $key );
	if ( is_array( $data ) ) {
		$name = (string) ( $data['device_name'] ?? '' );
	}

	$ok = $action === 'approve'
		? wooagent_companion_pair_approve_code( $code )
		: wooagent_companion_pair_reject_code( $code );

	if ( ! $ok ) {
		wooagent_companion_pair_redirect( 'not_found' );
	}

	wooagent_companion_pair_redirect( $action === 'approve' ? 'approved' : 'rejected', $name );
}

/**
 * wp-admin "Remove" button handler. Deletes the device from
 * wooagent_devices by id without requiring a bearer token — the only
 * way to recover from a lost daemon (e.g., daemon DB wiped, no token to
 * present to /pair/revoke). Capability + nonce gate access.
 */
function wooagent_companion_handle_admin_remove(): void {
	if ( ! current_user_can( 'manage_options' ) ) {
		wp_die( esc_html__( 'You are not allowed to remove devices.', 'wooagent-companion' ), '', array( 'response' => 403 ) );
	}
	check_admin_referer( 'wooagent_pair_admin_remove' );

	$device_id   = sanitize_text_field( wp_unslash( $_POST['device_id'] ?? '' ) );
	$device_name = sanitize_text_field( wp_unslash( $_POST['device_name'] ?? '' ) );

	$devices = get_option( WOOAGENT_DEVICES_OPTION, array() );
	if ( ! is_array( $devices ) ) {
		$devices = array();
	}
	$kept = array_values(
		array_filter(
			$devices,
			static function ( $d ) use ( $device_id ) {
				return ! ( isset( $d['id'] ) && $d['id'] === $device_id );
			}
		)
	);
	update_option( WOOAGENT_DEVICES_OPTION, $kept );

	wooagent_companion_pair_redirect( 'removed', $device_name );
}

function wooagent_companion_pair_redirect( string $notice, string $device = '' ): void {
	$args = array( 'page' => 'wooagent', 'notice' => $notice );
	if ( $device !== '' ) {
		$args['device'] = $device;
	}
	wp_safe_redirect( add_query_arg( $args, admin_url( 'admin.php' ) ) );
	exit;
}

/**
 * Format a device's created_at (ISO-8601 UTC) for the wp-admin device
 * list. Uses WP's site timezone + date format so it matches every other
 * timestamp in wp-admin. Falls back to the raw string on parse error.
 */
function wooagent_companion_format_paired_at( string $iso ): string {
	$ts = strtotime( $iso );
	if ( $ts === false ) {
		return $iso;
	}
	return wp_date( get_option( 'date_format', 'M j, Y' ) . ' ' . get_option( 'time_format', 'g:i a' ), $ts );
}
