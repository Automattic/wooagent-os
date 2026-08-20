#!/usr/bin/env bash
# Verify first-party Companion behavior, version metadata, updater wiring,
# and the exact shape of the locally built WordPress plugin archive.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PLUGIN_DIR="$REPO_ROOT/companion-plugin"
MAIN_FILE="$PLUGIN_DIR/wooagent-companion.php"
README_FILE="$PLUGIN_DIR/readme.txt"
ROOT_README="$REPO_ROOT/README.md"
UPDATE_FILE="$PLUGIN_DIR/includes/update-checker.php"
ZIP_FILE="$REPO_ROOT/build/wooagent-companion.zip"
INTEGRATION_MODE=false

if [ "${1:-}" = "--integration" ]; then
	INTEGRATION_MODE=true
elif [ "$#" -gt 0 ]; then
	echo "Usage: $0 [--integration]" >&2
	exit 2
fi

fail() {
	echo "FAIL: $1" >&2
	exit 1
}

require_text() {
	local file="$1"
	local text="$2"
	local label="$3"
	if ! rg -Fq "$text" "$file"; then
		fail "$label"
	fi
}

sha256_file() {
	local file="$1"
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$file" | awk '{print $1}'
	else
		sha256sum "$file" | awk '{print $1}'
	fi
}

echo "Linting first-party Companion PHP..."
while IFS= read -r file; do
	php -l "$file"
done < <(find "$PLUGIN_DIR" -path "$PLUGIN_DIR/vendor" -prune -o -type f -name '*.php' -print | sort)

echo "Running Companion regression tests..."
php "$REPO_ROOT/tests/companion-plugin/abilities-permissions-test.php"
php "$REPO_ROOT/tests/companion-plugin/debug-surface-test.php"
php "$REPO_ROOT/tests/companion-plugin/auth-rest-test.php"
php "$REPO_ROOT/tests/companion-plugin/auth-non-rest-test.php"
php "$REPO_ROOT/tests/companion-plugin/pairing-security-test.php"

if rg -n "wooagent_companion_register_debug_route|'/source'|'/selftest'|Reflection(Class|Function)|file_get_contents|set_error_handler" "$MAIN_FILE"; then
	fail "internal debug/source markers remain in the main plugin file"
fi

echo "Checking 0.4.1 version and updater metadata..."
require_text "$MAIN_FILE" "Version:           0.4.1" "plugin header is not 0.4.1"
require_text "$MAIN_FILE" "define( 'WOOAGENT_COMPANION_VERSION', '0.4.1' );" "plugin version constant is not 0.4.1"
require_text "$README_FILE" "Stable tag: 0.4.1" "plugin readme stable tag is not 0.4.1"
require_text "$ROOT_README" 'WOOAGENT_VERSION=v0.4.1' "root README version pin is not v0.4.1"
require_text "$UPDATE_FILE" "https://github.com/Automattic/wooagent-os/" "updater repository changed"
require_text "$UPDATE_FILE" "'wooagent-companion'" "updater slug changed"
require_text "$UPDATE_FILE" "'/^wooagent-companion\.zip$/'" "updater does not select the exact Companion zip"
require_text "$UPDATE_FILE" "Api::REQUIRE_RELEASE_ASSETS" "updater can fall back to the wrong-layout source zip"

echo "Building and inspecting Companion archive..."
bash "$REPO_ROOT/scripts/build-companion-plugin-zip.sh"
unzip -tq "$ZIP_FILE"

ENTRY_LIST="$(mktemp)"
INTEGRATION_BODY=""
cleanup() {
	rm -f "$ENTRY_LIST"
	if [ -n "$INTEGRATION_BODY" ]; then
		rm -f "$INTEGRATION_BODY"
	fi
}
trap cleanup EXIT

unzip -Z1 "$ZIP_FILE" > "$ENTRY_LIST"

TOP_LEVELS="$(cut -d/ -f1 "$ENTRY_LIST" | sort -u)"
if [ "$TOP_LEVELS" != "wooagent-companion" ]; then
	fail "archive must contain exactly one wooagent-companion/ top-level directory"
fi

while IFS= read -r entry; do
	case "$entry" in
		wooagent-companion/*) ;;
		*) fail "archive entry escapes the wooagent-companion/ directory: $entry" ;;
	esac

	case "$entry" in
		*/.git|*/.git/*|*.DS_Store|*.swp|*.swo|*/.env|*/.env.*|*/tests|*/tests/*|*.pem|*.key|*.crt|*.p12|*.pfx)
			fail "archive contains a forbidden development or credential artifact: $entry"
			;;
	esac
done < "$ENTRY_LIST"

required_entries=(
	"wooagent-companion/wooagent-companion.php"
	"wooagent-companion/readme.txt"
	"wooagent-companion/includes/abilities-products.php"
	"wooagent-companion/includes/abilities-orders.php"
	"wooagent-companion/includes/abilities-customers.php"
	"wooagent-companion/includes/pair-rest.php"
	"wooagent-companion/includes/auth-bridge.php"
	"wooagent-companion/includes/admin-pair-screen.php"
	"wooagent-companion/includes/update-checker.php"
	"wooagent-companion/vendor/plugin-update-checker/plugin-update-checker.php"
)

for entry in "${required_entries[@]}"; do
	if ! rg -Fxq "$entry" "$ENTRY_LIST"; then
		fail "archive is missing required entry: $entry"
	fi
done

FIRST_ZIP_SHA256="$(sha256_file "$ZIP_FILE")"
sleep 2
bash "$REPO_ROOT/scripts/build-companion-plugin-zip.sh"
SECOND_ZIP_SHA256="$(sha256_file "$ZIP_FILE")"
if [ "$FIRST_ZIP_SHA256" != "$SECOND_ZIP_SHA256" ]; then
	fail "identical source produced different Companion zip hashes"
fi

if $INTEGRATION_MODE; then
	if [ -z "${WOOAGENT_TEST_SITE_URL:-}" ]; then
		fail "--integration requires WOOAGENT_TEST_SITE_URL"
	fi

	INTEGRATION_BODY="$(mktemp)"
	SITE_URL="${WOOAGENT_TEST_SITE_URL%/}"

	expect_status() {
		local actual="$1"
		local expected="$2"
		local label="$3"
		case ",$expected," in
			*,"$actual",*) ;;
			*) fail "$label returned HTTP $actual; expected one of $expected" ;;
		esac
	}

	http_status="$(curl -sS -o "$INTEGRATION_BODY" -w '%{http_code}' "$SITE_URL/wp-json/wooagent-companion/v1/source?class=wpdb")"
	expect_status "$http_status" "404" "removed source route"

	http_status="$(curl -sS -o "$INTEGRATION_BODY" -w '%{http_code}' "$SITE_URL/wp-json/wooagent-companion/v1/selftest")"
	expect_status "$http_status" "404" "removed selftest route"

	http_status="$(curl -sS -o "$INTEGRATION_BODY" -w '%{http_code}' "$SITE_URL/wp-json/wooagent/v1/pair/poll?code=invalid")"
	expect_status "$http_status" "400" "malformed pairing poll"

	printf -v unknown_code 'WOOA-%04X-%04X' "$RANDOM" "$RANDOM"
	http_status="$(curl -sS -o "$INTEGRATION_BODY" -w '%{http_code}' "$SITE_URL/wp-json/wooagent/v1/pair/poll?code=$unknown_code")"
	expect_status "$http_status" "404" "unknown pairing poll"

	http_status="$(curl -sS -o "$INTEGRATION_BODY" -w '%{http_code}' -H 'Authorization: Bearer fixture-invalid-device-token' "$SITE_URL/wp-json/wooagent/v1/devices/me")"
	expect_status "$http_status" "401,403" "invalid bearer probe"

	if [ -n "${WOOAGENT_TEST_DEVICE_TOKEN:-}" ]; then
		http_status="$(curl -sS -o "$INTEGRATION_BODY" -w '%{http_code}' -H "Authorization: Bearer $WOOAGENT_TEST_DEVICE_TOKEN" "$SITE_URL/wp-json/wooagent/v1/devices/me")"
		expect_status "$http_status" "200" "valid bearer probe"
	fi

	echo "Read-only integration checks passed."
fi

ZIP_SHA256="$SECOND_ZIP_SHA256"
ZIP_BYTES="$(wc -c < "$ZIP_FILE" | tr -d ' ')"

echo "Verified $ZIP_FILE"
echo "Bytes: $ZIP_BYTES"
echo "SHA-256: $ZIP_SHA256"
