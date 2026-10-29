# Changelog

All notable changes to devx will be documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- `.github/dependabot.yml` (gomod + github-actions) and `.github/CODEOWNERS`.
- Lifecycle hooks (`afterUp`, `beforeDown`) — run migrations, scripts, or exec commands inside containers at environment start/stop
- `devx version` command — prints the binary version set at build time
- `scripts/install.sh` and `scripts/install.ps1` — one-liner installers
- Integration test scaffolding under `tests/integration/`
- Comprehensive `examples/basic/` with all profile types and stub service source

### Changed
- **Breaking**: `devx.yaml` is now a flat map of task names to task definitions (`run`/`deps`/`ready`/`detached`/...) instead of `profiles`/`services`/`deps` — see `docs/manifest.md` and `docs/design/task-executor-refactor.md`. `run:` executes directly via the system shell; nothing is rendered to Docker Compose or Kubernetes manifests anymore.
- `devx up`/`devx run <name>` now resolve and execute the task dependency graph directly (levelized parallel execution, `--dry-run`/`--keep-going`/`--group` support) instead of shelling out to `docker compose`/`kubectl`.
- `devx down` now stops plain-process detached tasks by signaling their recorded process-group PID (previously a no-op for anything that wasn't a container).
- `devx <name>` is shorthand for `devx run <name>`.
- Release process rewritten around
  [release-please](https://github.com/googleapis/release-please) (automated
  SemVer + changelog from Conventional Commits) +
  [goreleaser](https://goreleaser.com) (cross-platform build, checksums, and
  Homebrew tap publish), replacing the hand-rolled matrix-build +
  manual-tag + manual-changelog `release.yml`. See `.goreleaser.yaml`,
  `.github/workflows/release-please.yml`, `docs/install.md`.
- `README.md` rewritten to document the current flat task-map CLI (it still
  described the removed `devx render`/profiles/telemetry-by-default model).
- `.github/workflows/ci.yml` now also runs `govulncheck` and enforces a
  coverage-regression floor (`scripts/check-coverage.sh`) on every push/PR.
- `.github/workflows/release.yml` actions are now pinned to commit SHAs,
  matching `ci.yml`'s existing convention (previously used floating tags).
- Build scripts now inject version via `-ldflags -X main.version` and include `linux/arm64` + `windows/arm64` targets
- Repository structure reorganised: `examples/basic/src/` holds example app stubs

### Removed
- `devx render`, `devx lock`, `devx providers`, `devx export` — no compose/k8s/helm/terraform artifact to render under the Task model; these now print a clear "not supported" error.
- `internal/compose`, `internal/k8s`, `internal/providers`, `internal/lock` packages.
- Stale `schema/devx.schema.json` (pre-refactor shape, unreferenced by any
  code) — `schemas/devx.schema.json` is now the single source of truth.
- `docs/telemetry.md` — documented the Grafana/Loki/Prometheus telemetry
  stack that was already removed (see the compose-rendering removal above).
- `packaging/npm`, `packaging/chocolatey`, `packaging/winget` — stuck at
  placeholder version `0.0.0` with no release automation ever wired up.
  Homebrew remains the one supported, automated package-manager channel.
- `scripts/generate-formula.py` — superseded by goreleaser's
  `homebrew_casks:` config.

### Fixed
- **`devx` did not actually cross-compile for Windows.** `internal/task` and
  `cmd/devx` called Unix-only `syscall.Kill`/`SysProcAttr.Setpgid` directly for
  process-group management, so `GOOS=windows go build ./...` failed outright —
  every past `devx-windows-*.exe` release asset predates this and the release
  workflow's Windows matrix entries could never have succeeded. Process-group
  start/stop/liveness now goes through small `//go:build` Unix/Windows files
  (`driver_unix.go`/`driver_windows.go`, `cmd_down_unix.go`/`cmd_down_windows.go`);
  Windows uses `taskkill /T` for tree termination and
  `OpenProcess`+`GetExitCodeProcess` for liveness checks.
- Root `devx.yaml` (the project's own dogfood manifest) was still in the
  pre-refactor `profiles:`/`services:`/`deps:`/`registry:` shape and failed
  `devx validate` against the current flat task-map model. Replaced with a
  real task graph (`build`/`vet`/`lint`/`test`/`integration`/`all`).
- 28 leftover `dever-labs/dever` references (the repo's pre-rename name) across
  README, CONTRIBUTING, SECURITY, CODE_OF_CONDUCT, docs/install.md, and the
  install scripts — these broke badges, install one-liners, and the security
  advisory link.
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
