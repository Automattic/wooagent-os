# Companion Plugin 0.4.1 security-hardening design

**Date:** 2026-08-20
**Status:** Approved for implementation planning

## Summary

WooAgent Companion 0.4.0 shipped temporary diagnostic REST routes that can return PHP source files to an administrator. Version 0.4.1 will remove that debug surface and perform a targeted audit of every first-party Companion Plugin trust boundary. It will fix confirmed security and hardening issues without adding features or redesigning the plugin.

The result will be a review-ready 0.4.1 change set and verified plugin ZIP. Publishing the GitHub release remains a separate, explicit approval step.

## Goals

- Remove the `/source` and `/selftest` diagnostic REST routes.
- Audit first-party Companion Plugin authorization, authentication, validation, data exposure, and state changes.
- Fix confirmed weaknesses that fit a backward-compatible patch release.
- Add the smallest repeatable verification tooling the repository needs for future Companion Plugin releases.
- Produce a version-consistent, inspected `wooagent-companion.zip` for review.

## Non-goals

- New Companion Plugin features or abilities.
- Broad code organization or style refactors.
- A redesign of daemon/plugin pairing or the MCP protocol.
- Re-auditing the internals of the vendored `plugin-update-checker` library.
- Changes to the daemon or UI unless a secure, compatible plugin fix cannot be made without them.
- Rotating local development credentials, which is unrelated operational housekeeping and does not block 0.4.1.
- Tagging or publishing the 0.4.1 GitHub release.

## Scope

The audit covers all first-party PHP under `companion-plugin/`:

- `wooagent-companion.php`
- `includes/pair-rest.php`
- `includes/auth-bridge.php`
- `includes/admin-pair-screen.php`
- `includes/abilities-products.php`
- `includes/abilities-orders.php`
- `includes/abilities-customers.php`
- `includes/update-checker.php`

It also covers WooAgent's integration with the vendored updater and `scripts/build-companion-plugin-zip.sh`. Vendored library internals are trusted as an upstream dependency; the audit checks how WooAgent configures and packages them.

## Threat model

### Callers

The audit evaluates each boundary from five perspectives:

1. An unauthenticated internet visitor.
2. An authenticated WordPress user without administrative or WooCommerce management capabilities.
3. A WordPress administrator.
4. A valid paired WooAgent device acting through its bearer token.
5. A caller holding an invalid, expired, revoked, replayed, stolen, or legacy device token.

### Assets and security properties

- WordPress user identity and capabilities must not be elevated or confused.
- Pairing codes and device tokens must remain confidential, time-bounded where applicable, and revocable.
- Product, order, and customer data must only be exposed to callers with the intended capabilities.
- Store mutations must be authorized, schema-valid, and limited to the requested operation.
- PHP source, server paths, secrets, token hashes, and unnecessary PII must not be disclosed.
- WordPress options and transients must not be overwritable, enumerable, or replayable beyond the documented pairing flow.
- Plugin updates must come from the canonical repository and produce the expected installable ZIP shape.

## Boundaries to inventory

The audit begins with a matrix of every externally reachable or security-sensitive operation. Each row records the caller, required authority, accepted input, output, state mutation, sensitive data, failure behavior, and verification evidence.

The inventory includes:

- Pairing REST routes: `/pair/request`, `/pair/poll`, `/pair/revoke`, and `/devices/me`.
- Temporary diagnostic routes: `/source` and `/selftest`.
- Every registered product, order, and customer ability.
- All wp-admin pairing approval, rejection, removal, and redirect handlers.
- Bearer-token extraction, hashing, lookup, user resolution, legacy-record behavior, and revocation.
- Pairing transients and the `wooagent_devices` option.
- Update-checker repository and release-asset configuration.
- Plugin ZIP staging, contents, and version metadata.

## Audit method

### 1. Trace untrusted data

For every boundary, trace request parameters, headers, stored values, and WordPress user state through permission checks and callbacks to their final response or sensitive sink. Sensitive sinks include authentication, capability-bearing user selection, options/transients, WooCommerce mutations, file access, reflection, outbound requests, redirects, and rendered HTML.

### 2. Review security-specific behaviors

The audit gives extra scrutiny to:

- Public pairing-code entropy, normalization, overwrite behavior, enumeration, brute force, expiry, caching, and replay.
- Single-use delivery of plaintext device tokens.
- Bearer parsing, constant-time comparison, authentication scope, revocation, and legacy fallback to a WordPress user.
- Whether one paired device can affect another device.
- Ability schemas, `additionalProperties`, enums, bounds, callback-side defensive validation, and authorization capabilities.
- Customer and order PII minimization.
- Admin-action capabilities, nonces, escaping, and redirect construction.
- Error responses and logs for secrets, token material, server paths, raw exceptions, or excessive detail.
- Canonical updater source, release-asset selection, and package integrity.

### 3. Record findings and decisions

Every confirmed finding is recorded with:

- Evidence and reproduction.
- Affected boundary and callers.
- Impact and severity.
- Root cause.
- Chosen remediation.
- Regression verification.

Suspicions that cannot be reproduced or supported by code flow are recorded as observations, not silently changed. Any finding whose secure fix would invalidate existing pairings, change the daemon/plugin protocol, or require a data migration is brought back for approval before implementation.

## Change policy

The known mandatory change is removal of the `rest_api_init` hook and complete `wooagent_companion_register_debug_route()` implementation, eliminating both `/source` and `/selftest`.

Other changes must address a confirmed authorization, validation, disclosure, replay, secret-handling, or update-integrity weakness. The implementation should preserve current route and ability contracts wherever doing so is secure. It must not bundle features, speculative defenses, or unrelated cleanup.

Failure behavior should be explicit and closed:

- Malformed input returns a bounded `400` response.
- Missing or invalid authentication returns `401`; an authenticated caller without the required capability receives `403`.
- Missing or expired pairing state returns `404` without enumeration-friendly detail.
- Unexpected internal conditions return a generic error without paths, secrets, or raw exception text.
- Sensitive responses carry no-cache headers where replay or stale authorization state matters.

## Verification design

### Static and package verification

Add a repository script under `scripts/` that:

- Runs `php -l` over all first-party Companion Plugin PHP files.
- Rejects production occurrences of the removed debug-route names and arbitrary source-read implementation.
- Checks the plugin header, version constant, readme stable tag, and changelog for 0.4.1 consistency.
- Builds the plugin with the existing ZIP script.
- Runs `unzip -t` and inspects the archive layout.
- Rejects editor, VCS, test, secret, or temporary artifacts in the archive.

The script must not print credentials and must be runnable without access to a live store for its default checks.

### WordPress behavior verification

Run a repeatable HTTP smoke matrix against a disposable or designated test WordPress installation with the candidate plugin installed. The commands and results are captured in the audit record. Credentials are passed through the environment and never printed.

The matrix verifies:

- `/source` and `/selftest` return `404 rest_no_route`, including for an administrator.
- Public pairing routes accept only their intended requests and reject malformed or unknown codes.
- Invalid, expired, and revoked bearer tokens cannot authenticate or access device routes.
- A valid paired device can probe and revoke itself, and revocation immediately invalidates it.
- Lower-privilege users cannot call protected abilities or admin actions.
- Intended product, order, and customer reads still work for authorized callers.
- Intended store mutations still work for callers with the required WooCommerce capabilities.
- Error bodies and logs contain no plaintext tokens, hashes, PHP paths, or unnecessary PII.

If the current repository cannot automate a behavior without introducing a large WordPress test framework, the verification script provides an integration mode driven by environment variables, and the audit record preserves the exact assertions and observed statuses.

## Versioning and release artifacts

The review-ready change set updates every applicable plugin version reference from 0.4.0 to 0.4.1, including:

- The plugin `Version` header.
- `WOOAGENT_COMPANION_VERSION`.
- `Stable tag` in `companion-plugin/readme.txt`.
- A sanitized 0.4.1 changelog entry.
- User-facing pinned-version documentation where it refers to the release artifact being prepared.

The existing build script produces `build/wooagent-companion.zip`. The ZIP is verified but not published. Tag creation, pushing a release tag, and GitHub publication require a later explicit approval.

## Deliverables

- A completed Companion Plugin boundary matrix and audit record.
- Code fixes for all confirmed, in-scope findings.
- Targeted regression verification and the reusable verification script.
- Consistent 0.4.1 metadata and changelog.
- A verified `build/wooagent-companion.zip`.
- Sanitized release notes suitable for the eventual GitHub release and security-report response.

## Acceptance criteria

- The audit covers every first-party route, ability, admin action, auth path, persistence boundary, updater integration, and packaging step listed above.
- `/source`, `/selftest`, their hook, and their implementation are absent from production code and the ZIP.
- Unauthorized and lower-privilege callers cannot cross a protected boundary.
- Invalid, expired, and revoked pairing credentials fail closed.
- Valid pairing, revocation, authorized reads, and authorized mutations remain functional.
- Sensitive values are absent from responses, logs, audit artifacts, and packaged files.
- All confirmed findings have a remediation and regression check, or an explicitly approved deferral.
- PHP syntax checks, static guards, WordPress smoke checks, ZIP integrity, and archive-content checks pass.
- All applicable release metadata consistently identifies version 0.4.1.
- No tag or GitHub release is published without separate approval.
