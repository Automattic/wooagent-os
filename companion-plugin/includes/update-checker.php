<?php
/**
 * Self-update wiring for the WooAgent Companion plugin.
 *
 * Points plugin-update-checker at the public Automattic/wooagent-os
 * repo and tells it to download the wooagent-companion.zip release
 * asset (not the GitHub-generated source zipball, which has the wrong
 * directory structure for a WordPress plugin install).
 *
 * Version comparison uses the Version: header inside the zip's main
 * plugin file — so daemon-only releases (where the companion bytes
 * didn't change) don't surface as plugin updates on customer stores.
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

require_once WOOAGENT_COMPANION_PATH . 'vendor/plugin-update-checker/plugin-update-checker.php';

use YahnisElsts\PluginUpdateChecker\v5\PucFactory;
use YahnisElsts\PluginUpdateChecker\v5p6\Vcs\Api;

$wooagent_companion_update_checker = PucFactory::buildUpdateChecker(
	'https://github.com/Automattic/wooagent-os/',
	WOOAGENT_COMPANION_PATH . 'wooagent-companion.php',
	'wooagent-companion'
);

$wooagent_companion_update_checker->getVcsApi()->enableReleaseAssets(
	'/^wooagent-companion\.zip$/',
	Api::REQUIRE_RELEASE_ASSETS
);
