# WooAgent Companion 0.4.1 Targeted Security Audit

**Date:** 2026-08-20

**Status:** Review-ready; not published or installed

**Branch:** `codex/companion-plugin-0.4.1-hardening`

**Scope:** First-party Companion Plugin code, pairing/auth integration, updater configuration, and release packaging. Vendored plugin-update-checker internals were inspected only where needed to validate first-party configuration and were not modified.

## Executive summary

The targeted audit confirmed and fixed six issues. The reported debug source-inspection route was removed entirely, along with its companion self-test route and persistent diagnostics. Device bearer authentication is now REST-only and fails closed when a device lacks a valid approving WordPress user. Public pairing inputs and state transitions are bounded and rate limited. The updater now requires the exact Companion zip instead of accepting the first GitHub release asset or falling back to the wrong-layout source archive. Malformed stored device rows no longer emit warnings or crash revocation. Release packaging is allow-listed and reproducible.

No additional first-party defects were found in the admin pairing actions or the nine product, order, and customer abilities. Their existing capability boundaries were retained and captured in regression tests.

## Threat model reviewed

- Unauthenticated callers of pairing request and poll routes.
- Unknown, revoked, replayed, stolen, legacy, and malformed device bearers.
- Authenticated users with insufficient WordPress capabilities.
- Administrators approving, rejecting, and removing devices in wp-admin.
- Authorized ability callers reading customer/order data or mutating products/orders.
- Release automation and WordPress self-update package selection.
- Accidental inclusion of local development files or credentials in the plugin archive.

## Confirmed findings and resolutions

| ID | Severity | Finding | Resolution | Regression evidence |
|---|---|---|---|---|
| CP-041-01 | Medium | Admin-only `/wooagent-companion/v1/source` accepted an arbitrary loadable class/function, reflected its local path, read its defining file, and returned source. `/selftest` exposed additional internal diagnostics. The capability gate reduced exploitability but did not justify shipping the source-read primitive. | Removed both routes, reflection/source reads, temporary error capture, and persistent registration diagnostics. | `tests/companion-plugin/debug-surface-test.php` |
| CP-041-02 | Medium | The bearer bridge ran through `determine_current_user` outside REST and legacy device records silently inherited the first administrator's identity. Deleted approving users also remained resolvable. | Limited the bridge to `REST_REQUEST`; preserved earlier auth; required the stored approving user to exist; removed first-admin fallback. `/devices/me` and revoke use the same validity rule, so invalid legacy records trigger the daemon's existing re-pair flow. | `tests/companion-plugin/auth-rest-test.php`, `auth-non-rest-test.php` |
| CP-041-03 | Medium | Public pairing requests accepted unbounded unsanitized device names, could overwrite pending or terminal state, and had no request-pressure controls. | Added REST schemas and defensive callback validation, a 100-byte device-name bound, non-destructive pending retries, HTTP 409 for terminal-code reuse, and separate fixed-window limits of 10 requests/minute and 90 polls/minute per hashed remote address. | `tests/companion-plugin/pairing-security-test.php` |
| CP-041-04 | Medium | `enableReleaseAssets()` accepted every release asset. PUC selects the first matching asset and otherwise falls back to GitHub's generated source archive, so WordPress could receive a daemon archive or a zip with the wrong plugin layout. | Filtered for exactly `wooagent-companion.zip` and set `Api::REQUIRE_RELEASE_ASSETS`; the verifier locks the canonical repository, slug, exact asset filter, and no-fallback setting. | `tests/companion-plugin/debug-surface-test.php`, `scripts/verify-companion-plugin.sh` |
| CP-041-05 | Low | Malformed `token_hash` values in the devices option could emit PHP warnings in auth resolution or throw a `TypeError` during revoke. | Ignore non-scalar hashes in auth/device lookup and preserve malformed rows for administrator cleanup while safely removing only the valid matching device. | `tests/companion-plugin/auth-rest-test.php`, `pairing-security-test.php` |
| CP-041-06 | Low | The zip script copied the whole source directory and produced different hashes from identical source because timestamps, ordering, and host metadata were not normalized. | Package only the main file, readme, `includes/`, and `vendor/`; reject development/credential artifacts; normalize timestamps; sort entries; omit host-specific metadata; compare two build hashes in verification. | `scripts/verify-companion-plugin.sh` |

## Boundary matrix

| Boundary | Result | Evidence |
|---|---|---|
| Debug `/source` and `/selftest` | Removed; an installed 0.4.1 site has no registered route and returns WordPress's normal 404. | Static regression plus absence from packaged source |
| Malformed or unknown pairing code | HTTP 400/404 without internal paths, secrets, or PII. | Pairing regression suite |
| Pairing request/poll pressure | Fixed-window limits; rate buckets store a SHA-256-derived address identifier, not the raw address. | Pairing regression suite |
| Duplicate pending code | Returns the original pending expiry without changing name, state, or remaining TTL. | Pairing regression suite |
| Duplicate terminal code | HTTP 409; approved/delivered/rejected state is not reset. | Pairing regression suite |
| Unknown or revoked bearer | No user identity; `/devices/me` and revoke permission reject it. | Auth and pairing regression suites |
| Legacy or deleted-approver bearer | No administrator fallback; re-pair required. | Auth regression suite |
| Valid REST bearer | Resolves only to its existing approving user; current WordPress capabilities still apply. | Auth and ability regression suites |
| Valid bearer outside REST | Ignored. | Non-REST auth regression |
| Admin approve/reject/remove | Existing `manage_options` checks and distinct nonces retained; displayed/request values remain sanitized and escaped. | Source review of `admin-pair-screen.php` |
| Product reads and update | Existing `read_private_products`/`edit_products` gates retained; closed input schemas. | Ability regression suite |
| Order reads and notes | Existing `read_private_shop_orders`/`edit_shop_orders` gates retained; customer-facing note behavior remains explicit. | Ability regression suite |
| Customer reads | Existing `list_users` or `edit_shop_orders` gate retained; returned PII remains limited to authorized operators. | Ability regression suite |
| Token storage/delivery | Device tokens remain high-entropy, hash-only in the durable option, plaintext only in the short-lived approval transient, and omitted from tests/logs/errors. | Source review of `pair-rest.php` |
| Updater | Canonical public repository and slug retained; exact Companion asset required; source-zip fallback disabled. | Updater regression and verifier |
| Zip contents | One `wooagent-companion/` root; exact first-party files and updater entrypoint; no tests, dotfiles, environment files, keys, certificates, or paths outside the slug. | Archive verifier |

## Verification evidence

Command:

```bash
bash scripts/verify-companion-plugin.sh
```

Verified on 2026-08-20:

- 8 first-party PHP files: no syntax errors.
- 22 security/permission regression checks: passed.
- Plugin header, constant, stable tag, and README pin: `0.4.1`/`v0.4.1` and consistent.
- Updater repository, slug, exact release asset, and required-asset behavior: passed.
- Archive integrity, required entries, one-root layout, and forbidden-artifact checks: passed.
- Two consecutive identical-source builds: byte-identical.
- Artifact: `build/wooagent-companion.zip`
- Size: `189617` bytes.
- SHA-256: `0f150ed07c5643c56d3a9babb14fb9812c31e1e6bbf700d2d647245094e10a6a`

The optional read-only `--integration` mode was not run because no designated staging URL or test device token was supplied. No live store was read from or mutated during this audit.

## Compatibility and release notes

- Device records created before `paired_by_user_id` capture, or whose approving user was deleted, intentionally fail closed. The daemon's existing staleness probe will mark the store unpaired; the operator must pair again.
- The wire protocol and success response shapes are unchanged.
- PUC derives the offered version from the GitHub release tag in this monorepo. Every published WooAgent release must therefore keep the tag, Companion plugin header, and readme stable tag aligned. The 0.4.1 verifier enforces this release's values.
- Credential rotation discussed separately is unrelated to these plugin changes and was not performed.

## Residual risks and accepted scope

- Device bearers remain long-lived until explicit revocation. A stolen valid bearer has the current capabilities of its approving user; token expiry/rotation is a future protocol decision.
- Pair approval delivery uses WordPress transient reads/writes rather than an atomic compare-and-swap. The code, ten-minute window, rate limits, TLS expectation, and single-use state reduce exposure, but fully atomic delivery would require a storage/protocol change.
- WordPress updater trust remains rooted in repository/release write access and HTTPS. No independent package-signature verification was added.
- Rate limiting is per site and remote address. Shared proxies/NATs share a bucket, and distributed callers can use separate addresses.
- Live WooCommerce read/mutation behavior was not exercised. Ability implementation code was unchanged, and its registrations, closed schemas, and permission callbacks are regression-tested.

## Publication gate

This branch is review-ready only. No push, tag, GitHub release, release-asset upload, plugin installation, or production/staging mutation was performed. Those actions require separate explicit approval.
