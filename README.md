# devx

[![CI](https://github.com/dever-labs/devx/actions/workflows/ci.yml/badge.svg)](https://github.com/dever-labs/devx/actions/workflows/ci.yml)
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
curl -fsSL https://raw.githubusercontent.com/dever-labs/devx/main/scripts/install.sh | sh
```

**Windows (PowerShell)**
```powershell
iwr https://raw.githubusercontent.com/dever-labs/devx/main/scripts/install.ps1 | iex
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

Pre-built binaries for all platforms are also available on the [Releases](https://github.com/dever-labs/devx/releases) page — download, make executable, and place on your `PATH`.
See [docs/install.md](docs/install.md) for all methods including step-by-step manual setup.

## Quickstart

```sh
devx init               # scaffold a starter devx.yaml
# edit devx.yaml to match your repo
devx up                 # start services (add --telemetry for the Grafana stack)
```

After startup, devx prints the URLs of every published service:

```
Environment is up

Available services:
  Api      http://localhost:8080
  Grafana  http://localhost:54231   # only with --telemetry
```

## Commands

| Command | Description |
|---|---|
| `devx init` | Interactive project scaffold (`devx.yaml` + devcontainer) |
| `devx setup` | Install required tools and run host-side setup steps |
| `devx run [script]` | Run a script defined in `devx.yaml` |
| `devx clone <repo> [dir]` | Clone a repo and run `devx setup` |
| `devx up` | Start all services for the active profile |
| `devx down` | Stop containers and host processes |
| `devx status` | Show project, devcontainer, and AI status |
| `devx logs [service]` | Stream logs from one or all services |
| `devx exec <service> -- <cmd>` | Run a command inside a running service |
| `devx doctor` | Health-check tools, backends, and config |
| `devx validate` | Validate `devx.yaml` schema and configuration |
| `devx render compose\|k8s` | Render the Docker Compose file or Kubernetes manifests |
| `devx lock update` | Resolve and pin image digests to `devx.lock` |
| `devx providers install\|list` | Manage third-party dependency providers declared in `devx.yaml` |
| `devx export --format <fmt>` | Export a profile as `compose`, `k8s`, `helm`, or `terraform` |
| `devx ai` | AI setup wizard, or launch the configured AI tool |
| `devx ai chat [question]` | One-off AI question |
| `devx ai setup\|status\|reset` | Configure, inspect, or reset the local/cloud AI backend |
| `devx ai model list\|pull\|remove` | Manage local AI models |
| `devx debug-config [--write]` | Generate VS Code attach configs for services with a `debug` block |
| `devx dev` | Show devcontainer status |
| `devx dev build\|rebuild\|up\|open\|exec\|shell\|logs\|ps` | Manage the devcontainer lifecycle |
| `devx env [--show]` | Show/validate env vars from the devcontainer + `.env` |
| `devx auth` | Check auth status for GitHub, Anthropic, OpenAI |
| `devx new service\|dep` | Add a service or dependency to `devx.yaml` |
| `devx ci [--write] [--stack ...]` | Generate a GitHub Actions CI workflow for the detected stack |
| `devx ci run [--job <name>]` | Run CI locally with [`act`](https://github.com/nektos/act) |
| `devx update [--check]` | Update devx to the latest version |
| `devx completion bash\|zsh\|fish` | Generate shell completions |
| `devx version` | Print the devx version |

Run `devx help` for the full usage text with all flags.

### Flags

**`devx setup`**
- `--fix` — install missing tools, re-run `runOnce` steps, regenerate `envFiles`
- `--non-interactive` — never prompt; fail listing the missing values
- `--json` — emit results as JSON (AI/CI friendly)

**`devx doctor`**
- `--fix` — install missing tools declared in the `tools` block
- `--json` — emit report as JSON

**`devx validate`**
- `--file <path>` — path to `devx.yaml` (default: `./devx.yaml`)
- `--json` — emit `{"valid": bool, "issues": [...]}` (AI/CI friendly)

**`devx auth`**
- `--json` — emit auth status as a JSON array (AI/CI friendly)

**`devx up`**
- `--profile <name>` — select a profile (default: `defaultProfile` in devx.yaml)
- `--build` — rebuild images before starting
- `--pull` — always pull latest images
- `--telemetry` — also start the built-in observability stack (off by default)
- `--attach <service>` — run a process service in the foreground with your terminal attached (debuggers, REPLs)

**`devx down`**
- `--volumes` — also remove named volumes

**`devx logs`**
- `--follow` — stream live
- `--since <duration>` — e.g. `10m`, `1h`
- `--json` — emit each line as a JSON object

**`devx render compose`**
- `--write` — write output to `.devx/compose.yaml` instead of stdout
- `--telemetry` — include telemetry services (off by default)

**`devx render k8s`**
- `--profile <name>` — profile to render
- `--namespace <ns>` — Kubernetes namespace
- `--write` — write to `.devx/k8s.yaml`

**`devx export`**
- `--format <fmt>` — required; one of `compose`, `k8s`, `helm`, `terraform`
- `--profile <name>` — profile to export
- `--out <dir>` — output directory (default: `.`)

**`devx status`**
- `--json` — emit report as JSON

**`devx env`**
- `--show` — reveal secret values instead of masking them

**`devx ci`**
- `--write` — write `.github/workflows/ci.yml` instead of printing to stdout
- `--stack <name>` — override stack detection (`dotnet`, `node`, `go`, `python`)

**`devx ci run`**
- `--job <name>` — run a specific job with `act` instead of the whole workflow

**`devx update`**
- `--check` — only check for a newer version, don't install it

## Running processes without containers

Services can be host processes instead of containers, so repos that just run `yarn dev` work with the same `up/status/logs/down/exec` commands. Containers and processes can be mixed in one profile; devx orders them by `dependsOn`, waits for each `ready` probe, and `devx up` returns once everything is ready.

```yaml
services:
  web:
    kind: process
    run: yarn dev
    workdir: ./packages/web
    ports: ["3000"]
    dependsOn: [db]
    ready: { http: http://localhost:3000/health }
    debug: { port: 9229, type: node }
```

Logs go to `.devx/logs/<service>.log`; state to `.devx/processes.json`. See [docs/manifest.md](docs/manifest.md#process-services) and `examples/process/`.

## Tools and setup

devx can install required SDKs and run host-side setup steps before you start containers.

### Declaring required tools

Add a `tools` block to `devx.yaml`. devx checks each tool with its `check` command and installs it using the platform-appropriate `install` command when `devx setup --fix` or `devx doctor --fix` is run.

```yaml
tools:
  - name: node
    version: "20"
    check: "node --version"
    install:
      windows: "winget install OpenJS.NodeJS.LTS"
      macos: "brew install node@20"
      linux: "curl -fsSL https://deb.nodesource.com/setup_20.x | sudo bash - && apt-get install -y nodejs"

  - name: dotnet
    version: "8"
    check: "dotnet --version"
    install:
      windows: "winget install Microsoft.DotNet.SDK.8"
      macos: "brew install dotnet@8"
      linux: "wget https://dot.net/v1/dotnet-install.sh -O install.sh && bash install.sh --version 8.0"
```

Tools can be minimal. For node, npm, yarn, pnpm, python, go and dotnet, `check` is optional and `version` is detected from the repo (`.nvmrc`, `.node-version`, `package.json` `engines`/`packageManager`, `global.json`, `go.mod`, `.tool-versions`, `.python-version`). An explicit value in `devx.yaml` always wins; `devx doctor` warns when it conflicts with the detected one.

```yaml
tools:
  - name: node   # version from .nvmrc
  - name: yarn   # version from package.json#packageManager
```

`devx setup --fix` installs through volta, fnm, nvm or corepack when present. Set `installer: system` on a tool to use only its `install` block.

### Declaring setup steps

Add a `setup` block to `devx.yaml` for host-side commands that run after tools are installed. Steps with `runOnce: true` are skipped on subsequent runs if their command hasn't changed (tracked in `.devx/setup-state.json`).

```yaml
setup:
  - name: restore-frontend
    run: "npm install"
    workdir: ./frontend
    runOnce: true         # skip if already run and command unchanged

  - name: restore-backend
    run: "dotnet restore"
    workdir: ./src
    runOnce: true

  - name: db-migrate
    run: "dotnet ef database update --project src/Api"
    platform: all         # all | windows | linux | macos (default: all)
```

In a JS monorepo use `workspace: auto` to run a single root install (`yarn install`, `pnpm install` or `npm ci`, chosen from `packageManager`/lockfiles). With no `setup` block at all, devx does this automatically when it detects a workspace.

```yaml
setup:
  - name: install
    workspace: auto
```

### Local config and secrets

`envFiles` generates gitignored local config from a template. Values come from env vars, commands (`op read`, `vault kv get`, ...) or prompts. Existing files are never overwritten unless `--fix`; secret values never appear in logs or `--json` output.

```yaml
envFiles:
  - file: app-config.local.yaml
    template: app-config.local.yaml.tmpl   # ${KEY} placeholders; omit for KEY=value lines
    values:
      GITHUB_TOKEN: { from: env, name: GITHUB_TOKEN }
      DB_PASS:      { from: command, run: "op read op://dev/db/pass" }
      SESSION_KEY:  { from: prompt, secret: true, default: generate }
```

In non-interactive mode (CI, `--json`, `--non-interactive`) missing values fail with a list of the keys.

### Running setup

```sh
devx setup          # check tools + run pending steps
devx setup --fix    # also install missing tools + re-run runOnce steps
devx setup --json   # machine-readable output (for AI/CI)
```

### Doctor workflow

```sh
devx doctor         # check container runtime + declared tools
devx doctor --fix   # attempt to install any missing tools
devx doctor --json  # structured JSON report for AI agents
```

## devx.yaml reference

See [docs/manifest.md](docs/manifest.md) for the full schema and all supported fields.

## Built-in telemetry stack

devx can start a full observability stack alongside your services (opt-in with `devx up --telemetry`):

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

Telemetry is off by default. Enable it with `devx up --telemetry`.

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

