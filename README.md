# devx

[![CI](https://github.com/dever-labs/dever/actions/workflows/ci.yml/badge.svg)](https://github.com/dever-labs/dever/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/dever-labs/devx)](https://goreportcard.com/report/github.com/dever-labs/devx)

**devx** is a single-binary, cross-platform dev orchestrator for any repo. Define your services and dependencies once in `devx.yaml`, then spin up identical environments with one command — locally, in CI, or on Kubernetes.

```sh
devx up          # start everything
devx status      # see running services + browser links
devx logs api    # tail logs
devx down        # tear down
```

## Installation

**Linux / macOS**
```sh
curl -fsSL https://raw.githubusercontent.com/dever-labs/dever/main/scripts/install.sh | sh
```

**Windows (PowerShell)**
```powershell
iwr https://raw.githubusercontent.com/dever-labs/dever/main/scripts/install.ps1 | iex
```

**npm**
```sh
npm install -g @dever-labs/devx
```

**Homebrew**
```sh
brew tap dever-labs/tap https://github.com/dever-labs/homebrew-tap
brew install devx
```

**Chocolatey / WinGet**
```powershell
choco install devx
winget install dever-labs.devx
```

Pre-built binaries for all platforms are also available on the [Releases](https://github.com/dever-labs/dever/releases) page — download, make executable, and place on your `PATH`.
See [docs/install.md](docs/install.md) for all methods including step-by-step manual setup.

## Quickstart

```sh
devx init               # scaffold a starter devx.yaml
# edit devx.yaml to match your repo
devx up                 # start services + telemetry stack
```

After startup, devx prints the URLs of every published service:

```
Environment is up

Available services:
  Api      http://localhost:8080
  Grafana  http://localhost:54231
```

## Commands

| Command | Description |
|---|---|
| `devx init` | Scaffold a starter `devx.yaml` in the current directory |
| `devx up` | Start all services for the active profile |
| `devx down` | Stop and remove containers |
| `devx status` | Show running containers, state, and published ports |
| `devx logs [service]` | Stream logs from one or all services |
| `devx exec <service> -- <cmd>` | Run a command inside a running service |
| `devx doctor` | Check runtime prerequisites |
| `devx render compose` | Print the generated Docker Compose file |
| `devx render k8s` | Render Kubernetes manifests from a profile |
| `devx lock update` | Resolve and pin image digests to `devx.lock` |

### Flags

**`devx up`**
- `--profile <name>` — select a profile (default: `defaultProfile` in devx.yaml)
- `--build` — rebuild images before starting
- `--pull` — always pull latest images
- `--no-telemetry` — skip the built-in observability stack

**`devx down`**
- `--volumes` — also remove named volumes

**`devx logs`**
- `--follow` — stream live
- `--since <duration>` — e.g. `10m`, `1h`
- `--json` — emit each line as a JSON object

**`devx render compose`**
- `--write` — write output to `.devx/compose.yaml` instead of stdout
- `--no-telemetry` — exclude telemetry services

**`devx render k8s`**
- `--profile <name>` — profile to render
- `--namespace <ns>` — Kubernetes namespace
- `--write` — write to `.devx/k8s.yaml`

**`devx doctor`**
- `--fix` — attempt to auto-fix detected issues

## devx.yaml reference

See [docs/manifest.md](docs/manifest.md) for the full schema and all supported fields.

## Built-in telemetry stack

devx starts a full observability stack alongside your services:

| Component | Role |
|---|---|
| **Grafana** | Dashboard UI (published on a random port) |
| **Loki** | Log aggregation |
| **Prometheus** | Metrics collection |
| **Grafana Alloy** | Log shipping from Docker containers |
| **cAdvisor** | Container CPU and memory metrics |
| **docker-meta exporter** | Per-container network metrics + label enrichment |

Four pre-built dashboards are provisioned automatically:

- **Container Logs** — full-text search across all services with service filter
- **Container Resources** — CPU %, memory, and per-container network Rx/Tx
- **Log Analytics** — error/warn trends, log volume, error rate over time
- **Service Health** — active services, error counts, top consumers

See [docs/telemetry.md](docs/telemetry.md) for details.

Disable with `devx up --no-telemetry`.

## Profiles

Profiles let you define different environments in the same file:

```yaml
profiles:
  local:    # devx up --profile local
  ci:       # devx up --profile ci
  k8s:      # devx up --profile k8s  (rendered to Kubernetes manifests)
    runtime: k8s
```

The `defaultProfile` in `project` is used when `--profile` is omitted.

## Kubernetes

Render a profile to Kubernetes manifests:

```sh
devx render k8s --profile k8s --write   # writes .devx/k8s.yaml
devx up --profile k8s                   # kubectl apply
devx down --profile k8s                 # kubectl delete
```

See [docs/manifest.md#kubernetes](docs/manifest.md#kubernetes) for constraints.

## Offline / airgapped

1. Set `registry.prefix` in devx.yaml (e.g. `myregistry.azurecr.io`).
2. Run `devx lock update` while you have registry access — this writes `devx.lock` with image digests.
3. Commit `devx.lock`. On airgapped machines `devx up` uses digest-pinned images automatically.

## Generated files

All runtime artifacts are written to `.devx/` (gitignored):

| Path | Contents |
|---|---|
| `.devx/compose.yaml` | Generated Docker Compose file |
| `.devx/state.json` | Active profile, runtime, and telemetry state |
| `.devx/telemetry/` | Grafana dashboards, Prometheus config, Alloy config |

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on development setup, testing, and submitting pull requests.

## License

[MIT](LICENSE) © dever-labs

