# Changelog

All notable changes to devx will be documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Changed
- **Breaking**: `devx.yaml` is now a flat map of task names to task definitions (`run`/`deps`/`ready`/`detached`/...) instead of `profiles`/`services`/`deps` — see `docs/manifest.md` and `docs/design/task-executor-refactor.md`. `run:` executes directly via the system shell; nothing is rendered to Docker Compose or Kubernetes manifests anymore.
- `devx up`/`devx run <name>` now resolve and execute the task dependency graph directly (levelized parallel execution, `--dry-run`/`--keep-going`/`--group` support) instead of shelling out to `docker compose`/`kubectl`.
- `devx down` now stops plain-process detached tasks by signaling their recorded process-group PID (previously a no-op for anything that wasn't a container).
- `devx <name>` is shorthand for `devx run <name>`.

### Removed
- `devx render`, `devx lock`, `devx providers`, `devx export` — no compose/k8s/helm/terraform artifact to render under the Task model; these now print a clear "not supported" error.
- `internal/compose`, `internal/k8s`, `internal/providers`, `internal/lock` packages.

### Added

- Lifecycle hooks (`afterUp`, `beforeDown`) — run migrations, scripts, or exec commands inside containers at environment start/stop
- `devx version` command — prints the binary version set at build time
- Multi-platform release workflow — GitHub Actions builds for Linux, macOS, Windows (amd64 + arm64) on `git tag v*`
- Distribution packaging — Homebrew formula, npm package, WinGet manifests, Chocolatey package, install scripts
- `scripts/install.sh` and `scripts/install.ps1` — one-liner installers
- Integration test scaffolding under `tests/integration/`
- Comprehensive `examples/basic/` with all profile types and stub service source

### Changed
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

[Unreleased]: https://github.com/dever-labs/dever/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/dever-labs/dever/releases/tag/v0.1.0
