# Companion Plugin 0.4.1 Security Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a review-ready WooAgent Companion 0.4.1 change set that removes the internal source/debug REST surface, makes device authentication fail closed, resists pairing-state abuse, and produces a verified update zip.

**Architecture:** Keep the existing WordPress REST and Abilities API architecture. Add a dependency-free PHP regression harness around first-party Companion code, centralize device-record validity in the pairing layer, and make the release script package an explicit allow-list. The daemon/UI protocol remains unchanged; the only compatibility break is that legacy device records without a valid `paired_by_user_id` must pair again.

**Tech Stack:** PHP 7.4-compatible WordPress plugin code, WordPress REST API and Abilities API, WooCommerce, Bash, `php -l`, `zip`/`unzip`, and the existing plugin-update-checker v5.6 integration.

## Global Constraints

- Work on `codex/companion-plugin-0.4.1-hardening`.
- First-party Companion code only; do not edit `companion-plugin/vendor/`.
- Keep PHP syntax compatible with the declared minimum PHP 7.4.
- Do not alter the daemon/UI pairing protocol or add product features.
- Never log or print bearer tokens, token hashes, pairing tokens, API keys, or other credentials.
- Do not run mutation checks against a live store. Any staging smoke test must use disposable fixtures and separate explicit approval.
- Do not push, tag, publish a GitHub release, or upload the zip in this plan.
- Preserve existing ability names, input/output schemas, and capability gates unless a test proves a security defect.
- Treat a device record as valid only when its token hash matches and its stored approving WordPress user still exists. Records missing that identity are rejected and the daemon's existing `/devices/me` staleness flow will require re-pairing.

---

## Task 1: Add a dependency-free Companion security regression harness

**Files:**

- Create: `tests/companion-plugin/bootstrap.php`
- Create: `tests/companion-plugin/abilities-permissions-test.php`

- [ ] **Step 1: Create the test bootstrap with WordPress REST and storage doubles**

Implement only the WordPress surface needed by the Companion files. The bootstrap must provide:

```php
<?php

define( 'WOOAGENT_TEST_ROOT', dirname( __DIR__, 2 ) );
define( 'ABSPATH', WOOAGENT_TEST_ROOT . '/' );

$GLOBALS['wooagent_test_options']    = array();
$GLOBALS['wooagent_test_transients'] = array();
$GLOBALS['wooagent_test_users']      = array();
$GLOBALS['wooagent_test_routes']     = array();
$GLOBALS['wooagent_test_abilities']  = array();
$GLOBALS['wooagent_test_caps']       = array();

final class WP_Error {
	private $code;
	private $message;
	private $data;

	public function __construct( $code = '', $message = '', $data = null ) {
		$this->code    = $code;
		$this->message = $message;
		$this->data    = $data;
	}

	public function get_error_code() { return $this->code; }
	public function get_error_message() { return $this->message; }
	public function get_error_data() { return $this->data; }
}

final class WP_REST_Request {
	private $params;
	private $headers;

	public function __construct( array $params = array(), array $headers = array() ) {
		$this->params  = $params;
		$this->headers = array_change_key_case( $headers, CASE_LOWER );
	}

	public function get_param( $name ) { return $this->params[ $name ] ?? null; }
	public function get_header( $name ) { return $this->headers[ strtolower( $name ) ] ?? ''; }
}

final class WP_REST_Response {
	private $data;
	public function __construct( $data ) { $this->data = $data; }
	public function get_data() { return $this->data; }
}
```

Add small stubs for `add_action`, `add_filter`, `register_rest_route`, `wp_register_ability`, `get_option`, `update_option`, `get_transient`, `set_transient`, `get_user_by`, `current_user_can`, `sanitize_text_field`, `rest_ensure_response`, `nocache_headers`, `wp_generate_uuid4`, `wp_generate_password`, `get_current_user_id`, `__`, and `is_wp_error`. Add assertion helpers that throw `RuntimeException` and a test runner that prints only test names and pass/fail state—never values that may contain credentials.

- [ ] **Step 2: Add baseline ability-permission tests**

Load the three ability files, capture registrations, and assert:

```php
$expected = array(
	'wooagent-products/list',
	'wooagent-products/get',
	'wooagent-products/list-categories',
	'wooagent-products/update',
	'wooagent-products/variations-list',
	'wooagent-orders/list',
	'wooagent-orders/get',
	'wooagent-orders/add-note',
	'wooagent-customers/get',
);
```

For every registration, require a callable permission callback and `additionalProperties => false` on its input schema. With no capabilities, every permission callback must return false. With `edit_products`, product update must pass. With `edit_shop_orders`, order-note mutation must pass. With each existing read capability, the corresponding read abilities must pass.

- [ ] **Step 3: Run the baseline test**

Run:

```bash
php tests/companion-plugin/abilities-permissions-test.php
```

Expected: all nine registrations and permission tests pass against 0.4.0. Fix only the test doubles if the harness is incomplete; do not change production ability code.

- [ ] **Step 4: Commit the harness**

```bash
git add tests/companion-plugin/bootstrap.php tests/companion-plugin/abilities-permissions-test.php
git commit -m "test(companion): add security regression harness"
```

---

## Task 2: Remove debug REST endpoints and persistent diagnostics

**Files:**

- Create: `tests/companion-plugin/debug-surface-test.php`
- Modify: `companion-plugin/wooagent-companion.php`

- [ ] **Step 1: Write the failing debug-surface regression test**

Read only `companion-plugin/wooagent-companion.php` and fail if any of these production markers remain:

```php
$forbidden = array(
	'wooagent_companion_register_debug_route',
	"'/source'",
	"'/selftest'",
	'ReflectionClass',
	'ReflectionFunction',
	'file_get_contents',
	'set_error_handler',
	'wooagent_companion_register_results',
	'wooagent_companion_register_errors',
);
```

Also assert that the three production registration functions are still called: products, orders, and customers.

- [ ] **Step 2: Confirm the test fails for the exposed surface**

Run:

```bash
php tests/companion-plugin/debug-surface-test.php
```

Expected: FAIL, identifying the forbidden debug route/diagnostic markers without echoing source content.

- [ ] **Step 3: Delete the debug endpoints and dead diagnostic writes**

In `wooagent_companion_register_categories()`, register the three categories directly without collecting return values or updating `wooagent_companion_category_results`.

In `wooagent_companion_register_abilities()`, keep only:

```php
function wooagent_companion_register_abilities(): void {
	wooagent_companion_register_product_abilities();
	wooagent_companion_register_order_abilities();
	wooagent_companion_register_customer_abilities();
}
```

Remove `wooagent_companion_describe_value()`, the `rest_api_init` debug hook, `/source`, `/selftest`, reflection, source reads, temporary error capture, and the `wooagent_companion_*register*` option writes. Do not replace them with feature flags or environment checks.

- [ ] **Step 4: Run focused checks**

Run:

```bash
php tests/companion-plugin/debug-surface-test.php
php -l companion-plugin/wooagent-companion.php
```

Expected: both pass. On an installed store, both removed paths now fall through WordPress route matching and return 404.

- [ ] **Step 5: Commit the debug-surface removal**

```bash
git add tests/companion-plugin/debug-surface-test.php companion-plugin/wooagent-companion.php
git commit -m "fix(companion): remove internal debug REST surface"
```

---

## Task 3: Make bearer authentication REST-only and fail closed

**Files:**

- Create: `tests/companion-plugin/auth-rest-test.php`
- Create: `tests/companion-plugin/auth-non-rest-test.php`
- Modify: `companion-plugin/includes/pair-rest.php`
- Modify: `companion-plugin/includes/auth-bridge.php`

- [ ] **Step 1: Write failing REST authentication tests**

In `auth-rest-test.php`, define `REST_REQUEST` as true before loading production files. Cover:

1. An already-authenticated cookie/Application Password user is returned unchanged.
2. A matching bearer record with `paired_by_user_id` pointing to an existing user resolves to that user.
3. An unknown bearer returns the incoming false value.
4. A matching legacy record without `paired_by_user_id` returns false.
5. A matching record whose approving user was deleted returns false.
6. `wooagent_companion_find_device_by_bearer()` returns null for legacy/deleted-user records so `/devices/me` and `/pair/revoke` reject them too.

Use a fixed synthetic test token stored only inside the test process. Assertions must compare IDs/statuses, not print the token.

- [ ] **Step 2: Write the failing non-REST test**

In a separate PHP process/file, define `REST_REQUEST` as false, install a valid device record and Authorization header, and assert:

```php
wooagent_test_expect_same(
	false,
	wooagent_companion_resolve_bearer_user( false ),
	'Bearer authentication must not run outside REST requests.'
);
```

- [ ] **Step 3: Confirm the security tests fail on 0.4.0 behavior**

Run:

```bash
php tests/companion-plugin/auth-rest-test.php
php tests/companion-plugin/auth-non-rest-test.php
```

Expected: legacy/deleted-user and non-REST assertions fail.

- [ ] **Step 4: Add a shared fail-closed device-user validator**

In `pair-rest.php`, add:

```php
function wooagent_companion_device_user_id( array $device ): int {
	$user_id = isset( $device['paired_by_user_id'] ) ? (int) $device['paired_by_user_id'] : 0;
	if ( $user_id <= 0 || ! get_user_by( 'id', $user_id ) ) {
		return 0;
	}
	return $user_id;
}
```

Require `wooagent_companion_find_device_by_bearer()` to return a record only when both the hash matches and `wooagent_companion_device_user_id()` returns a positive ID. This aligns the staleness probe/revoke routes with the auth bridge.

- [ ] **Step 5: Scope the bridge and remove the administrator fallback**

At the beginning of `wooagent_companion_resolve_bearer_user()`, after preserving an already-authenticated user, add:

```php
if ( ! defined( 'REST_REQUEST' ) || ! REST_REQUEST ) {
	return $user_id;
}
```

When a token hash matches, return `wooagent_companion_device_user_id( $device )` when positive; otherwise return the incoming `$user_id`. Delete `wooagent_companion_default_admin_user_id()` and update the file-level security comments to describe fail-closed legacy behavior.

- [ ] **Step 6: Run focused and regression tests**

Run:

```bash
php tests/companion-plugin/auth-rest-test.php
php tests/companion-plugin/auth-non-rest-test.php
php tests/companion-plugin/abilities-permissions-test.php
php -l companion-plugin/includes/pair-rest.php
php -l companion-plugin/includes/auth-bridge.php
```

Expected: all pass. Compatibility note for review: pre-`paired_by_user_id` records are intentionally invalid; the daemon receives 401/403 from `/devices/me`, marks the store unpaired, drops its local token reference, and the operator pairs again.

- [ ] **Step 7: Commit the auth hardening**

```bash
git add tests/companion-plugin/auth-rest-test.php tests/companion-plugin/auth-non-rest-test.php companion-plugin/includes/pair-rest.php companion-plugin/includes/auth-bridge.php
git commit -m "fix(companion): fail closed for device bearer auth"
```

---

## Task 4: Bound and protect public pairing endpoints

**Files:**

- Create: `tests/companion-plugin/pairing-security-test.php`
- Modify: `companion-plugin/includes/pair-rest.php`

- [ ] **Step 1: Write failing pairing input/state/rate tests**

Test these behaviors through `WP_REST_Request` doubles and captured route definitions:

1. Invalid codes return `invalid_code` with HTTP 400.
2. `device_name` is sanitized, defaults to `wooagent-device`, and values over 100 bytes return HTTP 400.
3. Re-posting an existing pending code returns the original pending record without changing `device_name`, `expires_at`, or its remaining TTL.
4. Re-posting an approved, delivered, or rejected code returns `pairing_code_in_use` with HTTP 409 and does not overwrite the transient.
5. The 11th `/pair/request` attempt from one IP in a fixed 60-second window returns `pairing_rate_limited` with HTTP 429.
6. The 91st `/pair/poll` attempt from one IP in a fixed 60-second window returns HTTP 429; 90 permits the current UI's 2-second polling with headroom for multiple views.
7. Rate buckets for request and poll are separate and store only a SHA-256-derived IP identifier, never the raw address.
8. Route definitions use concrete permission callbacks and declare `code`/`device_name` schemas; no public route uses `__return_true`.
9. Malformed stored device records cannot trigger a `TypeError` during revoke filtering.

- [ ] **Step 2: Confirm the tests fail before production changes**

Run:

```bash
php tests/companion-plugin/pairing-security-test.php
```

Expected: FAIL because 0.4.0 overwrites pairing transients, lacks bounds/rate callbacks, and has no route argument schemas.

- [ ] **Step 3: Add bounded route arguments**

Add PHP 7.4-compatible constants:

```php
const WOOAGENT_PAIR_DEVICE_NAME_MAX_BYTES = 100;
const WOOAGENT_PAIR_RATE_WINDOW_SECONDS   = 60;
const WOOAGENT_PAIR_REQUEST_RATE_LIMIT    = 10;
const WOOAGENT_PAIR_POLL_RATE_LIMIT       = 90;
```

For `/pair/request`, register `code` as required string and `device_name` as optional string with `maxLength => 100`. For `/pair/poll`, register `code` as required string. Keep defensive validation in callbacks because tests call them directly and WordPress filters can invoke callbacks outside normal schema validation.

- [ ] **Step 4: Implement fixed-window rate permission callbacks**

Add separate permission callbacks that call a shared helper:

```php
function wooagent_companion_pair_request_permission() {
	return wooagent_companion_pair_rate_limit( 'request', WOOAGENT_PAIR_REQUEST_RATE_LIMIT );
}

function wooagent_companion_pair_poll_permission() {
	return wooagent_companion_pair_rate_limit( 'poll', WOOAGENT_PAIR_POLL_RATE_LIMIT );
}
```

The shared helper must:

- derive the bucket key from the action plus a truncated SHA-256 of `REMOTE_ADDR` (use `unknown` if absent);
- store `{count, reset_at}` in a transient;
- preserve the original `reset_at` while incrementing, so continuous polling does not create an accidental sliding-window lockout;
- return `WP_Error( 'pairing_rate_limited', ..., array( 'status' => 429 ) )` after the limit;
- never include the raw IP, pairing code, bearer, or hash in errors/logs.

- [ ] **Step 5: Make pending registration non-destructive and bounded**

Normalize the code with `strtoupper( sanitize_text_field() )`, sanitize `device_name`, and reject names whose byte length exceeds the constant. Before writing:

```php
$key      = wooagent_companion_pair_transient_key( $code );
$existing = get_transient( $key );

if ( is_array( $existing ) ) {
	if ( ( $existing['status'] ?? '' ) === 'pending' ) {
		return rest_ensure_response(
			array(
				'status'     => 'pending',
				'expires_at' => gmdate( 'c', (int) ( $existing['expires_at'] ?? time() ) ),
			)
		);
	}
	return new WP_Error(
		'pairing_code_in_use',
		__( 'pairing code is already in use', 'wooagent-companion' ),
		array( 'status' => 409 )
	);
}
```

Only a previously unseen code may create a pending transient. Update comments that currently claim re-posting overwrites state.

- [ ] **Step 6: Defensively handle malformed stored device rows**

In the revoke filter, require `is_array( $d )`, a scalar/string-castable `token_hash`, and cast the stored hash to string before `hash_equals()`. Non-array/malformed records stay in the option for admin cleanup and must not crash the request.

- [ ] **Step 7: Run pairing, auth, and syntax checks**

Run:

```bash
php tests/companion-plugin/pairing-security-test.php
php tests/companion-plugin/auth-rest-test.php
php tests/companion-plugin/auth-non-rest-test.php
php -l companion-plugin/includes/pair-rest.php
```

Expected: all pass.

- [ ] **Step 8: Commit pairing hardening**

```bash
git add tests/companion-plugin/pairing-security-test.php companion-plugin/includes/pair-rest.php
git commit -m "fix(companion): harden public pairing endpoints"
```

---

## Task 5: Bump 0.4.1 and harden the release artifact

**Files:**

- Create: `scripts/verify-companion-plugin.sh`
- Modify: `scripts/build-companion-plugin-zip.sh`
- Modify: `companion-plugin/wooagent-companion.php`
- Modify: `companion-plugin/readme.txt`
- Modify: `README.md`

- [ ] **Step 1: Create a verifier that initially fails version consistency**

The script must use `set -euo pipefail`, resolve `REPO_ROOT`, and then:

1. `php -l` every first-party PHP file under `companion-plugin/`, excluding `vendor/`.
2. Run all five PHP regression files.
3. Reject debug/source markers in `wooagent-companion.php`.
4. Assert `Version: 0.4.1`, `WOOAGENT_COMPANION_VERSION` 0.4.1, `Stable tag: 0.4.1`, and README pin `WOOAGENT_VERSION=v0.4.1`.
5. Assert the updater still points to `https://github.com/Automattic/wooagent-os/`, slug `wooagent-companion`, and `enableReleaseAssets()`.
6. Run `scripts/build-companion-plugin-zip.sh`.
7. Run `unzip -t build/wooagent-companion.zip`.
8. Require exactly one top-level directory named `wooagent-companion/`.
9. Reject archive entries containing `.git`, `.DS_Store`, editor swap files, `.env`, tests, private keys/certificates, or any path outside the top-level slug.
10. Confirm the main plugin file, all seven first-party include files, readme, and vendored updater entrypoint exist in the archive.

Support an optional read-only `--integration` mode. It must require `WOOAGENT_TEST_SITE_URL` and use `curl` to assert `/wooagent-companion/v1/source` and `/selftest` return 404, a malformed poll code returns 400, a random well-formed unknown code returns 404, and an invalid synthetic bearer returns 401/403 from `/wooagent/v1/devices/me`. If `WOOAGENT_TEST_DEVICE_TOKEN` is set, use it only from the environment to assert `/devices/me` returns 200; never echo it or enable shell tracing. This mode must not create pairing requests or mutate products, orders, customers, devices, or options.

Run:

```bash
bash scripts/verify-companion-plugin.sh
```

Expected: FAIL only at the 0.4.1 version checks before the bump.

- [ ] **Step 2: Change the build script to an explicit shipping allow-list**

Replace `cp -R "$SRC_DIR/."` with exact copies of:

```bash
cp "$SRC_DIR/wooagent-companion.php" "$STAGE_DIR/wooagent-companion/"
cp "$SRC_DIR/readme.txt" "$STAGE_DIR/wooagent-companion/"
cp -R "$SRC_DIR/includes" "$STAGE_DIR/wooagent-companion/"
cp -R "$SRC_DIR/vendor" "$STAGE_DIR/wooagent-companion/"
```

Use `mktemp -d "$OUT_DIR/companion-plugin-stage.XXXXXX"` for staging and a `trap` to remove that exact directory on success or failure. Remove only the explicit output zip with `rm -f`. This prevents future local files from being silently shipped.

- [ ] **Step 3: Apply the full 0.4.1 version bump**

Update:

- plugin header `Version` to `0.4.1`;
- `WOOAGENT_COMPANION_VERSION` to `0.4.1`;
- `readme.txt` `Stable tag` to `0.4.1`;
- README version pin to `WOOAGENT_VERSION=v0.4.1`.

Add the 0.4.1 changelog:

```text
= 0.4.1 =
* Removes internal diagnostic REST endpoints, including the source-inspection endpoint.
* Restricts device bearer authentication to REST requests and valid approving WordPress users. Legacy pairings without an approver identity must pair again.
* Adds pairing input bounds, fixed-window rate limits, and non-destructive retry behavior.
* Hardens plugin packaging and adds repeatable security/release verification.
```

Do not change the vendored updater version or release repository.

- [ ] **Step 4: Run the complete verifier**

Run:

```bash
bash scripts/verify-companion-plugin.sh
```

Expected: PASS and a local `build/wooagent-companion.zip`. Record its byte size and SHA-256 in the audit evidence, but do not commit the generated zip and do not publish it.

- [ ] **Step 5: Commit the version/package changes**

```bash
git add scripts/verify-companion-plugin.sh scripts/build-companion-plugin-zip.sh companion-plugin/wooagent-companion.php companion-plugin/readme.txt README.md
git commit -m "chore(companion): prepare verified 0.4.1 package"
```

---

## Task 6: Record the targeted audit and perform final review

**Files:**

- Create: `docs/superpowers/audits/2026-08-20-companion-plugin-0.4.1-security-audit.md`
- Review: `companion-plugin/wooagent-companion.php`
- Review: `companion-plugin/includes/pair-rest.php`
- Review: `companion-plugin/includes/auth-bridge.php`
- Review: `companion-plugin/includes/admin-pair-screen.php`
- Review: `companion-plugin/includes/abilities-products.php`
- Review: `companion-plugin/includes/abilities-orders.php`
- Review: `companion-plugin/includes/abilities-customers.php`
- Review: `companion-plugin/includes/update-checker.php`
- Review: `scripts/build-companion-plugin-zip.sh`
- Review: `scripts/verify-companion-plugin.sh`

- [ ] **Step 1: Write the evidence-based audit matrix**

Record each boundary with status, evidence, fix, and regression coverage:

| Boundary | Expected result |
|---|---|
| Debug `/source` and `/selftest` | Removed; WordPress returns 404 |
| Unauthenticated pairing request/poll | Schema bounded and fixed-window rate limited |
| Duplicate pending code | Idempotent without TTL/name mutation |
| Duplicate terminal code | HTTP 409 without state overwrite |
| Unknown/revoked bearer | 401/403; no identity resolution |
| Legacy/deleted-approver bearer | 401/403 and re-pair required |
| Valid bearer on REST | Resolves only to its existing approving user |
| Valid bearer outside REST | Ignored |
| Admin approve/reject/remove | Existing `manage_options` + nonce gates retained |
| Product/order/customer reads | Existing capability gates retained; PII only to authorized operators |
| Product/order mutations | Existing `edit_products`/`edit_shop_orders` gates retained |
| Updater | Canonical repo, slug, release assets, version header preserved |
| Zip | Allow-listed contents, one top-level slug, archive tests pass |

Classify the three confirmed findings:

1. Arbitrary first-party/dependency source disclosure through the admin-only debug route — fixed by removal.
2. Bearer bridge authentication outside REST and fail-open first-admin fallback — fixed by REST scoping and valid-approver enforcement.
3. Public pairing state overwrite/unbounded request pressure — fixed by input bounds, non-destructive idempotency, and rate limits.

Record reviewed-with-no-code-change conclusions for ability permissions, admin nonces/capabilities, updater repository/asset selection, and vendor scope. Do not include secrets, real tokens, real pairing codes, raw IPs, customer PII, or local absolute paths.

- [ ] **Step 2: Run the full verification from a clean shell**

Run:

```bash
bash scripts/verify-companion-plugin.sh
git diff --check trunk...HEAD
git status --short
```

Expected: verifier passes, `git diff --check` is silent, and status shows only the audit document before its commit (plus ignored `build/` output).

- [ ] **Step 3: Inspect the release diff for scope and accidental disclosure**

Run:

```bash
git diff --stat trunk...HEAD
git diff trunk...HEAD -- companion-plugin scripts/build-companion-plugin-zip.sh scripts/verify-companion-plugin.sh README.md
rg -n "sk-ant-[A-Za-z0-9_-]{20,}|AKIA[0-9A-Z]{16}|BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY" companion-plugin tests scripts README.md
```

Expected: changes are limited to the approved Companion hardening/test/docs surface, and the secret-pattern scan returns no matches.

- [ ] **Step 4: Commit the audit evidence**

Because `docs/superpowers/` is intentionally ignored, force-add only this exact file:

```bash
git add -f docs/superpowers/audits/2026-08-20-companion-plugin-0.4.1-security-audit.md
git commit -m "docs(companion): record 0.4.1 security audit"
```

- [ ] **Step 5: Present the review-ready handoff**

Report:

- branch and commit list;
- the three fixed findings and the reviewed/no-change areas;
- all automated test/build results;
- local zip path, size, and SHA-256;
- the legacy-pairing re-pair impact;
- that credential rotation is unrelated and remains optional housekeeping;
- that no push, tag, release, or store mutation occurred.

Stop here. Request a separate explicit approval before pushing, tagging `v0.4.1`, publishing the GitHub release, or installing the zip on a store.
