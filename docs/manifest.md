# devx.yaml reference

`devx.yaml` is a flat manifest of **tasks**. There is no `services:`/`deps:`/`profiles:`
wrapper — every top-level key other than a small reserved set (`version`, `project`, `ai`,
`tools`, `setup`, `vars`, `includes`, `agents`) *is* a task name, and its value is that
task's definition.

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

Run it with `devx up` (starts every task, respecting `deps:`), or a single task and its
dependencies with `devx run api`. See `devx help` for the full command list.

---

## Top-level structure

| Key | Type | Description |
|---|---|---|
| `version` | int | Always `1`. |
| `project.name` | string | Project name. |
| `vars` | map | Default `${VAR}` substitution values (see [Variable substitution](#variable-substitution)). |
| `default` | list | Task names and/or `group:` names run by a bare `devx up` (no explicit task names or `--group`). Each entry must match a known task name or a `group:` value used by at least one task — `devx validate` rejects anything else. Omitted (the default): bare `devx up` still runs every task, unchanged. |
| `includes` | map | Merge another `devx.yaml`'s tasks into this graph (see [Includes](#includes)). |
| `agents` | map | Named shell commands for the planned `devx workspace` feature (parallel per-branch workspaces with an AI coding agent auto-started on creation). Reserved and parsed today; not yet wired to a CLI command — see `docs/design/task-executor-refactor.md`. |
| `ai` | object | Optional LLM provider config used by `devx ai`. See `docs/ai.md`-equivalent comments in `schemas/devx.schema.json`. Credentials always come from environment variables, never from `devx.yaml`. |
| `tools` | list | Required CLI tools/SDKs, checked by `devx doctor`, installed by `devx init`. |
| `setup` | list | Ordered host-side commands run by `devx init`. |
| `envFiles` | list | Local config/secrets files scaffolded from a template by `devx init` (see [Scaffolding local config/secrets](#scaffolding-local-configsecrets)). |
| *(anything else)* | task | A task definition — see below. |

---

## Tasks

A task is either a bare string (shorthand for `{run: <string>}`) or a mapping with any of
the fields below.

```yaml
worker: ./worker --port 9000
```

is shorthand for:

```yaml
worker:
  run: ./worker --port 9000
```

(a plain string task and a one-field mapping are equivalent — the shorthand just saves the
`run:` key for the simplest cases).

### Field reference

| Field | Type | Description |
|---|---|---|
| `run` | string | Shell command, executed via the system shell (`sh -c` on Linux/macOS, `cmd /C` on Windows). |
| `desc` | string | One-line description shown in `devx tasks`. Supports `${VAR}` substitution, same as `run:`/`env:`/`dir:` (falls back to the raw, unsubstituted text if a referenced variable is undefined, since `desc:` is purely informational and shouldn't be able to break `devx tasks`). |
| `deps` | list | Task names that must complete (one-shot) or become ready (detached) before this task starts. `devx up`/`devx run <name>` compute the transitive closure and run independent tasks in parallel, level by level. |
| `dir` | string | Working directory for `run:`, relative to the `devx.yaml` that defines the task. Defaults to that file's own directory. |
| `env` | map | Environment variables merged into the task's process environment (after `${VAR}` substitution). |
| `detached` | bool | `true` starts the task in the background and moves on once it's ready, instead of waiting for it to exit. Inferred from `run:` (e.g. `docker run -d`, a trailing `&`, or a long-running dev-server pattern) when omitted — set explicitly to override a wrong guess. |
| `interactive` | bool | `true` attaches the task directly to the terminal's stdin/stdout/stderr (e.g. a shell, a REPL) — only valid when running that one task alone (`devx run <name>`), not as part of a larger `devx up`. Cannot be combined with `detached: true`. |
| `ready` | object | How devx decides a detached task has become ready — see [Readiness](#readiness). |
| `port` | int | Port this task listens on, used for `devx validate`'s port-collision check. Inferred from `run:` (`-p`, `--port`, `--publish`) when omitted. |
| `containerName` | string | Overrides the container name devx infers from `run:`, used by `devx down`/`logs`/`exec`/`status` to address the right container. Set explicitly if the name can't be parsed automatically. If a container with this name already exists from an earlier, untracked `devx up` (e.g. devx was killed before recording state), devx adopts it in place if it's still running, or removes it first (`rm -f`) if it's stopped — either way, `devx up` never fails with "name already in use". |
| `requires` | list | Binaries that must be on `PATH` for this task to run — checked by `devx doctor`. |
| `sources` / `generates` | list of globs | Skip re-running a one-shot task when every `generates:` file is newer than every `sources:` file (like a Makefile rule). `generates:` is invalid on a detached task. |
| `os` | list | Restrict this task to `linux`/`darwin`/`windows` (`macos` accepted as an alias for `darwin`); skipped (not an error) on any other OS. |
| `timeout` | duration string | Kill the task and treat it as failed if it runs longer than this (e.g. `"30s"`, `"2m"`). |
| `stopTimeout` | duration string | How long `devx down` waits after `SIGTERM` before `SIGKILL`. Defaults to `10s`. |
| `restart` | `never` \| `on-failure` \| `always` | Only meaningful on a detached task. Defaults to `never`. **Reserved for a later phase**: devx validates and persists this field today, but does not yet watch a detached task's process/container for exit and restart it — a crashed task simply stays dead regardless of `restart:`. |
| `shell` | string | Override the shell used to execute `run:`. |
| `group` | string | Logical group name, used by `devx up --group <name>` to start a subset of the graph. |
| `conflicts` | list | Other task names this one is intentionally mutually exclusive with (e.g. dev vs. prod variants sharing a port). Suppresses `devx validate`'s port-collision warning for declared pairs; each entry must match a known task name. |
| `scope` | `workspace` \| `shared` | Defaults to `workspace`. A `shared` task may not reference `${workspace}`/`${port}`-style per-workspace substitutions, since it's meant to be started once and reused across workspaces (see the workspace/agents design). |

### Readiness

```yaml
ready:
  http: http://localhost:8080/health   # polled until it returns 2xx/3xx
  # or
  tcp: localhost:5432                  # polled until the socket accepts a connection
  # or
  cmd: docker inspect -f '{{.State.Running}}' my-app-db   # polled until it exits 0
```

Set at most one of `http`/`tcp`/`cmd`. When `ready:` is omitted entirely, devx infers a
check from the task's `run:` text (an exposed port implies a TCP check against
`localhost:<port>`). If nothing can be inferred and the task is detached, `devx validate`
warns — add `ready:` or `port:` explicitly.

**Docker-outside-of-Docker (DooD) fallback:** if devx itself is running inside a
container sharing the host's Docker socket (e.g. a devcontainer), an `http`/`tcp` check
addressed at `localhost`/`127.0.0.1` may fail even though the target container is healthy
— the host-published port isn't reachable as "localhost" from a sibling container. For a
container-shaped, named task (`containerName:` set or inferred), devx automatically
retries a failed `http`/`tcp` check once against the container's own network IP (and its
internal port, when parseable from a `-p hostPort:containerPort` mapping) before giving
up. This is fully automatic — no manifest changes needed — and only applies to `http`/
`tcp`; a `cmd:` check is never rewritten, since you already fully control it.

### Variable substitution

`${VAR}` in `run:`, `env:`, `ready:`, `dir:`, etc. is substituted before the task executes,
in this precedence order (highest wins):

1. `--var KEY=value` on the CLI (`devx up --var DB_PORT=5433`)
2. The inherited process environment
3. `.env` / `.env.local` in the project root
4. The manifest's `vars:` block (or, for an included file's tasks, that file's own `vars:`
   merged over the root's `vars:` — the included file's value wins on conflict)

`$$` escapes to a literal `$`. An unbraced `$VAR` (no braces) is left untouched — only
`${VAR}` is substituted, so `run: echo $HOME` passes `$HOME` straight through to the shell
for the shell itself to expand, while `run: echo ${API_PORT}` is substituted by devx first.

### Includes

```yaml
includes:
  infra: ./infra/devx.yaml
```

merges every task from `./infra/devx.yaml` into this graph, namespaced as `infra:<task>`
(e.g. `infra:db`). Reference it from `deps:` the same way: `deps: [infra:db]`. This is a
flat merge, not a recursive sub-process — the included file's own `includes:` (if any) are
not followed. Each included task's `${VAR}` substitution uses *that file's own* `vars:`
merged over the root's `vars:` (the included file's value wins on conflict), so a shared
`infra/devx.yaml` can be included by multiple projects with different root-level overrides.

---

## Tools and setup

```yaml
tools:
  - name: node
    check: node --version
    install:
      macos: brew install node
      linux: sudo apt-get install -y nodejs
      windows: choco install nodejs

setup:
  - name: install dependencies
    run: npm install
    runOnce: true
```

`devx doctor` checks every `tools:` entry's `check:` command and reports missing tools;
`devx init` (or `devx doctor --fix`) runs each tool's `install:` command for the current
OS, then runs `setup:` steps in order. A `runOnce: true` step is skipped on a later
`devx init` if its `run:` command's content hasn't changed since the last successful run.

`devx init` auto-detects a JS/TS package manager and workspace layout from an existing
`package.json` (`packageManager` field, lockfile, or `workspaces`/`pnpm-workspace.yaml`) and
writes a single root-level `setup:` install step for the detected manager (`npm install`,
`yarn install`, or `pnpm install`) — this covers the common "install once at the workspace
root" pattern for yarn/npm/pnpm monorepos without per-package `run:`/`cwd:` boilerplate.

For the `node` stack, `devx init` also auto-detects a pinned tool version so `devx.yaml`
doesn't duplicate (and risk drifting from) the real source of truth:

- **Node version**, from `.nvmrc`, `.node-version`, or `package.json`'s `engines.node` field
  (checked in that priority order), generates a `tools:` entry pinning `node`'s `version:`.
- **Package manager version**, from `package.json`'s Corepack `packageManager` field (e.g.
  `"yarn@4.1.0"`), generates a second `tools:` entry when Corepack is installed locally.

When a Node version manager (`volta`, `fnm`, or `nvm`) is already installed, the generated
`install:` commands activate the pinned version through it (`volta install node@20.11.0`,
`fnm install 20.11.0`, ...) instead of mutating the global toolchain via `brew`/`apt`/`choco`
— the same way Corepack (`corepack prepare yarn@4.1.0 --activate`) is preferred for the
package manager over installing it globally. With no version manager installed, generated
commands fall back to the OS package manager, same as a hand-written `tools:` entry.

---

## Debugging a process task

`devx up` doesn't swallow a task's stdout/stderr — a detached task's output always lands in
`.devx/logs/<task>.log`, readable live via `devx logs <task> -f`, and setting
`interactive: true` on a task attaches your terminal's stdin/stdout/stderr directly. Either
way, a debugger inspector port (e.g. Node's `--inspect`) is a plain TCP listener independent
of stdio, so wrapping a task in devx doesn't block attaching to it.

`devx init` can optionally scaffold a `.vscode/launch.json` with an **attach** configuration
so pressing F5 connects to a devx-managed process:

- **node**: add `--inspect` (or `--inspect-brk`) to the task's `run:` command; the generated
  config attaches on port `9229`.
- **go**: wrap the task's `run:` command with
  `dlv exec --headless --listen=:2345 --api-version=2 -- <your binary>`; the generated
  config attaches to Delve on port `2345`.

Other stacks (.NET, Python) aren't templated yet — `devx init` reports this and skips the
file rather than guessing a wrong debug-adapter shape.

---

## Scaffolding local config/secrets

```yaml
envFiles:
  - name: local app config
    template: app-config.local.yaml.tmpl
    output: app-config.local.yaml
    # mode: merge   # append new keys to an existing output file instead of skipping it
    vars:
      - name: DB_PASSWORD
        envVar: DB_PASSWORD      # checked first; defaults to the var's name
        prompt: "Database password"
        secret: true             # documentation only — see note below
      - name: API_URL
        default: "http://localhost:8080"
```

```
# app-config.local.yaml.tmpl
database:
  password: {{.DB_PASSWORD}}
api:
  url: {{.API_URL}}
```

`devx init` renders each `envFiles:` entry's `template:` (a Go
[`text/template`](https://pkg.go.dev/text/template) file — placeholders are
`{{.VAR_NAME}}`) to `output:`, resolving every declared var in priority order:

1. an OS environment variable (`envVar:`, defaulting to the var's `name:`)
2. an interactive terminal prompt (`prompt:`), when `devx init` is running in a real
   terminal — skipped entirely in `--json` mode, with `--no-prompt`, or when stdin isn't
   a terminal (e.g. CI), so a missing value fails fast instead of hanging
3. `default:`

It's an error for a var to resolve from none of those. **By default, `output:` is only
ever created, never overwritten** — once the file exists, `devx init` (even with `--fix`)
reports that entry as `skipped` and leaves it untouched, so a developer's already-filled-in
local secrets/config are never clobbered. Add `output:`'s path to `.gitignore` yourself;
devx does not do this automatically since the path is project-defined.

Set `mode: merge` on an entry to change that behavior: instead of skipping an existing
`output:` entirely, devx renders the template and appends only the `KEY=VALUE` lines whose
key isn't already present in the file — every existing key (including ones the developer
has since hand-edited) is left untouched. This is meant for files that legitimately grow
new keys over time (e.g. a shared `.env` that gains new required vars as the project
evolves), reported as `merged` rather than `written`/`skipped`. `devx up` also runs
`envFiles:` as a best-effort, non-interactive step before starting the graph, so a `mode:
merge` entry's new keys show up automatically on the next `devx up` without a separate
`devx init`/`devx setup` run; failures here are reported as warnings and never block `devx
up`.

`secret: true` is documentation only — it doesn't log the resolved value, but terminal
input typed at the `prompt:` is not hidden. For real secrets, prefer setting `envVar:`
from your existing secrets manager (1Password CLI, Vault, etc.) rather than prompting.

---

## CLI commands

Flags for `devx up`/`devx run`/`devx logs` can appear anywhere in the argument list —
before, after, or interspersed with task names (e.g. both `devx up --dry-run api` and
`devx up api --dry-run` work identically).

| Command | Description |
|---|---|
| `devx up` | Run every task, respecting `deps:` — parallel within each dependency level. If the manifest sets `default:`, a bare `devx up` (no task names, no `--group`) runs only those targets instead. |
| `devx up --group <name>` | Only start tasks with `group: <name>` (and their deps). |
| `devx up --dry-run` | Print the planned execution order without running anything. |
| `devx run <name>` | Run one task and its transitive `deps:` only. |
| `devx run <name> --watch` | Re-run the task whenever its `sources:` globs change (polling, not inotify, in this release). |
| `devx <name>` | Shorthand for `devx run <name>`. |
| `devx down` | Stop detached tasks, in reverse dependency order. |
| `devx status` | Show which detached tasks are currently running/ready. |
| `devx logs <name>` / `devx logs` | Tail one task's logs, or all detached tasks'. |
| `devx exec <name> -- <cmd>` | Run `<cmd>` inside a running container task. |
| `devx tasks` / `devx tasks --json` | List every task with its description and inferred mode. |
| `devx validate` | Check `devx.yaml` for structural errors and common mistakes (see below). |
| `devx doctor` / `devx doctor --fix` | Check (and optionally install) `tools:`, and flag port collisions / missing `requires:` binaries. |
| `devx init` | Interactive scaffold for a new project; also runs `setup:` steps (installs tools, runs host-side setup, scaffolds `envFiles:`). |
| `devx new <name>` | Append a new task skeleton to `devx.yaml`. |

`devx validate` additionally prints (but does not fail on) warnings for:
- A task name colliding with a built-in command name (`devx <name>` would always run the
  built-in, never the task — use `devx run <name>` to run it explicitly).
- A detached task with no inferrable `ready:`/`port:`.
- A **detached** container task (`run:` contains `docker run`/`docker compose`/`podman
  run` *and* looks long-running, e.g. `-d`/`--detach`) whose container name couldn't be
  parsed and no `containerName:` override was set — `devx down`/`exec`/`logs`/`status`
  would have nothing to act on. A one-shot container (no `-d`/`--detach`) never triggers
  this, since nothing persists afterward anyway.
- Two or more tasks inferring (or declaring) the same port, unless every pairwise
  collision among them is covered by `conflicts:` declarations (see the task field
  reference above).

These are warnings, not errors — `devx validate` only exits non-zero on structural
problems (missing `run:`, an unknown task referenced by `deps:`/`default:`/`conflicts:`,
etc.), reported separately from the warnings list (as a `warnings` field alongside
`issues` in `--json` mode).

---

## Full example

See [`examples/basic/devx.yaml`](../examples/basic/devx.yaml) for a complete project with a
database, cache, a one-shot migration, a built-from-source API with an HTTP readiness
check, and a background worker.
