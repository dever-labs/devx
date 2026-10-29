# devx

[![CI](https://github.com/dever-labs/devx/actions/workflows/ci.yml/badge.svg)](https://github.com/dever-labs/devx/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/dever-labs/devx)](https://goreportcard.com/report/github.com/dever-labs/devx)

**devx** is a single-binary, cross-platform dev task orchestrator. Define
your dev tasks once as a flat, dependency-ordered graph in `devx.yaml`, then
run them identically — locally, in CI, or inside a devcontainer — with one
command.

```sh
devx up          # start the task graph, respecting deps:
devx status      # see what's running
devx logs api    # tail a task's output
devx down        # stop everything
```

Unlike Compose/Helm-style tools, `devx.yaml` tasks are plain shell commands —
`run: docker run ...`, `run: npm run dev`, `run: dotnet watch run`, anything.
devx doesn't own or render your infrastructure; it just sequences and
supervises whatever commands you already use.

## Installation

**Linux / macOS**
```sh
curl -fsSL https://raw.githubusercontent.com/dever-labs/devx/main/scripts/install.sh | sh
```

**Windows (PowerShell)**
```powershell
iwr https://raw.githubusercontent.com/dever-labs/devx/main/scripts/install.ps1 | iex
```

**Homebrew (macOS / Linux)**
```sh
brew tap dever-labs/tap https://github.com/dever-labs/homebrew-tap
brew install --cask devx
```

**Dev Containers**
```json
"features": {
    "ghcr.io/dever-labs/devx/devx:1": {}
}
```

Pre-built binaries for all platforms are also available on the
[Releases](https://github.com/dever-labs/devx/releases) page — download, make
executable, and place on your `PATH`.
See [docs/install.md](docs/install.md) for details and building from source.

## Quickstart

```sh
devx init               # interactive scaffold: devx.yaml (+ optional devcontainer)
# edit devx.yaml to match your repo — add/remove tasks as needed
devx up                  # start the task graph
```

```
[db] running
[db] ready
[api] running
[api] ready

Environment is up
```

## Commands

| Command | Description |
|---|---|
| `devx init` | Interactive scaffold: `devx.yaml` (+ optional devcontainer) |
| `devx setup [--fix] [--json] [--no-prompt]` | Install required tools, run host-side setup steps, and scaffold `envFiles:` |
| `devx up [<name...>] [-d\|--detach] [--dry-run] [--keep-going] [--group <name>]` | Bring up the task graph (or a partial closure) |
| `devx run <name...> [--dry-run] [--watch]` | Run task(s) directly, respecting `deps:` |
| `devx <name>` | Shorthand for `devx run <name>` |
| `devx down [--force]` | Stop every running detached task |
| `devx status [--json]` | Task + project + devcontainer + AI status |
| `devx logs [name] [--follow]` | Tail captured output for one or all running tasks |
| `devx exec <name> -- <cmd...>` | Run a command inside a running task's container |
| `devx tasks [--json]` | Dump the full resolved task list |
| `devx doctor [--fix] [--json]` | Health-check tools, `requires:`, and port collisions |
| `devx validate [--file <path>]` | Validate `devx.yaml` |
| `devx clone <repo> [dir]` | Clone a repo and run `devx setup` |
| `devx new <name> [--run <cmd>]` | Append a minimal task skeleton to `devx.yaml` |
| `devx ai` / `devx ai chat`, `setup`, `status`, `reset` | AI setup wizard / one-off question / config |
| `devx dev` | Devcontainer status |
| `devx dev build\|rebuild [image] [--no-cache]` | Build/rebuild the devcontainer image |
| `devx dev up [--recreate]` | Start the devcontainer |
| `devx dev open\|exec\|shell` | Open an editor / run a command / shell into the devcontainer |
| `devx dev logs [--follow]`, `devx dev ps` | Devcontainer logs / process status |
| `devx env [--show]` | Show/validate env vars from devcontainer + `.env` |
| `devx auth` | Check auth status for GitHub, Anthropic, OpenAI |
| `devx ci [--write] [--stack ...]` / `devx ci run [--job <name>]` | Scaffold / locally run CI (via `act`) |
| `devx update [--check]` | Update devx to the latest version |
| `devx completion bash\|zsh\|fish` | Generate shell completions |
| `devx version` | Print the devx version |

Run `devx` with no arguments for the full, always-current command list.

## Tools and setup

devx can check for and install required SDKs/CLIs, then run host-side setup
steps before any tasks start.

```yaml
tools:
  - name: node
    version: "20"
    check: "node --version"
    install:
      windows: "winget install OpenJS.NodeJS.LTS"
      macos: "brew install node@20"
      linux: "curl -fsSL https://deb.nodesource.com/setup_20.x | sudo bash - && apt-get install -y nodejs"

setup:
  - name: restore-frontend
    run: "npm install"
    workdir: ./frontend
    runOnce: true   # skipped on later runs if the command hasn't changed
```

```sh
devx doctor         # check runtime + declared tools
devx doctor --fix   # install missing tools declared in tools:
devx setup          # run pending setup: steps
devx setup --fix    # also install missing tools + re-run runOnce steps
```

## devx.yaml reference

`devx.yaml` is a **flat map of tasks** — there is no `services:`/`profiles:`
wrapper. Every top-level key other than a small reserved set (`version`,
`project`, `ai`, `tools`, `setup`, `vars`, `includes`, `agents`) *is* a task
name:

```yaml
version: 1

project:
  name: my-app

vars:
  DB_PORT: "5432"

db:
  desc: Postgres for local dev
  run: docker run --rm -d --name my-app-db -p ${DB_PORT}:5432 postgres:16
  ready:
    tcp: localhost:${DB_PORT}

api:
  desc: API server
  deps: [db]
  dir: ./src/api
  run: docker build -t my-app-api . && docker run --rm -d --name my-app-api -p 8080:80 my-app-api
  ready:
    http: http://localhost:8080/
```

See [docs/manifest.md](docs/manifest.md) for the full field reference
(`deps`, `ready`, `env`, `restart`, `sources`/`generates`, `includes`, and
more).

## Devcontainer integration

`devx init` can scaffold a `.devcontainer/devcontainer.json` alongside your
`devx.yaml`. `devx dev up`/`devx dev open`/`devx dev exec` wrap the
[devcontainer CLI](https://github.com/devcontainers/cli) so AI agents and
humans share the same entry points for starting and entering the container.

## Generated files

Runtime state is written to `.devx/` (gitignored):

| Path | Contents |
|---|---|
| `.devx/tasks.json` | Task runtime registry (running/detached task state) |
| `.devx/setup-state.json` | `runOnce` setup-step hashes |
| `.devx/ai.yaml` | Saved AI provider configuration |

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for
guidelines on development setup, testing, and submitting pull requests.

## License

[MIT](LICENSE) © dever-labs
