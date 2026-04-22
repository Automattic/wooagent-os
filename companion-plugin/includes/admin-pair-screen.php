<?php
/**
 * wp-admin "WooAgent → Pair device" screen — v0.1 scaffold.
 *
 * Displays placeholder copy pointing operators at Application Password auth until the
 * device-pair token flow ships in v0.2.
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

add_action( 'admin_menu', 'wooagent_companion_register_admin_menu' );

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
	?>
	<div class="wrap">
		<h1><?php esc_html_e( 'Pair a WooAgent device', 'wooagent-companion' ); ?></h1>
		<div class="notice notice-info inline">
			<p>
				<strong><?php esc_html_e( 'Device pairing ships in WooAgent Companion v0.2.', 'wooagent-companion' ); ?></strong>
				<?php esc_html_e( 'Until then, connect the WooAgent daemon using a WordPress Application Password.', 'wooagent-companion' ); ?>
			</p>
		</div>
		<h2><?php esc_html_e( 'Authenticate with an Application Password (temporary)', 'wooagent-companion' ); ?></h2>
		<ol>
			<li><?php esc_html_e( 'Go to Users → Profile → Application Passwords.', 'wooagent-companion' ); ?></li>
			<li><?php esc_html_e( 'Create a password named, e.g., "wooagent-os".', 'wooagent-companion' ); ?></li>
			<li><?php esc_html_e( 'Paste it into the WooAgent daemon when prompted.', 'wooagent-companion' ); ?></li>
		</ol>
		<p>
			<a class="button" href="<?php echo esc_url( admin_url( 'profile.php#application-passwords-section' ) ); ?>">
				<?php esc_html_e( 'Open Application Passwords', 'wooagent-companion' ); ?>
			</a>
		</p>
	</div>
	<?php
}
