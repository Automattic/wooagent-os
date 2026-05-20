# Security policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Report security issues privately to Automattic via:

- **HackerOne (preferred):** <https://hackerone.com/automattic>
- **Email:** <security@automattic.com>

Please include:

- A description of the issue and the impact you believe it has.
- Steps to reproduce — minimum reliable repro.
- The affected version (release tag or commit SHA) and the platform you tested on.
- Any proof-of-concept code or screenshots, if applicable.

We aim to acknowledge reports within three business days and to provide an initial assessment within ten business days.

## Scope

In scope:

- The Go daemon (`daemon/`) and the binaries it produces.
- The React UI (`ui/`) as served by the daemon or built standalone.
- The Companion Plugin (`companion-plugin/`) and the ability surface it registers on a WooCommerce store.
- The release pipeline (`scripts/`, GitHub Releases artifacts) and supply-chain integrity of distributed binaries.

Out of scope:

- Vulnerabilities in third-party LLM providers, WordPress core, WooCommerce, or other plugins the project integrates with — report those to their respective maintainers.
- Issues that require physical access to the host running the daemon, or that depend on an attacker already having root on that host.
- Social-engineering attacks against project maintainers or community members.

## Supported versions

Because WooAgent OS is pre-1.0, only the latest tagged release is supported for security fixes. We do not backport patches to older minor versions during the `v0.x` series.

## Disclosure

We follow coordinated disclosure. After a fix is shipped, we credit the reporter in the release notes unless they prefer to remain anonymous.
