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
cmd/devx/           — CLI entry point; one cmd_<name>.go per subcommand
internal/           — private packages
  config/           — devx.yaml parsing and validation
  task/             — task graph resolution, scheduling, process/container drivers
  runtime/          — container runtime abstraction (Docker, Podman)
  graph/            — dependency graph and topological sort
  doctor/           — tool/health checks for `devx doctor`
  setup/            — tool install, host-side setup steps
  ai/               — LLM client (openai, anthropic, azure-openai, ollama, local)
docs/               — user-facing documentation
examples/           — complete working project examples
schemas/            — JSON Schema for devx.yaml
scripts/            — build and install scripts
tests/integration/  — end-to-end tests (build tag: integration)
```

## Making changes

- Create a branch from `main`:
  ```sh
  git checkout -b feat/my-feature
  ```
- Keep commits focused and atomic.
- Follow existing code style; CI runs `golangci-lint` with the repo's `.golangci.yml` config.
- Commit messages must follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat:`, `fix:`, `chore:`, etc.) — releases are automated via release-please,
  which derives version bumps and the changelog directly from these prefixes.
- **One fix/issue per commit, one fix/issue per PR.** Never batch several unrelated
  fixes or issues into a single commit or a single PR, even if squash-merged —
  release-please generates exactly one changelog entry per commit, so batching
  collapses several distinct fixes into one vague changelog line and hides real
  release scope. If you're addressing multiple issues, open a separate branch/PR
  per issue and merge each independently.
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
