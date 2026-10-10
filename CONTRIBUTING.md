# Contributing to devx

Thank you for your interest in contributing! This document covers how to get started, submit changes, and understand the project conventions.

## Table of contents

- [Code of conduct](#code-of-conduct)
- [Getting started](#getting-started)
- [Development setup](#development-setup)
- [Project structure](#project-structure)
- [Making changes](#making-changes)
- [Tests](#tests)
- [Submitting a pull request](#submitting-a-pull-request)
- [Reporting bugs](#reporting-bugs)
- [Requesting features](#requesting-features)

## Code of conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating you agree to uphold it.

## Getting started

1. [Fork](https://github.com/dever-labs/devx/fork) the repository.
2. Clone your fork:
   ```sh
   git clone https://github.com/<your-username>/devx.git
   cd devx
   ```
3. Add the upstream remote:
   ```sh
   git remote add upstream https://github.com/dever-labs/devx.git
   ```

## Development setup

**Requirements:**
- [Go 1.21+](https://go.dev/dl/)
- [Docker Desktop](https://www.docker.com/products/docker-desktop/) or equivalent

**Build the binary:**
```sh
go build ./cmd/devx
./devx version
```

**Run unit tests:**
```sh
go test ./...
```

**Run integration tests** (requires Docker):
```sh
go test ./tests/integration/... -tags integration -v
```

**Cross-compile all platforms:**
```sh
./scripts/build.sh        # Linux/macOS
.\scripts\build.ps1       # Windows
```

## Project structure

```
cmd/devx/           — CLI entry point and command handlers
internal/           — private packages
  config/           — manifest parsing and validation
  compose/          — Docker Compose rendering and telemetry stack
  runtime/          — container runtime abstraction (Docker, Podman)
  k8s/              — Kubernetes manifest rendering
  graph/            — dependency graph and topological sort
  lock/             — lockfile management
docs/               — user-facing documentation
examples/           — complete working project examples
packaging/          — distribution packaging (Homebrew, npm, WinGet, Chocolatey)
scripts/            — build and install scripts
schemas/            — JSON Schema for devx.yaml
tests/integration/  — end-to-end tests
```

## Making changes

- Create a branch from `main`:
  ```sh
  git checkout -b feat/my-feature
  ```
- Keep commits focused and atomic.
- Follow existing code style (no external linter config — just match what's there).
- Add or update tests for any behaviour you change.
- Update relevant docs under `docs/` if the change affects user-facing behaviour.

## Tests

| Command | What it runs |
|---------|-------------|
| `go test ./...` | All unit tests |
| `go test ./tests/integration/... -tags integration -v` | Integration tests (requires Docker) |
| `go test ./tests/integration/... -tags integration -short` | Integration tests, skips container startup |

New code should include unit tests. Integration tests are appropriate for end-to-end CLI behaviour.

## Submitting a pull request

1. Push your branch and open a pull request against `main`.
2. Fill in the pull request template.
3. Ensure CI passes (build + unit tests + integration tests).
4. A maintainer will review and merge.

For large or breaking changes, please open an issue first to discuss the approach.

## Reporting bugs

Use the [Bug Report](.github/ISSUE_TEMPLATE/bug_report.md) issue template. Include:
- devx version (`devx version`)
- OS and architecture
- Steps to reproduce
- Expected vs actual behaviour

## Requesting features

Use the [Feature Request](.github/ISSUE_TEMPLATE/feature_request.md) issue template. Describe the problem you're trying to solve and why it belongs in devx.

## Releasing (maintainers)

Releases are cut manually by pushing a version tag; nothing tags automatically.

1. Move the `[Unreleased]` entries in `CHANGELOG.md` under the new version and commit to `main`.
2. Make sure CI is green on `main`.
3. Tag and push: `git tag v0.1.0 && git push origin v0.1.0`.
4. The `Release` workflow tests, builds all six binaries, publishes them with `checksums.txt`, and updates the Homebrew tap (needs the `TAP_GITHUB_TOKEN` secret).
5. When the release is published, the `Install channels` workflow installs devx via the script, npm and Homebrew and runs `devx version` + `devx validate`. Re-run it manually with a tag via *Actions → Install channels → Run workflow*.
6. Publish to the remaining channels by hand: `npm publish` from `packaging/npm` (set the version first), and submit the Chocolatey and winget manifests from `packaging/` with the new version and checksums.

Pull requests that touch `packaging/` or `scripts/` are linted offline by the same workflow.
