# Contributing to WooAgent OS

Thanks for your interest in contributing. This document covers the practical bits — how to file an issue, set up your environment, and structure a pull request.

## Code of conduct

Participation is governed by the [Code of Conduct](./CODE_OF_CONDUCT.md). By contributing, you agree to abide by its terms.

## Filing issues

Before opening an issue:

- **Search existing issues first** (open and closed) to avoid duplicates.
- **For security vulnerabilities, do not open a public issue** — see [`SECURITY.md`](./SECURITY.md) for the private disclosure process.

For bug reports, include:

- WooAgent OS version (or commit SHA).
- Platform (macOS / Linux / Windows; arch).
- Steps to reproduce.
- Expected vs. observed behavior.
- Relevant log output (`~/.wooagent/wooagent.log` if running locally).

For feature requests, describe the user problem first, then a proposed solution. We may close requests that don't fit the project's scope — see the README for the project's goals.

## Development setup

See the [Quickstart (dev) section of the README](./README.md#quickstart-dev) for the canonical setup. In short:

- **Go daemon** — Go 1.23+, run from `daemon/`.
- **React UI** — Node 20+, run from `ui/`.
- **Companion Plugin** — local WordPress install or pair against a test WooCommerce store.

## Pull requests

For non-trivial changes, **open an issue first** to discuss the approach. A PR that arrives without prior discussion may be closed if it doesn't fit the project's direction.

Before submitting:

- Run `go vet ./...` and `go build ./...` from `daemon/` — both must pass.
- Run `npm run typecheck` from `ui/` if you touched the UI.
- Add or update tests for behavior changes. We use the standard Go testing package plus a small set of helpers in `daemon/internal/personas/testhelpers.go`.
- Keep PRs focused. One concern per PR; large refactors should be split.
- Write a clear PR description. The format `<type>(<scope>): <subject>` is conventional (`feat`, `fix`, `chore`, `docs`, `refactor`).

## UI conventions

UI work in `ui/` follows the WordPress Design System (WPDS) exclusively. See [`CLAUDE.md`](./CLAUDE.md) and [`DESIGN.md`](./DESIGN.md) for the canonical components, tokens, and the documented persona-color exception. New custom UI requires a `// CUSTOM:` comment explaining why no WPDS component fits.

## Go conventions

- `go fmt` and `go vet` are mandatory. CI enforces both.
- Prefer table-driven tests where the shape fits.
- Package-internal helpers go in lower-case files; do not export from `internal/` packages.

## Contributor License Agreement (CLA)

The CLA / DCO process for external contributions is being finalized. Once it's in place, you'll be prompted to sign before your first PR is merged. We'll update this document with the specific process when it lands.

## Questions

Open a discussion on GitHub for design questions or scope conversations. For triage-level questions on an existing issue, comment on the issue itself.
