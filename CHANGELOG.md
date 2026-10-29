# Changelog

All notable changes to devx will be documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.1.3](https://github.com/dever-labs/devx/compare/v0.1.2...v0.1.3) (2026-10-29)


### Features

* add devx Dev Container Feature ([#19](https://github.com/dever-labs/devx/issues/19)) ([f834084](https://github.com/dever-labs/devx/commit/f834084dd1ed3583758935c8277c8555c995ce51))

## [0.1.2](https://github.com/dever-labs/devx/compare/v0.1.1...v0.1.2) (2026-10-29)


### Features

* add AI deployment generation and provider management ([84a92e0](https://github.com/dever-labs/devx/commit/84a92e0bb3182a2f775a2d483963ae533485c9a2))
* add background hook support with live output streaming ([5067532](https://github.com/dever-labs/devx/commit/5067532837995147cbabb93eb16f775e9a38eaee))
* add Kubernetes support with rendering, applying, and deleting manifests; update documentation and validation ([81977c9](https://github.com/dever-labs/devx/commit/81977c9862becfd014591fa34d25c08b60b95c14))
* add manifest and validation for compose configuration ([a4bddd4](https://github.com/dever-labs/devx/commit/a4bddd4707b3fbe29541a877755c3201c69bee67))
* add setup, validate, doctor --fix, and JSON output ([574bc48](https://github.com/dever-labs/devx/commit/574bc48013eb82e3ae379b2a0502ecacaeed4056))
* add telemetry support with Grafana, Loki, and Prometheus; update commands and documentation ([e84d7bc](https://github.com/dever-labs/devx/commit/e84d7bc07f724633fbdedb02f059a692116deba7))
* AI wizard, devcontainer management, script runner, Homebrew distribution ([15ab075](https://github.com/dever-labs/devx/commit/15ab0757805cf04b0441899ff09b1f58def689cf))
* env, auth, dev logs/ps/shell, ai chat, ci run, new, .env loading, Continue.dev config ([9446e18](https://github.com/dever-labs/devx/commit/9446e18649c04dfa0b6c8f8e9ff10be961378e62))
* replace Profile/Compose/K8s model with flat Task scheduler (Phase 1) ([#9](https://github.com/dever-labs/devx/issues/9)) ([2be55f9](https://github.com/dever-labs/devx/commit/2be55f97efa6d4bd8b2262985d3cd91183d6682d))


### Bug Fixes

* **ci:** use PAT for release-please so it can trigger release.yml ([#16](https://github.com/dever-labs/devx/issues/16)) ([38f87e7](https://github.com/dever-labs/devx/commit/38f87e791faeac456b402230a725c055c45f07e1))
* **devcontainer:** add copilot-config volume mount and align AI extensions ([205efcc](https://github.com/dever-labs/devx/commit/205efccbe7eb67f6f203dc012c319db4d19fa639))
* handle case where no profile is specified and no defaultProfile is set in manifest ([202b503](https://github.com/dever-labs/devx/commit/202b5037f7cb6a0036a50b8298360739234028b6))
* install Copilot CLI in devcontainer and fix volume name ([a7c7f86](https://github.com/dever-labs/devx/commit/a7c7f86ea53507c0776931bbf7ff9927f7f31189))
* **release-please:** drop component prefix from tags ([#18](https://github.com/dever-labs/devx/issues/18)) ([e146ab2](https://github.com/dever-labs/devx/commit/e146ab2d728b46a04b9c9e0763c97aa2980dccaa))

## [0.1.1](https://github.com/dever-labs/devx/compare/devx-v0.1.0...devx-v0.1.1) (2026-10-29)


### Features

* add AI deployment generation and provider management ([84a92e0](https://github.com/dever-labs/devx/commit/84a92e0bb3182a2f775a2d483963ae533485c9a2))
* add background hook support with live output streaming ([5067532](https://github.com/dever-labs/devx/commit/5067532837995147cbabb93eb16f775e9a38eaee))
* add Kubernetes support with rendering, applying, and deleting manifests; update documentation and validation ([81977c9](https://github.com/dever-labs/devx/commit/81977c9862becfd014591fa34d25c08b60b95c14))
* add manifest and validation for compose configuration ([a4bddd4](https://github.com/dever-labs/devx/commit/a4bddd4707b3fbe29541a877755c3201c69bee67))
* add setup, validate, doctor --fix, and JSON output ([574bc48](https://github.com/dever-labs/devx/commit/574bc48013eb82e3ae379b2a0502ecacaeed4056))
* add telemetry support with Grafana, Loki, and Prometheus; update commands and documentation ([e84d7bc](https://github.com/dever-labs/devx/commit/e84d7bc07f724633fbdedb02f059a692116deba7))
* AI wizard, devcontainer management, script runner, Homebrew distribution ([15ab075](https://github.com/dever-labs/devx/commit/15ab0757805cf04b0441899ff09b1f58def689cf))
* env, auth, dev logs/ps/shell, ai chat, ci run, new, .env loading, Continue.dev config ([9446e18](https://github.com/dever-labs/devx/commit/9446e18649c04dfa0b6c8f8e9ff10be961378e62))
* replace Profile/Compose/K8s model with flat Task scheduler (Phase 1) ([#9](https://github.com/dever-labs/devx/issues/9)) ([2be55f9](https://github.com/dever-labs/devx/commit/2be55f97efa6d4bd8b2262985d3cd91183d6682d))


### Bug Fixes

* **devcontainer:** add copilot-config volume mount and align AI extensions ([205efcc](https://github.com/dever-labs/devx/commit/205efccbe7eb67f6f203dc012c319db4d19fa639))
* handle case where no profile is specified and no defaultProfile is set in manifest ([202b503](https://github.com/dever-labs/devx/commit/202b5037f7cb6a0036a50b8298360739234028b6))
* install Copilot CLI in devcontainer and fix volume name ([a7c7f86](https://github.com/dever-labs/devx/commit/a7c7f86ea53507c0776931bbf7ff9927f7f31189))

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

[Unreleased]: https://github.com/dever-labs/devx/compare/v0.1.3...HEAD
[0.1.0]: https://github.com/dever-labs/devx/releases/tag/v0.1.0
