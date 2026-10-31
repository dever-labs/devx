# Changelog

All notable changes to devx will be documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.0.1](https://github.com/dever-labs/devx/compare/v1.0.0...v1.0.1) (2026-10-31)


### Bug Fixes

* **ci:** publish devx Feature to ghcr.io/dever-labs/features/devx ([#75](https://github.com/dever-labs/devx/issues/75)) ([f4b0c08](https://github.com/dever-labs/devx/commit/f4b0c0814249d211e9b8f060cc2a0c9a6afc0501))
* **ci:** rename Feature id to 'cli' to avoid devx/devx path duplication ([#77](https://github.com/dever-labs/devx/issues/77)) ([af53e1c](https://github.com/dever-labs/devx/commit/af53e1c4ee29a0d5dbc0ae194837cb73a9e0cbee))

## [1.0.0](https://github.com/dever-labs/devx/compare/v0.1.7...v1.0.0) (2026-10-31)


### Bug Fixes

* **ci:** log in to GHCR before publishing features (403 bearer token) ([#67](https://github.com/dever-labs/devx/issues/67)) ([7db6c56](https://github.com/dever-labs/devx/commit/7db6c56324118192869cd8affcd4adf9d28eb3ea))


### Continuous Integration

* Feature shares the devx version via release-please ([#70](https://github.com/dever-labs/devx/issues/70)) ([7740ad7](https://github.com/dever-labs/devx/commit/7740ad715ade88e5151bd0ebb5a97dc372904da2))

## [0.1.7](https://github.com/dever-labs/devx/compare/v0.1.6...v0.1.7) (2026-10-31)


### Bug Fixes

* **task:** allow one-shot containers without a parseable name ([#65](https://github.com/dever-labs/devx/issues/65)) ([3a3245f](https://github.com/dever-labs/devx/commit/3a3245f6044885ddf7e6c0589e6fe8a2c80bef34)), closes [#58](https://github.com/dever-labs/devx/issues/58)

## [0.1.6](https://github.com/dever-labs/devx/compare/v0.1.5...v0.1.6) (2026-10-31)


### Bug Fixes

* resolve migration-feedback issues [#55](https://github.com/dever-labs/devx/issues/55)-[#61](https://github.com/dever-labs/devx/issues/61) ([#62](https://github.com/dever-labs/devx/issues/62)) ([59cb6da](https://github.com/dever-labs/devx/commit/59cb6da5008a74def72ee8023c43f50a58451d51))

## [0.1.5](https://github.com/dever-labs/devx/compare/v0.1.4...v0.1.5) (2026-10-30)


### Bug Fixes

* handle docker present-but-unreachable CI flake; fix publish-features invalid tag ([#51](https://github.com/dever-labs/devx/issues/51)) ([237ac78](https://github.com/dever-labs/devx/commit/237ac7892c65801b814a39eb11130c03ff6967d5))
* make test fixtures and env lookups Windows-compatible ([#45](https://github.com/dever-labs/devx/issues/45)) ([8044fb4](https://github.com/dever-labs/devx/commit/8044fb46a7498951660f648af7117cce28ba93f3))
* toolchain, security, timeout, and validation gaps from repo review ([#47](https://github.com/dever-labs/devx/issues/47)) ([0b30403](https://github.com/dever-labs/devx/commit/0b30403d4426e09e7696ad08c2e1366f4c2d8798))

## [0.1.4](https://github.com/dever-labs/devx/compare/v0.1.3...v0.1.4) (2026-10-29)


### Features

* envFiles: local config/secrets scaffolding block ([#43](https://github.com/dever-labs/devx/issues/43)) ([f87c2d0](https://github.com/dever-labs/devx/commit/f87c2d0fb675e091cc4c4232fcf91cec56bb80cf))
* JS/TS package-manager + workspace auto-detection in devx init ([#41](https://github.com/dever-labs/devx/issues/41)) ([8371267](https://github.com/dever-labs/devx/commit/8371267c57a7d01c6e7df8bb9c09607ca13f7b3a))
* Node version + Corepack version-manager-aware installs in devx init ([#44](https://github.com/dever-labs/devx/issues/44)) ([19a3bc5](https://github.com/dever-labs/devx/commit/19a3bc5ea6b9f5f4593e3e3fe2d47403a2598ee4))
* VS Code attach-debugging launch.json + debugging docs ([#40](https://github.com/dever-labs/devx/issues/40)) ([b34b4d9](https://github.com/dever-labs/devx/commit/b34b4d9d1df43428d87affa9ac43d06d4ec0f825))


### Bug Fixes

* **ci:** enforce gofmt and reformat 7 drifted files ([#34](https://github.com/dever-labs/devx/issues/34)) ([4516b07](https://github.com/dever-labs/devx/commit/4516b07e91f0430bffa782fdc95c42fd4e588129))
* **docs:** remove stale hand-written Unreleased changelog section ([#31](https://github.com/dever-labs/devx/issues/31)) ([00f8272](https://github.com/dever-labs/devx/commit/00f82726b35bd4cc3c0c2e229961d4eb3cc766be))
* **security:** verify checksums.txt before devx update replaces the binary ([#35](https://github.com/dever-labs/devx/issues/35)) ([15bd377](https://github.com/dever-labs/devx/commit/15bd37714e1d8a086d7d15718e4f824af149d261))

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
