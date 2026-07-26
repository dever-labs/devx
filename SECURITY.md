# Security policy

## Supported versions

| Version | Supported |
|---------|-----------|
| Latest release | ✅ |
| Older releases | ❌ |

We support only the latest released version of devx. Please upgrade before reporting a vulnerability.

## Reporting a vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Please report security issues by emailing **security@labs.dever.dk** (or open a [GitHub private security advisory](https://github.com/dever-labs/dever/security/advisories/new)).

Include:
- A description of the vulnerability
- Steps to reproduce or a proof-of-concept
- The version of devx affected
- Any potential impact you've identified

We will acknowledge receipt within **48 hours** and aim to release a fix within **14 days** for critical issues.

## Scope

This policy covers the `devx` binary and the packages under `internal/`. It does not cover:

- Third-party images started by devx (those are the user's responsibility)
- The telemetry stack containers (Grafana, Prometheus, etc.) — report those upstream
