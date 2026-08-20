<?php
/**
 * Self-update wiring for the WooAgent Companion plugin.
 *
 * Points plugin-update-checker at the public Automattic/wooagent-os
 * repo and tells it to download the wooagent-companion.zip release
 * asset (not the GitHub-generated source zipball, which has the wrong
 * directory structure for a WordPress plugin install).
 *
 * PUC derives the available version from the GitHub release tag because the
 * main plugin file lives in a repository subdirectory. Release tags and the
 * Companion Version/Stable tag must therefore stay aligned. The exact asset
 * filter below prevents daemon archives or GitHub's source zip from being
 * offered to WordPress as the plugin package.
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
