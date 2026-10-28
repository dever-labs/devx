# Changelog

All notable changes to devx will be documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- Standalone repo support: `kind: process` services (host commands supervised by devx, mixable with containers), `ready` HTTP/TCP probes, `devx up --attach`, and `devx debug-config` for VS Code attach configs
- `devx status`, `logs`, `exec` and `down` handle process services; `status --json` reports kind, state, pid and ports
- Tool versions and checks are inferred from `.nvmrc`, `package.json`, `global.json`, `go.mod`, `.tool-versions`; `installer: system` opt-out; version-manager-aware installs (volta, fnm, nvm, corepack); `doctor` warns on mismatches
- `setup`: `workspace: auto` single-root installs for JS monorepos, and `envFiles` for generating local config/secrets (env, command, prompt sources)
- `--json` for `validate` and `auth`; `setup --non-interactive`
- `AGENTS.md`, accurate `schemas/devx.schema.json`, and an `Install channels` workflow
- `doctor` treats a missing container runtime as a warning for manifests with no containers
- Lifecycle hooks (`afterUp`, `beforeDown`) — run migrations, scripts, or exec commands inside containers at environment start/stop
- `devx version` command — prints the binary version set at build time
- Multi-platform release workflow — GitHub Actions builds for Linux, macOS, Windows (amd64 + arm64) on `git tag v*`
- Distribution packaging — Homebrew formula, npm package, WinGet manifests, Chocolatey package, install scripts
- Dev Container Feature (`ghcr.io/dever-labs/devx/devx`) to install devx in any devcontainer without a custom install step; published via `.github/workflows/publish-features.yml`
- `scripts/install.sh` and `scripts/install.ps1` — one-liner installers
- Integration test scaffolding under `tests/integration/`
- Comprehensive `examples/basic/` with all profile types and stub service source

### Changed
- Fixed repository references in installers and packaging (`dever-labs/dever` → `dever-labs/devx`)
- **Breaking:** the built-in telemetry stack is now opt-in. Use `devx up --telemetry` / `devx render compose --telemetry`; `--no-telemetry` is accepted but is a no-op
- CI updated to `actions/checkout@v4` and `actions/setup-go@v5` with `go-version-file`
- Build scripts now inject version via `-ldflags -X main.version` and include `linux/arm64` + `windows/arm64` targets
- Repository structure reorganised: `packaging/` consolidates all distribution artefacts, `examples/basic/src/` holds example app stubs

### Fixed
- YAML indentation errors in `internal/config/manifest_test.go`
- `cmd/devx/main_test.go` rewritten to use temp files instead of invalid function-variable monkey-patching

---

## [0.1.0] - 2024-01-01

### Added
- Initial release
- `devx up / down / status / logs / exec / doctor / render / lock / init` commands
- Docker Compose and Kubernetes rendering from `devx.yaml`
- Built-in telemetry stack (Grafana, Loki, Prometheus, Grafana Alloy, cAdvisor, docker-meta exporter)
- Four Grafana dashboards: Service Health, Container Resources, Logs, Log Analytics
- Per-container network metrics via Docker Stats API
- Browser link printing after `devx up`
- Multi-profile support (`local`, `ci`, `k8s`, etc.)
- Dependency graph with cycle detection
- Lockfile support for pinning image digests
- Plugin system

[Unreleased]: https://github.com/dever-labs/devx/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/dever-labs/devx/releases/tag/v0.1.0
