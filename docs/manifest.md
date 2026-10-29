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
| `includes` | map | Merge another `devx.yaml`'s tasks into this graph (see [Includes](#includes)). |
| `agents` | map | Named shell commands for the planned `devx workspace` feature (parallel per-branch workspaces with an AI coding agent auto-started on creation). Reserved and parsed today; not yet wired to a CLI command — see `docs/design/task-executor-refactor.md`. |
| `ai` | object | Optional LLM provider config used by `devx ai`. See `docs/ai.md`-equivalent comments in `schemas/devx.schema.json`. Credentials always come from environment variables, never from `devx.yaml`. |
| `tools` | list | Required CLI tools/SDKs, checked by `devx doctor`, installed by `devx setup`. |
| `setup` | list | Ordered host-side commands run by `devx setup`. |
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
| `desc` | string | One-line description shown in `devx tasks`. |
| `deps` | list | Task names that must complete (one-shot) or become ready (detached) before this task starts. `devx up`/`devx run <name>` compute the transitive closure and run independent tasks in parallel, level by level. |
| `dir` | string | Working directory for `run:`, relative to the `devx.yaml` that defines the task. Defaults to that file's own directory. |
| `env` | map | Environment variables merged into the task's process environment (after `${VAR}` substitution). |
| `detached` | bool | `true` starts the task in the background and moves on once it's ready, instead of waiting for it to exit. Inferred from `run:` (e.g. `docker run -d`, a trailing `&`, or a long-running dev-server pattern) when omitted — set explicitly to override a wrong guess. |
| `interactive` | bool | `true` attaches the task directly to the terminal's stdin/stdout/stderr (e.g. a shell, a REPL) — only valid when running that one task alone (`devx run <name>`), not as part of a larger `devx up`. Cannot be combined with `detached: true`. |
| `ready` | object | How devx decides a detached task has become ready — see [Readiness](#readiness). |
| `port` | int | Port this task listens on, used for `devx validate`'s port-collision check. Inferred from `run:` (`-p`, `--port`, `--publish`) when omitted. |
| `containerName` | string | Overrides the container name devx infers from `run:`, used by `devx down`/`logs`/`exec`/`status` to address the right container. Set explicitly if the name can't be parsed automatically. |
| `requires` | list | Binaries that must be on `PATH` for this task to run — checked by `devx doctor`. |
| `sources` / `generates` | list of globs | Skip re-running a one-shot task when every `generates:` file is newer than every `sources:` file (like a Makefile rule). `generates:` is invalid on a detached task. |
| `os` | list | Restrict this task to `linux`/`darwin`/`windows`; skipped (not an error) on any other OS. |
| `timeout` | duration string | Kill the task and treat it as failed if it runs longer than this (e.g. `"30s"`, `"2m"`). |
| `stopTimeout` | duration string | How long `devx down` waits after `SIGTERM` before `SIGKILL`. Defaults to `10s`. |
| `restart` | `never` \| `on-failure` \| `always` | Only meaningful on a detached task. Defaults to `never`. **Reserved for a later phase**: devx validates and persists this field today, but does not yet watch a detached task's process/container for exit and restart it — a crashed task simply stays dead regardless of `restart:`. |
| `shell` | string | Override the shell used to execute `run:`. |
| `group` | string | Logical group name, used by `devx up --group <name>` to start a subset of the graph. |
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
`devx setup` (or `devx doctor --fix`) runs each tool's `install:` command for the current
OS, then runs `setup:` steps in order. A `runOnce: true` step is skipped on a later
`devx setup` if its `run:` command's content hasn't changed since the last successful run.

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

## CLI commands


| Command | Description |
|---|---|
| `devx up` | Run every task, respecting `deps:` — parallel within each dependency level. |
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
| `devx setup` | Run `setup:` steps. |
| `devx new <name>` | Append a new task skeleton to `devx.yaml`. |
| `devx init` | Scaffold a starter `devx.yaml`. |

`devx validate` additionally warns (and exits non-zero) on:
- A task name colliding with a built-in command name (`devx <name>` would always run the
  built-in, never the task).
- A detached task with no inferrable `ready:`/`port:`.
- A container task (`run:` contains `docker run`/`docker compose`/`podman run`) whose
  container name couldn't be parsed and no `containerName:` override was set — `devx
  down`/`exec`/`logs`/`status` would have nothing to act on.
- Two tasks inferring (or declaring) the same port.

---

## Full example

See [`examples/basic/devx.yaml`](../examples/basic/devx.yaml) for a complete project with a
database, cache, a one-shot migration, a built-from-source API with an HTTP readiness
check, and a background worker.
