# Design: a lean, unified task model for devx.yaml

Status: proposal — not yet implemented. Written to align on direction
before any code changes.

## Problem

devx orchestrates three independent models today, each with its own
schema and its own partial feature set:

| | Config | Ordering | Readiness | Visible to `status`/`logs`/`down` |
|---|---|---|---|---|
| Container services/deps | `Service`/`Dep` under `profiles:` | delegated to `docker compose`'s native `depends_on` | `Health`/`Ready` fields | yes |
| Process services | `Service{Kind: "process"}` | `internal/graph.TopoSort`, started **sequentially** | `Ready` fields | yes |
| Kubernetes | separate renderer (`internal/k8s/render.go`) | none | none | no |
| Top-level `scripts:` | `Script` (flat list) | **none** — no `dependsOn` field exists | none | no, invisible to `up` |

On top of that, "where/how does this run" is encoded three incompatible
ways: `Service.Kind` ("container"/"process"), `Hook.Exec`/`Run` (with its
own mutual-exclusion validation in `internal/config/validate.go:90-114`),
and `Script.Container` ("auto"/"true"/"false", resolved via `docker ps`
label-matching plus shelling to the external `devcontainer` CLI in
`cmd/devx/cmd_run.go:189-217`). Every new technology or new "where does it
run" case has touched a different one of these, and `scripts:` — the
closest thing devx has to a Make-like escape hatch today — has no
dependency graph or readiness concept at all, so it can't express "start
the db, then run this script, then start the app."

Provider plugins (`internal/providers`) compound this: `ComposeFragment`'s
`Healthcheck` field is literally `compose.Healthcheck` under another name
— "provider data" is actually compose-shaped, not neutral — and
`internal/runtime/docker/docker.go`/`internal/k8s/kubectl.go` both shell
out to and parse the output of the `docker compose`/`kubectl` CLIs
directly, not an SDK.

## Goals

1. **One schema for "a named thing with dependencies that runs a
   command"**, replacing `Service`/`Dep`/`Script`/`Hook`.
2. **Adding a technology should touch one file**, not every exhaustive
   switch over a closed union (`Kind`, `ExecSpec`, `ready.Probe`).
3. **Don't abstract the technology away.** `docker run ...`/
   `kubectl apply ...` are already trivial to write (more so with AI
   assistance) and should stay plain shell in the manifest — devx's job is
   the handful of things a plain recipe can't express for a *dev
   environment* specifically: waiting for actual readiness (not just
   exit-0), running something detached/supervised, and unified
   `status`/`logs`/`down` across whatever mix of tech a repo uses.
4. **Minimize manifest surface.** Most tasks should need zero config
   beyond "what command to run" — anything else is inferred by convention
   and only written to override it.

## Non-goals

- Replacing `docker`/`compose`/`kubectl` with an SDK (a real, separate
  concern — see "Version/surface coupling" below — but independent of the
  schema redesign here).
- Devx parsing or executing real GNU Make syntax. Make's *philosophy*
  (flat namespace, deps + a recipe, infer rather than require config)
  informed this design, but `devx.yaml` stays devx's own plain YAML file,
  not a `.mk` parser. (This was prototyped and explicitly dropped — see
  "Alternatives considered".)

## Manifest design

A task is either a bare string (just the command — the common case, no
config needed) or an object, needed **only** when a task has `deps:`, an
explicit `ready:`/`detached:` override, or a `desc:`. Task names are
**top-level keys
directly** — no `tasks:`/`services:`/`profiles:` wrapper. `vars:`,
`includes:`, and `agents:` are the only other reserved top-level keys.

```yaml
vars:
  PORT: 5432
  IMAGE: postgres:16

lint: golangci-lint run
test: go test ./...
db: docker run --rm -d --name db -p ${PORT}:5432 ${IMAGE}
web: docker run --rm -d -p 8080:8080 myapp

api:
  run: ./bin/api --port 9000
  ready: { http: localhost:9000/status }   # override: non-standard path

migrate:
  deps: [db]
  run: ./scripts/migrate.sh up
```

`db`/`web` stay bare strings — nothing to configure: readiness and
detached are both inferred purely from the command (see below), with
zero fields written. `api` needs the object form only because it
overrides the inferred readiness path; `migrate` needs it only because it
declares `deps:`. The object form is never required just to attach a
command — if a task would have no fields besides `run:`, it isn't object
shaped at all.

Full task shape: `deps: [name, ...]`, `run: <shell command>`, `ready:
{http|tcp|cmd}` (override only), `detached: true|false` (override only),
`desc: <string>` (optional, drives `devx --help`), `group: <string>`
(optional, drives section headers in `devx --help` and `--group`
selection — see below), `env: {KEY: value}` (optional, raw pass-through to
the subprocess — distinct from `vars:`, see below), `os: [linux, darwin,
windows]` (optional, skip this task entirely on other platforms),
`interactive: true|false` (optional, attach stdin/stdout/stderr directly
instead of capturing for `logs`), `sources: [glob, ...]` (optional,
watch list for `--watch`, valid on one-shot and `detached: true` tasks
alike — see "Reliability" below) / `generates: [glob, ...]` (optional,
one-shot only — paired with `sources:` to skip re-running if newer than
every matched `sources:` file — see the `sources:`/`generates:` field
description below), `port:
<int>` (optional, override when `run:`'s port can't be parsed — see
"Inference" below), `timeout: <duration>` (optional, hard ceiling on a
one-shot recipe or a readiness wait — see "Reliability" below), `restart:
never|on-failure|always` (optional, only for `detached: true` tasks —
see "Reliability" below), `stopTimeout: <duration>` (optional,
detached-only, grace period before escalating a stop — see "Reliability"
below), `shell: bash|sh|cmd|powershell|pwsh|<path>` (optional, opt out of
the embedded shell interpreter for this task — see "`run:` execution"
below), `dir: <path>` (optional, working directory for `run:` and the
base for relative `sources:`/`generates:` globs — see "Working directory"
below), `requires: [tool, ...]` (optional, external binaries this task's
`run:` needs on `PATH` — see "Reliability" below), `containerName:
<string>` (optional, override when a container's name can't be parsed
from `run:` for `docker stop` — see "Reliability" below), `scope: shared
| workspace` (optional, default `workspace` — see "Parallel-issue
workflows" above; `shared` tasks are deduplicated across concurrently
running `devx up --workspace <id>` invocations instead of started once
per workspace). No `driver:`/`with:` field — `run:` is plain shell, technology-neutral by
construction. No before/after hooks —
`deps:` is the only ordering primitive; there is no separate "runs after,
without being a declared prerequisite" concept (see "Alternatives
considered").

### Inference, not required config

- **Readiness**: if `run:` exposes a port (parsed from `-p`/`--port`, or
  an explicit `port:` field for launchers the parser can't read — a
  wrapper script, a non-Docker command), default to an HTTP GET on
  `http://localhost:<port>/healthz` with short retry/timeout; if that
  exchange fails for *any* reason other than a clean 2xx/3xx/4xx HTTP
  response — connection refused, reset, timeout, or a non-HTTP peer
  closing/garbling the connection (e.g. Postgres on its native port) —
  fall back to a plain TCP connect on the same port; no known port →
  ready immediately. This matters: a port being open doesn't imply the
  thing listening on it speaks HTTP, so "falls back on refusal" alone
  would never trigger for most non-HTTP services. No Docker-specific
  healthcheck coupling — identical whether `run:` launches a container or
  a bare process. `ready:` only needs to be written to override this
  (non-standard path, or a `cmd:` check — exit code `0` means ready, any
  other exit code or a timeout means not-yet-ready and the check retries).
  Since port parsing is heuristic and will miss custom launchers, `devx
  validate` warns ("`web` looks long-running but no port/readiness could
  be inferred — add `ready:` or `port:` explicitly") rather than silently
  treating an unreadable launcher as "ready immediately," which is the
  worst failure mode here (devx believing something is ready when it
  isn't).
- **Detached vs one-shot**: a task with a port (parsed or `port:`) *or* a
  `ready:` (inferred or explicit) is almost certainly long-running, so
  default `detached: true`; otherwise default `detached: false` (block
  until exit code). Writing `ready:` already implies `detached: true` —
  there's never a need to write both just because `run:` itself has no
  literally-parseable port. Named `detached` (the term
  the ecosystem already uses for this — Docker's own `-d`/`--detach`
  flag) rather than `background`/`service`, so it needs no definition.
  `detached:` is only written to override the inference.
- **Variables**: `${VAR}` (braced) substitution resolved by devx itself
  (not shell expansion, since it applies to non-shell fields like
  `ready.http` too), applied to exactly these string fields: `run`,
  `ready.http`/`ready.tcp`/each element of `ready.cmd`, `env` values,
  `dir`, `sources`/`generates` glob patterns, `containerName`, and each
  `agents:` entry's command — **not** `desc`,
  `group`, or a task's own name, which are purely cosmetic/structural and
  substituting into them would just be confusing (and risks leaking a
  value into `--help` output outside the redaction path). A literal `$`
  that isn't meant as a devx variable is escaped Make-style: `$$` is
  rendered as a literal single `$`, so writing `$${VAR}` produces the
  literal text `${VAR}` instead of triggering substitution — reusing
  Make's own escape convention rather than inventing a new one.
  Precedence mirrors Make, extended with
  `.env` for local-dev convenience (same convention as Compose/foreman/
  direnv): CLI override (`devx up db PORT=5555`) > inherited process
  environment > `.env`/`.env.local` in the project root (loaded once at
  startup, never committed — `.env.local` wins over `.env` if both
  exist) > `vars:` defaults. `.env`/`.env.local` discovery is root-only
  — the root manifest's own directory, not re-checked per `includes:`
  file — matching Compose's own single-`.env`-file behavior; an
  included file's tasks still see it, since it's loaded once into the
  whole process's environment before anything runs. A `.env` file is
  optional; nothing changes for a repo that doesn't have one. Unbraced
  `$VAR` is **not** touched by devx — it's
  left as literal text for the embedded shell interpreter (or `shell:`
  override) to resolve at actual exec time from the real subprocess
  environment, which includes whatever `env:` set. This distinction
  matters for secrets: a value that only ever needs to exist inside the
  launched process (never in devx's own command-text buffer) should be
  referenced as `$VAR`, not `${VAR}` — see `env:` below and "Secrets
  never echo back" under "Reliability". **Caveat:** `$VAR` is POSIX
  shell syntax, understood by the embedded interpreter and by `shell:
  bash|sh`. A task that also sets `shell: cmd` or `shell: powershell`/
  `pwsh` won't resolve `$VAR` at all (`cmd` needs `%VAR%`, PowerShell
  needs `$env:VAR`) — the literal text would reach the external command
  unexpanded instead of silently leaking, but it does mean the
  unbraced-`$VAR` convention is tied to the default/POSIX `shell:`
  values, not universal across every `shell:` option.
- **Composition**: `includes: {name: path}` merges another `devx.yaml`'s
  tasks into **one** graph with namespaced task names (`infra:db`), not a
  recursive sub-process per file — see "Alternatives considered" for why
  recursive sub-invocation is the wrong model to copy from Make. Task
  names are merged into one flat namespace, but `vars:` are not: an
  included file's own `vars:` are scoped to that file's own tasks only
  (mirrors "Working directory" below — each included file owns its
  directory and its vars), not visible to the root manifest or to other
  included files. The root manifest's `vars:` remain globally visible to
  every task, including included ones, and act as the fallback/default
  layer beneath an included file's own `vars:` of the same name — same
  precedence order as CLI/env override root `vars:` (see "Variables"
  above), just one level deeper per included file.
- **Working directory**: `run:` (and relative `sources:`/`generates:`
  globs) execute with the defining `devx.yaml` file's own directory as
  the working directory by default — for the root manifest that's the
  repo root; for an `includes:`-merged file (e.g. `web/devx.yaml`) it's
  that file's directory, not the root's. This removes the `cd web && ...`
  boilerplate an included file's tasks would otherwise need in every
  `run:` line, and resolves what would otherwise be an unstated ambiguity
  the moment a manifest uses `includes:` across directories. `dir:
  <path>` overrides this per task (relative to the defining file's
  directory, or absolute) for the rare case a task needs a directory
  other than its own file's.

### Reliability (matters as much to an AI agent driving devx as to a human)

A CLI tool an agent shells out to has no way to distinguish "still
working" from "stuck forever" beyond what the tool itself guarantees, and
no way to know a stack is already running unless re-running the start
command is safe. These are existing gaps this design should close, not
new abstractions:

- **`timeout: <duration>`** — a hard ceiling on a one-shot task's recipe,
  or on a `ready:` wait, after which devx fails fast with a clear timeout
  error instead of hanging indefinitely. Optional, with a conservative
  built-in default (so a forgotten `timeout:` still bounds things) rather
  than requiring every task to set one.
- **`devx up` is idempotent**: running it again while the stack is
  already up reconciles rather than erroring or double-starting — tasks
  already running/ready are reported as such and skipped, exactly like
  `docker compose up` run twice. This matters more for devx than for a
  human-only tool, since an agent retrying a command after an ambiguous
  result is a normal failure-recovery pattern, not a mistake.
- **`restart: never|on-failure|always`** (only meaningful on `detached:
  true` tasks; default `never`) — Docker's own naming for this, reused
  rather than inventing new vocabulary. Retry behavior is a fixed
  exponential backoff capped at a conservative ceiling (1s, 2s, 4s, 8s,
  16s, 30s, 30s, ...), reporting `crashed` instead of continuing to
  `restart` after 5 consecutive failures — mirroring Kubernetes's own
  `CrashLoopBackOff`, not a new scheme, and not configurable per task
  initially (fixed built-in constants; revisit if real usage shows the
  cap/count wrong for some workload). `devx status` distinguishes
  `crashed`/`restarting` from `running`/`ready` so both a human and an
  agent parsing `--json` output can tell "it's fine" from "it died and
  came back" from "it died and stayed dead."
- **Secrets never echo back**: `env:` values resolved from
  `${HOST_ENV_VAR}` (braced, devx-substituted once at startup) must
  never appear in `--dry-run` output, `--json` output, or logs —
  substituted commands shown by `--dry-run`/`devx tasks` render `env:`
  entries as `KEY=<redacted>`, never the resolved value. The companion
  rule in "Variables" above is what actually makes this hold end to end:
  a secret referenced inside `run:` (or any other string field) must use
  unbraced `$VAR`, never braced `${VAR}` — braced references are
  substituted by devx into its own command text (and so would appear in
  `--dry-run`/`--json` regardless of where the value came from), while
  unbraced references are left untouched for the subprocess's real
  environment to supply at exec time. This matters more than it would
  for a human-only tool: both a human pasting terminal output into
  chat/an issue and an agent echoing tool output verbatim (a documented
  failure mode for AI coding tools generally) are realistic leak paths,
  not hypothetical ones.
- **Graceful stop, driver-aware**: `Handle.Stop()`'s actual mechanism
  differs by what `run:` launched, and the design didn't previously say
  so. For a plain detached process (`processDriver`), `Stop()` sends
  SIGTERM to the process group, waits up to `stopTimeout:` (optional,
  conservative default — e.g. 10s, mirroring Docker's own default grace
  period), then SIGKILL. For a container (`run:` parsed as `docker run`/
  `docker compose`, the same `--name`/service-name parsing already used
  for port inference, with an optional `containerName:` override —
  identical override shape to `port:` — for the rare `run:` shape that
  parsing can't read a name out of), `Stop()` calls `docker stop <name>`
  instead — the launched `docker run -d` process has already exited by
  the time `Stop()` runs (that's what `-d` means), so there's no live
  process on devx's side to signal directly; `docker stop` itself already
  implements the SIGTERM-then-grace-then-SIGKILL contract, so devx
  doesn't reimplement it, just invokes it (with `stopTimeout:` passed
  through as `docker stop -t <seconds>` when set). `devx validate` warns
  when a container-shaped `run:` has no parseable/overridden name, since
  `Stop()` would otherwise have nothing to act on and `devx down` would
  leave the container orphaned and running.
- **Port-collision detection**: since ports are inferred silently from
  `run:` text (or `port:`), `devx validate` flags two tasks that resolve
  to the same port as a warning — otherwise the only symptom is one task
  failing to bind at `up` time with a generic "address already in use"
  error that doesn't point at the actual conflicting task.
- **`requires: [tool, ...]`** — the external binaries a task's `run:`
  needs on `PATH` (`docker`, `kubectl`, `brew`, ...), checked by `devx
  doctor` and by `devx validate`/`devx up`/`devx run` before dispatching
  the recipe, surfaced as "`web` needs `docker`, which isn't installed —
  see `devx doctor`" instead of an opaque shell "command not found."
  Optional — `run:` stays plain shell either way; `requires:` only adds a
  better error message ahead of a failure devx can't otherwise predict,
  since it never parses `run:` beyond port/`--name` sniffing.
- **Inferred `dir:` is always shown, never silently implicit**: `devx
  tasks --json`/`devx run <name> --dry-run` always include the resolved
  `dir:` (the defining file's own directory, or the override), even when
  it was never written in the manifest — so moving a task between files
  (e.g. out of an `includes:`-merged file into the root manifest) is
  visible the moment its effective working directory changes, instead of
  surfacing only as a confusing runtime path error.

### Additional optional fields (from Make / modern task runners)

Reviewed against Make and modern runners (Taskfile, Just) for what else
genuinely earns its keep in a dev-environment context — see "Alternatives
considered (and rejected)" below for what was reviewed and left out:

- **`env: {KEY: value}`** — raw environment variables passed to the
  task's subprocess only. Deliberately separate from `vars:`: `vars:` are
  devx's own named values (CLI override > inherited env > `vars:`
  defaults) substituted into manifest string fields like `run:`/
  `ready.http`; `env:` keys go straight into the subprocess's OS
  environment instead of a manifest field, which is where host-provided
  secrets belong (consistent with the repo's existing convention that
  secrets are never written into `devx.yaml`). Braced `${VAR}`
  substitution still applies to `env:` *values* the same as any other
  string field — `env: { DB_PASSWORD: ${HOST_DB_PASSWORD} }` resolves
  `${HOST_DB_PASSWORD}` from the inherited shell environment (never a
  literal secret in `devx.yaml`) before setting `DB_PASSWORD` in the
  subprocess's environment. The distinction from `vars:` is the
  *destination* (subprocess env vs. a manifest field), not whether
  substitution runs on the right-hand side. But the resulting
  `DB_PASSWORD` must only ever be *referenced* elsewhere (e.g. inside
  `run:`) as unbraced `$DB_PASSWORD` — never `${DB_PASSWORD}` — or devx's
  own substitution step would re-resolve it from the `vars:`-precedence
  chain and bake the secret into the literal command text devx builds,
  which is exactly what `--dry-run`/`--json` redaction can't see through
  (see "Secrets never echo back" under "Reliability").
- **`os: [linux, darwin, windows]`** — skip this task entirely (not an
  error) when the host OS isn't in the list. Relevant because devx already
  does cross-platform tool detection/`doctor`; some recipes (Homebrew
  installs, WSL-specific steps) are legitimately OS-specific. A skipped
  task is treated as satisfied for anything that depends on it (same
  outcome as a `sources:`/`generates:` "up to date" skip below) — the
  dependent proceeds rather than failing or being skipped itself.
- **`interactive: true`** — attach the task's stdin/stdout/stderr directly
  to the terminal instead of capturing output for `devx logs`, for tasks
  that need a live TTY (a `psql` shell, an interactive debugger). Mirrors
  the gap already implied by today's `Service.Debug` field. Mutually
  exclusive with `detached: true` (validation error): an interactive
  task needs a terminal attached to one foreground invocation, which
  doesn't make sense for a supervised background process whose output
  also needs to be captured for `devx logs`.
- **`sources: [glob, ...]` / `generates: [glob, ...]`** — two related but
  distinct behaviors, split by task shape:
  - **One-shot tasks** (the "Make's actual core feature" case — skip a
    recipe if the target is already up to date versus its prerequisites,
    applied to files instead of files-as-targets generically): if every
    `generates:` glob match has a newer mtime than every `sources:` glob
    match, `devx run`/`up` skips the recipe (reports "up to date", same
    spirit as Make's own skip message) and anything depending on it
    treats the skip as satisfied, exactly like an `os:` skip above.
    `generates:` only makes sense paired with `sources:` on a one-shot
    task — a supervised long-running process has no single "run to
    completion" point to compare timestamps against, so `generates:`
    together with `detached: true` is a validation error.
  - **Detached tasks**: `sources:` alone (no `generates:`) is valid, and
    means something different — purely the watch list for `devx up
    --watch`/`devx run <name> --watch` (see "CLI surface" below) to
    restart the task on change. There's no "up to date" skip concept
    here (nothing to compare timestamps against), just "these files,
    when changed, mean this service needs a restart." This is why the
    worked example's `api` task (inferred `detached: true` via its
    `ready:`) can carry `sources:` for `--watch` without contradicting
    the one-shot-only skip rule above.
  - Opt-in either way — a task with neither field always runs (one-shot)
    or is never auto-restarted (detached). High value for expensive
    one-shot steps (codegen, compiling a tool, migrations keyed off a
    migrations directory) that are wasteful to always re-run, and for
    detached services whose own tooling doesn't already self-watch.
- **`shell: bash|sh|cmd|powershell|pwsh|<path>`** — opt out of the
  embedded cross-platform shell interpreter (see "Internal architecture")
  for a specific task, exec'ing a real installed shell binary instead.
  Needed only for recipes relying on shell features the embedded
  interpreter doesn't cover (advanced Bash extensions, a pre-existing
  `.sh`/`.ps1` script that assumes a real shell). Unset (the default)
  means every task gets the embedded interpreter.

```yaml
# --- env: vs vars: ---
db:
  # $DB_PASSWORD is unbraced on purpose: it's resolved by the shell from
  # the subprocess's real environment at exec time (set by env: below),
  # never by devx's own ${VAR} substitution — so it never appears in
  # devx's own command text, and `--dry-run`/`--json` can't leak it.
  run: docker run --rm -d --name db -p ${PORT}:5432 -e POSTGRES_PASSWORD=$DB_PASSWORD ${IMAGE}
  env:
    DB_PASSWORD: ${HOST_DB_PASSWORD}   # pulled from the inherited shell env, never written literally

# --- os: ---
install-tools:
  desc: Install macOS-only dev dependencies via Homebrew
  os: [darwin]
  run: brew bundle --file=./Brewfile

# --- interactive: ---
psql:
  desc: Open a psql shell against the dev db
  deps: [db]
  interactive: true
  run: docker exec -it db psql -U postgres

# --- sources:/generates: ---
build:
  desc: Compile the CLI binary
  sources: ["cmd/**/*.go", "internal/**/*.go", "go.sum"]
  generates: ["./bin/devx"]
  run: go build -o ./bin/devx ./cmd/devx

# --- shell: (opt out of the embedded interpreter) ---
legacy-migrate:
  desc: Pre-existing bash script using array tricks the embedded shell can't parse
  shell: bash
  run: ./scripts/legacy-migrate.sh

# --- timeout: (bound a recipe/readiness wait) ---
smoke:
  desc: One-shot smoke test — never let a hung request block `devx up` forever
  timeout: 30s
  run: ./scripts/smoke-test.sh

# --- restart: / stopTimeout: (only for detached: true) ---
db:
  desc: Postgres for local dev, restarted if the container dies
  run: docker run --rm -d --name db -p ${PORT}:5432 ${IMAGE}
  restart: on-failure
  stopTimeout: 15s      # give Postgres longer than the 10s default to
                        # shut down cleanly before `docker stop` escalates

# --- dir: (working directory override) ---
build-web:
  desc: Build the frontend bundle
  dir: ./web            # relative to this file's own directory
  run: npm run build    # equivalent to `cd web && npm run build`, but
                        # composable with includes: across directories

# --- requires: (external tools this recipe needs) ---
deploy-local:
  desc: Apply manifests to the local k3d cluster
  requires: [kubectl]   # `devx doctor`/`validate` report this clearly
  run: kubectl apply -f ./k8s/local

# --- containerName: (override when --name can't be parsed from run:) ---
cache:
  desc: Redis via a wrapper script that hides the real `docker run` invocation
  run: ./scripts/start-cache.sh
  containerName: devx-cache   # `devx down`/`devx exec` need this to find
                              # the container; validate would otherwise
                              # warn that Stop() has nothing to act on
```


### `devx --help` follows standard CLI conventions, not a custom format

devx's CLI today (`cmd/devx/main.go`) is a hand-rolled `switch` over
`os.Args` with `fmt.Println`-based usage text — no framework, no
generated flag parsing, no standard help shape. The original version of
this proposal generated task help as its own bespoke "key: value" dump,
which repeats that mistake in a new place: anyone fluent in `git`/
`docker`/`kubectl`/`gh` already has a shape burned into muscle memory
(`Usage:` / `Flags:` / "Use `<cmd> --help` for more info"), and a
devx-specific layout fights that instead of using it. The fix is to
converge on that shape, generating the task-listing *portion* from the
manifest but keeping the surrounding structure conventional:

```
$ devx --help
devx — lean task runner + dev environment orchestrator

Usage:
  devx <command> [flags]
  devx <task> [flags]              (shorthand for `devx run <task>`)

Commands:
  up          Start detached tasks (+ dependencies) from devx.yaml
  down        Stop running detached tasks
  run         Run one or more tasks to completion
  status      Show status of detached and one-shot tasks
  logs        Show captured output for a task
  tasks       List all tasks defined in devx.yaml
  validate    Validate devx.yaml
  doctor      Check required tools are installed
  ...

Tasks (from devx.yaml — see `devx tasks` for the full list):
  lint        golangci-lint run
  test        go test ./...
  db          Postgres for local dev
  api         Backend API                           ready: http localhost:9000/status
  migrate     Apply DB migrations                    deps: db

Flags:
  -h, --help      help for devx
  -v, --version   print devx version

Use "devx <command> --help" for more information about a command.
Use "devx <task> --help" for more information about a task from devx.yaml.

$ devx run migrate --help
Usage:
  devx run migrate [flags]

Apply DB migrations

Dependencies:
  db

Recipe:
  ./scripts/migrate.sh up

Flags:
      --dry-run   print the resolved command, run nothing
  -h, --help      help for run
```

- **Commands vs. Tasks are two clearly labeled sections**, not
  interleaved — a CLI-literate user scans `Commands:` for devx's own
  verbs exactly like any other tool, and `Tasks:` for what *this repo*
  defines, without needing to learn that devx blends the two.
- `desc:` is shown when present; otherwise the (possibly truncated) `run:`
  command itself is shown, so every task is listed even with zero extra
  config — nobody has to write `desc:` just to appear in `--help`.
- Listing is alphabetical by default; an optional `group: <string>` field
  (unset by default) lets a manifest opt into Make's `##@`-style section
  headers within the `Tasks:` listing (both `--help`'s capped view and
  `devx tasks`'s full view), without being required for either to work.
- `devx --help`'s task list is capped (e.g. 10-15 entries) with a pointer
  to `devx tasks`/`devx tasks --group <name>` for the full list — large
  manifests (many `includes:`) shouldn't make top-level `--help` unusably
  long. `devx tasks` itself has no cap.
- Per-task help (`devx run <task> --help`) mirrors a real subcommand's
  help page: `Usage:` line, free-text `desc:` (or nothing, if unset) up
  top, labeled sections for whatever's non-default (`Dependencies:`,
  `Recipe:`, `Readiness:`, `Environment:`, ...), and a `Flags:` section —
  not a flat, unlabeled key/value dump.
- **`devx <task>` is shorthand for `devx run <task>`** (mirrors `make
  <target>` directly, the convention this whole redesign is borrowing
  from). Built-in command names always win on collision — a task literally
  named `up` or `run` is still reachable via `devx run up`, but `devx up`
  always means the built-in; `devx validate` warns on such a collision
  since it's very likely a naming mistake, not intentional shadowing.
- `includes:`-merged tasks show under their namespaced name (`infra:db`),
  so `--help`/`devx tasks` reflect the real, fully-merged graph, not just
  the local file.
- Same data backs `devx --json` output per the repo's existing convention
  (`cmd_status.go`/`cmd_doctor.go` pattern) — `devx tasks --json` dumps the
  full structured list for tools/agents instead of scraping `--help` text.

Adopting a real CLI framework (**`spf13/cobra`**) to drive this is now a
settled decision, not an open question (see implementation plan step 5)
— made in this same rewrite rather than deferred, since devx's hand-
rolled `completion bash|zsh|fish` (`cmd_completion.go`) already
duplicates functionality Cobra provides for free, and every `cmd_*.go`
entrypoint is being touched regardless.

## CLI surface

Commands and flags this adds/changes, reviewed against Make's own CLI
(`make target1 target2`, `-n`, `-k`, `-j`) and the gaps that review
surfaced:

| Command | Behavior |
|---|---|
| `devx up` | Transitive closure of every `detached: true` task's `deps:` (see worked example below for the exact rule). Independent one-shot branches run in parallel, not strictly sequential — mirrors Make's `-j` by default, not an opt-in flag. **Idempotent**: tasks already running/ready are reported and skipped, not restarted — safe to re-run, including by an agent retrying after an ambiguous result. **Default attached**: streams every started task's logs interleaved, name-prefixed and colored, to the terminal (mirrors `docker compose up`'s own default); Ctrl-C stops the stack (equivalent to `devx down`) rather than detaching. |
| `devx up -d` / `--detach` | **New.** Start the stack and return immediately instead of attaching to logs — mirrors `docker compose up -d`, for scripts/agents that don't want a foregrounded terminal. |
| `devx up <name...>` | **New.** Partial up — only the named tasks + their transitive deps, not the whole closure. Mirrors `make a b c`. |
| `devx up --dry-run` | **New.** Print the resolved graph and every substituted `run:`/`ready:` command, execute nothing. Mirrors `make -n`. Resolved `env:` values are shown redacted (`KEY=<redacted>`), never the real secret. |
| `devx up --keep-going` | **New.** Default is fail-fast (stop everything on first error, closest to least-surprising for a dev-env bootstrap). `--keep-going` continues unrelated branches of the graph on failure — mirrors `make -k`. |
| `devx up --group <name>` | **New.** Only tasks whose `group:` matches (+ their deps) — reuses the `group:` field already defined for `--help`, instead of inventing a second selection mechanism (e.g. tags). |
| `devx up --workspace <id>` | **New.** See "Parallel-issue workflows" above. Namespaces container/network/volume names and non-pinned ports with `<id>`, so multiple `devx up` invocations against different `git worktree`s of the same manifest (e.g. one per issue, attached to the same shared devcontainer) can run concurrently without colliding. Defaults to a value derived from the current `git worktree` path when omitted; `scope: shared` tasks are deduplicated across workspaces rather than started once per workspace. |
| `devx workspace list` / `devx workspace rm <id>` | **New.** `list` shows every active workspace id and its tasks (grouped, mirrors `devx ps --all-workspaces` below). `rm <id>` tears down that workspace's containers/volumes and frees its ports; `scope: shared` tasks are only stopped if no other workspace still references them. |
| `devx ps --all-workspaces` | **New.** Same as `devx status`'s running-task view, but across every active workspace at once instead of just the current one — for checking what's running across issues without switching windows. |
| `devx workspace new <id> [--agent <name>]` | **New.** Create a `git worktree` for `<id>` (destination `../<repo-dir-name>-<id>`, new branch named `<id>` branched from the current `HEAD`; errors if the branch or worktree path already exists, suggesting `devx workspace agent <id>` instead of silently reusing/overwriting it), run `devx up --workspace <id>` in it, then attach interactively to the named `agents:` entry (or the sole one) — one command to start working on an issue. |
| `devx workspace` (bare) | **New.** Shorthand for `devx workspace new` that fills in gaps interactively: prompts for `<id>` (used as both workspace id and new branch name) if omitted, and prompts to pick an `agents:` entry if more than one is configured and `--agent` wasn't given. TTY-only — in a non-interactive context (script, or an agent driving devx itself) it errors asking for `devx workspace new <id> --agent <name>` explicitly instead of hanging on a prompt. With exactly one `agents:` entry, no prompt either way. |
| `devx workspace agent <id> [<name>]` | **New.** Attach interactively to an `agents:` entry, cwd'd into workspace `<id>`'s worktree with that workspace's env resolution — for re-attaching in a new terminal/window without recreating the stack. |
| `devx workspace clean` | **New.** Bulk-removes stale workspaces: worktree directory no longer exists (removed outside devx) → reclaims its orphaned containers/volumes/ports; worktree's branch merged/deleted upstream → candidate for removal. Skips (warns, doesn't delete) any workspace with uncommitted/unpushed changes unless `--force`. On-demand only — no background/automatic cleanup, consistent with devx having no daemon. |
| `devx workspace list --stale` | **New.** Dry-run preview of exactly what `devx workspace clean` would remove, without removing anything. |
| `devx up --watch` | **New.** After the stack is up, watches every task in the resolved closure that has `sources:` set and restarts it (stop, re-run recipe, wait for ready again for `detached:` tasks; just re-run for one-shot `deps:` of a detached task, e.g. `migrate`) on change — the whole-stack version of `devx run <name> --watch` below, applied automatically across the graph instead of needing one `--watch` per service in separate terminals. Mirrors `docker compose up --watch`/`docker compose watch`. A rebuild failure on one task is reported inline and leaves the rest of the stack running (per-task isolation) rather than stopping the whole `--watch` session. |
| `devx run <name...>` | Run one-shot task(s) to completion, in the order given, respecting `deps:`. Multi-target, like `devx up <name...>`. |
| `devx run <name> --dry-run` | Same dry-run semantics as `up`. |
| `devx run <name> --watch` | **New.** Re-run the task whenever its `sources:` globs change (requires `sources:` to be set — error otherwise, nothing to watch). Same spirit as Docker Compose Watch, scoped to one task instead of a whole stack. |
| `devx <name>` | **New.** Shorthand for `devx run <name>` (mirrors `make <target>`). Built-in command names always take precedence on collision — see "`devx --help`" below. |
| `devx status` | Unchanged in spirit — detached tasks: `running`/`ready`/`crashed`/`restarting` (per `restart:`); one-shot tasks with `sources:`/`generates:`: up to date / stale, last run time, exit code. |
| `devx logs` | **New.** With no task name: tails every currently-running detached task's output interleaved, name-prefixed and colored, in one stream — the same engine `devx up`'s default attached mode uses, available standalone for re-attaching to an already-`-d`-started stack. |
| `devx logs <name>` | Unchanged — tails captured output for one task. For `interactive: true` tasks, `logs` has nothing captured (documented, not an error) since output went straight to the terminal when it ran. |
| `devx exec <name> -- <cmd...>` | Unchanged from today — run `<cmd>` inside a running `detached: true` task's context (container/process), via `Handle.Exec`. Errors if the task isn't currently running. |
| `devx down` | Stops every running detached task belonging to the current workspace, in reverse dependency order. Graceful by default (see "Graceful stop, driver-aware" under "Reliability": SIGTERM/`docker stop` + grace period from `stopTimeout:`, then force); `devx down --force` skips straight to SIGKILL/`docker kill`. **`scope: shared` tasks are reference-counted, same as `devx workspace rm`** (see "Parallel-issue workflows"): stopped only once no other workspace still references them — `devx down` in one workspace never kills a shared dependency another workspace is actively using. |
| `devx tasks` / `devx tasks --json` | **New.** Dumps the full resolved task list (name, `desc`, `group`, `deps`, `ready`, `detached`, `env`, `os`, `interactive`, `sources`, `generates`, `shell`, `timeout`, `restart`, `stopTimeout`, `dir`, `requires`, `containerName`, `scope`) — the structured data backing `--help`, consumable by tools/agents per the repo's `--json` convention. `env:` values shown redacted, same as `--dry-run`. `dir:` is always included resolved, even when inferred (never silently implicit — see "Reliability"). |
| `devx --help` / `devx run <name> --help` / `devx <name> --help` | Generated from the manifest, Cobra-style — see "`devx --help` follows standard CLI conventions" above. |
| `devx validate` | Unchanged command, extended checks (all warnings, not errors): a task name colliding with a built-in command; `restart:` set on a non-`detached` task (meaningless — nothing to restart); two tasks resolving to the same port (see "Port-collision detection" under "Reliability"); a detached task with no port/readiness inferred and no `ready:`/`port:` written (see "Readiness" under "Inference"); a container-shaped `run:` with no parseable/overridden `containerName:` (see "Graceful stop, driver-aware" under "Reliability"). `timeout:` has no such check: it's valid on both one-shot recipes and detached tasks' `ready:` waits, so there's no task shape it's ever a mistake on. |
| `devx doctor` | Extended to also check every task's `requires:` binaries are on `PATH`, not just devx's own built-in tool checks — surfaces "`web` needs `docker`, which isn't installed" per task instead of only devx's own dependencies. |

`--watch` (either form) is **restart-only** — stop, re-run the recipe,
wait for ready again — not file-sync-into-a-running-container the way
Compose Watch's `sync` action is. A task whose own tooling already
watches and hot-reloads itself (`npm run dev`/Vite, most framework dev
servers) doesn't need `sources:`/`--watch` at all; it's for the cases
that don't self-watch (a Go binary needing `go build` + restart, a
wrapper script). Consistent with "don't abstract the technology away" —
devx doesn't try to replace a dev server's own hot-reload, only fills the
gap for things that don't have one.

## Example: a realistic multi-service project

A fuller project, showing every field and override in context — not just
the two-task teaching snippet above. One repo root, one included infra
module, one included web module.

`devx.yaml` (root):

```yaml
vars:
  PORT: 5432
  REDIS_PORT: 6379
  IMAGE: postgres:16
  ENV: dev

includes:
  infra: ./infra/devx.yaml   # namespaced: infra:minio, infra:create-bucket
  web: ./web/devx.yaml       # namespaced: web:app

# ---- ad hoc commands: plain bare strings, no overrides needed ----
# Not part of `devx up` — nothing detached depends on them, so they only
# run via `devx run <name>`.
lint: golangci-lint run
fmt: gofmt -l -w .
test: go test ./...

smoke:
  desc: One-shot smoke test against the running stack
  deps: [web:app]              # still ad hoc: nothing depends ON smoke
  timeout: 30s                 # never let a hung request block CI/an agent forever
  run: ./scripts/smoke-test.sh

build:
  desc: Compile the CLI binary
  sources: ["cmd/**/*.go", "internal/**/*.go", "go.sum"]
  generates: ["./bin/devx"]
  run: go build -o ./bin/devx ./cmd/devx

install-tools:
  desc: Install macOS-only dev dependencies via Homebrew
  os: [darwin]
  run: brew bundle --file=./Brewfile

psql:
  desc: Open a psql shell against the dev db
  deps: [db]
  interactive: true
  run: docker exec -it db psql -U postgres

# ---- services: part of `devx up`, fully inferred (bare string, zero config) ----
db:
  desc: Postgres for local dev
  # $DB_PASSWORD unbraced — resolved by the shell from the subprocess's
  # real env (set by env: below), never by devx's own ${VAR} substitution.
  run: docker run --rm -d --name db -p ${PORT}:5432 -e POSTGRES_PASSWORD=$DB_PASSWORD ${IMAGE}
  env:
    DB_PASSWORD: ${HOST_DB_PASSWORD}   # from the inherited shell env, never a literal in devx.yaml

# ---- service needing a readiness override: TCP-open isn't "ready" here ----
redis:
  group: Services
  desc: Redis cache
  run: docker run --rm -d --name redis -p ${REDIS_PORT}:6379 redis:7
  ready: { cmd: ["redis-cli", "-p", "${REDIS_PORT}", "ping"] }

# ---- service needing ONE override: `run:` has no port/flag for
#      inference to key off at all — writing `ready:` alone already
#      implies detached: true, no need to also write it ----
queue:
  group: Services
  desc: Message queue (wrapper script hides the real port)
  run: ./scripts/start-queue.sh
  ready: { tcp: localhost:5672 }

# ---- data bootstrap: one-shot, chained, pulled into `up` only because
#      `api` (a detached task) transitively depends on them ----
migrate:
  group: Data
  desc: Apply DB migrations
  deps: [db]
  timeout: 1m                  # migrations hanging against a half-started db
  run: ./scripts/migrate.sh up

seed:
  group: Data
  desc: Seed fixtures for ${ENV}
  deps: [migrate]
  run: ./scripts/seed.sh ${ENV}

# ---- service with an inferred port but a non-standard health path ----
api:
  group: Services
  desc: Backend API
  deps: [migrate, infra:minio]
  sources: ["api/**/*.go"]     # `devx up --watch` / `devx run api --watch`
                                # rebuilds + restarts api on a source change
  restart: on-failure          # crash-loop during a bad rebuild shouldn't kill `up`
  run: ./bin/api --port 9000
  ready: { http: localhost:9000/status }
```

`infra/devx.yaml` (included as `infra:`):

```yaml
vars:
  BUCKET: devbucket

minio: docker run --rm -d --name minio -p 9100:9000 -e MINIO_ROOT_USER=admin -e MINIO_ROOT_PASSWORD=admin123 minio/minio server /data

create-bucket:
  desc: Create the dev S3 bucket
  deps: [minio]
  run: ./scripts/create-bucket.sh ${BUCKET}
```

`web/devx.yaml` (included as `web:`):

```yaml
app:
  group: Web
  desc: Frontend dev server with hot reload
  deps: [seed, infra:minio]
  run: npm run dev
  ready: { http: localhost:5173 }   # override: `npm run dev` has no literal
                                     # port for inference; implies detached: true
```

**What `devx up` actually runs**, which this example makes concrete: the
transitive closure of every `detached: true` task's `deps:`. Walking it:
`api` and `web:app` are the two detached tasks with deps, so `up` resolves
their full dependency chains — `db`, `migrate` (dep of `api`), `seed` (dep
of `web:app`, which deps on `migrate`), and `infra:minio` (dep of both) —
plus `redis` and `queue`, detached but standalone. **Not** run by `up`:
`lint`, `fmt`, `test`, `smoke`, `build`, `install-tools`, `psql`, and
`infra:create-bucket` — one-shot tasks with no detached task depending on
them, invoked explicitly with `devx run <name>` instead. This is the one
rule governing `up`'s scope — no separate "include in up" flag needed.

A few CLI invocations against this same manifest:

```sh
devx up                        # everything above
devx up db redis               # just those two + nothing else (no deps)
devx up --group Services       # redis, queue (standalone) + api, migrate,
                                # db, infra:minio (api's transitive deps);
                                # NOT web:app — it's `group: Web`, and
                                # nothing Services-tagged depends on it
devx up --dry-run              # print the resolved graph + substituted
                                # commands, run nothing
devx up --keep-going           # if `redis` fails to start, `db`/`queue`/
                                # `api` still proceed; only redis's branch stops
devx run build                 # skip, printing "up to date", if ./bin/devx
                                # is newer than every cmd/**/*.go + go.sum
devx run api --watch           # rebuild+restart api whenever api/**/*.go changes
devx up --watch                # same, applied to every sources:-bearing task
                                # in the closure (here: just api)
devx run psql                  # attaches the terminal directly (interactive: true)
devx run install-tools         # no-op on Linux/CI (os: [darwin]), runs on macOS
devx tasks --json               # structured list for tools/agents
```

## Internal architecture

The manifest redesign is paired with an execution-model redesign so
"adding a technology" stays a one-file change on the Go side too:

```go
// Task is the resolved, post-inference shape consumed by the scheduler/
// Driver — e.g. Detached here is always a concrete true/false because
// inference has already run; the raw YAML-level type (populated by
// Task's custom UnmarshalYAML) separately tracks which fields were
// actually written in the manifest vs. defaulted/inferred, which is what
// feeds the "written vs. inferred" distinction used by `devx validate`,
// `--dry-run`, and `devx tasks --json`'s "dir: always shown resolved,
// even when inferred" behavior described above.
type Task struct {
    Name        string
    Deps        []string
    Run         string
    Ready       ReadyCheck
    Detached    bool
    Desc        string
    Group       string
    Env         map[string]string
    OS          []string // empty = all platforms
    Interactive bool
    Sources     []string // glob patterns
    Generates   []string // glob patterns
    Shell       string        // "" (default, embedded) | "bash" | "sh" | "cmd" | "powershell" | "pwsh" | a custom path
    Port        int           // 0 = infer by parsing Run; set only when parsing can't find it
    Timeout     time.Duration // 0 = built-in conservative default, not unbounded
    Restart     string        // "never" (default) | "on-failure" | "always"; detached-only
    StopTimeout time.Duration // 0 = built-in conservative default; detached-only
    Dir         string        // "" = defining file's own directory
    Requires    []string      // external binaries on PATH this task's Run needs
    ContainerName string      // "" = infer --name/service from Run; override when parsing can't
    Scope       string        // "workspace" (default) | "shared" — see "Parallel-issue workflows"
}

type ReadyCheck struct {
    HTTP, TCP string
    Command   []string
}

// Open, string-keyed, compile-time registry — not a closed union, and not
// a network-fetched versioned plugin like today's internal/providers.
var Drivers = map[string]Driver{
    "process": processDriver{}, // wraps internal/process.Supervisor, near-zero change
    "command":  commandDriver{}, // one-shot shell, absorbs today's Script/Hook
}

// Start's blocking behavior depends on the task, not the driver type:
// a one-shot task (t.Detached == false) blocks until the recipe exits and
// returns a Handle already in its terminal Status(); a detached task
// (t.Detached == true) returns as soon as the process/container is
// launched, with a Handle whose Status() reflects the live supervised
// process until Stop() is called.
type Driver interface {
    Start(ctx context.Context, t Task) (Handle, error)
}
```

### `run:` execution: embedded shell by default, real shell as an opt-out

`run:` text is interpreted by an **embedded**, pure-Go POSIX/Bash-subset
shell ([`mvdan.cc/sh`](https://github.com/mvdan/sh) — also what `go-task`
uses for the same reason) rather than exec'd via `/bin/sh -c` or
`cmd /C`/PowerShell, by default. This is a `commandDriver`/`processDriver`
execution detail, not a manifest concern — `run:` is still plain shell
text to the author either way:

- No dependency on whatever shell happens to be installed/on `PATH` —
  `rm -rf dist && cp -r src dist` behaves identically on Linux, macOS, and
  Windows, including common coreutils-style builtins (`cp`, `rm`, `mkdir`,
  `mv`, ...) implemented in Go rather than shelled out to external
  binaries.
- One grammar devx controls, instead of three (`bash`/`cmd`/`powershell`)
  silently diverging between platforms.
- External programs (`docker`, `kubectl`, `brew`, ...) are unaffected —
  only the shell *glue* (pipes, `&&`, redirects, `if`/`for`) runs
  in-process; anything that isn't a builtin still execs normally via
  `PATH`.

It isn't 100% bash-compatible (no real process forking/job control inside
the interpreter, some advanced Bash extensions unsupported), so an
escape hatch exists for recipes that genuinely need a real installed
shell — an optional **`shell:`** field, same shape as `os:` (an override,
never required):

```yaml
legacy-migrate:
  desc: Needs real bash for an existing script with bash-only array tricks
  shell: bash          # exec /usr/bin/env bash -c "<run>" instead of the
                        # embedded interpreter; also accepts sh/cmd/
                        # powershell/pwsh (each invoked with its own
                        # native "run a string" flag — `-c` for bash/sh,
                        # `/C` for cmd, `-Command` for powershell/pwsh),
                        # or a custom path to any other shell binary
                        # paired with its own such flag
  run: ./scripts/legacy-migrate.sh
```

`shell:` unset (the common case) means the embedded interpreter;
set means devx execs that real shell binary with the recipe instead,
exactly like today's behavior.

```go
type Handle interface {
    Status() (string, error)
    Stop() error
    Logs() (io.ReadCloser, error)
    Exec(ctx context.Context, args []string) error
}
```

`internal/graph`'s existing `Node`/`TopoSort` (already generic, currently
underused — only consulted for process ordering today) becomes the single
scheduler for every task, including across `includes:` boundaries, instead
of three separate per-backend orderings.

Note that in this design there's no `driver:` field in the *manifest* —
every task is uniformly dispatched (container tasks are just `run: docker
run ...`/`run: docker compose ...`, handled by `commandDriver`; only
`detached: true` tasks that need process-group supervision route to
`processDriver`). The `Driver` interface exists on the Go side to keep the
container-vs-process-vs-k8s *execution* concern in one small, swappable
place per backend, without requiring the manifest author to declare which
one.

### Two axes of coupling

1. **Architectural coupling** (how many files/how far per-tech knowledge
   spreads) — fixed by the `Driver` split above.
2. **Version/surface coupling** (fragility from one external tool's CLI
   flags/output format drifting between versions) — NOT fixed by `Driver`
   alone, since `commandDriver` still shells out to whatever's in `run:`.
   A separate, ongoing concern: prefer official client SDKs (`client-go`,
   Docker's Go SDK) over CLI-shelling + text-parsing where devx itself
   needs to introspect state (not for user-authored `run:` commands, which
   are opaque shell by design); target the open Compose Specification via
   `compose-spec/compose-go` rather than the `docker` CLI binary for
   anything devx generates; capability-probe instead of version-assume;
   pin minimum supported versions and contract-test them in CI.

## Parallel-issue workflows (working on N issues at once)

A recurring pain point with AI coding agents (Copilot and others) and with
multi-issue human workflows alike: sandboxing/isolation per issue is
valuable, but "one full devcontainer + one VS Code window per issue"
quickly becomes its own management overhead (N windows, N running stacks,
N sets of ports to remember). This isn't something devx should force one
answer to — different issues genuinely need different amounts of
isolation. Instead devx should make the *cheap* end of this spectrum good,
and document the rest so people know what's available when they outgrow
it.

### Tier 1 (new, devx-native): shared devcontainer, isolated stacks

VS Code's Dev Containers extension can attach **multiple windows to one
already-running container** ("Dev Containers: Attach to Running
Container") — no rebuild, no new container, just a second window rooted at
a different path. Combined with `git worktree`, this gives near-zero-
overhead parallelism for issues that don't need hard isolation:

1. One devcontainer, built once, mounts a parent folder containing
   multiple `git worktree add`-created checkouts (`../repo-issue-123`,
   `../repo-issue-124`, ...).
2. A second/third VS Code window attaches to the same running container,
   rooted at each worktree.
3. **devx's job**: make the *running stacks* not collide inside that
   shared container. Today nothing prevents two `devx up` invocations
   (one per worktree) from colliding on container names, networks,
   volumes, or ports. This needs:
   - A **workspace id**, derived by default from the git worktree path
     (falls back to the current directory name when not inside a
     worktree), overridable via `devx up --workspace <id>`.
   - A `${workspace}` template token, usable in `containerName:` and
     anywhere `${VAR}` substitution already applies (see "Variables"
     above), so container/network/volume names get it folded in
     automatically without the manifest author doing string
     concatenation.
   - Auto port allocation is **opt-in, not retroactive magic**: devx
     cannot safely rewrite a literal host port embedded in opaque `run:`
     text (`docker run -p 5432:5432`, `--port 9000`) — it never parses
     or edits `run:` beyond port/name *sniffing* for inference. Instead
     devx exposes a `${port}` variable (same `${VAR}` substitution
     mechanism, scoped per-workspace) that a manifest author references
     explicitly — `run: ./bin/api --port ${port}` or `-p ${port}:5432`
     — to get a free ephemeral port allocated per workspace instead of a
     fixed one. A task that hardcodes a literal port with no `${port}`
     reference is unaffected by workspaces at all; if two workspaces
     both hit that same hardcoded port, `devx validate` warns on the
     collision (as already described under "Inference") rather than
     devx silently rewriting text it was never asked to control.
     `${port}` is a separate, built-in token (same family as
     `${workspace}`, not a user-defined `vars:` entry) — unrelated to
     the existing `port:` field, since a task using `${port}` is
     self-describing to the same sniffer that reads a literal port out
     of `run:`, making an explicit `port:` override unnecessary for it.
   - A new `scope: shared | workspace` task field (default `workspace`).
     `shared` tasks (e.g. one Postgres everyone's issue branches point
     at) run once regardless of how many workspaces are active; a
     second `devx up` from another workspace recognizes it's already up
     and just waits on its `ready:` check instead of starting a second
     copy. `${workspace}`/`${port}` on a `shared` task's fields is a
     validation error — a shared resource resolving to a different name
     or port per workspace would defeat the point of sharing it.
   - A new `devx workspace` subcommand: `list` (what's running, grouped
     by workspace id), `rm <id>` (tear down that workspace's containers/
     volumes and free its ports — including `shared`-scope tasks only if
     no other workspace still references them), and `devx ps
     --all-workspaces` to see everything across issues from any window.
   - **State**: per-task runtime state (logs, pids) stays `.devx/`,
     cwd-relative exactly as today — free, no change. But workspace
     bookkeeping that must be visible *across* worktrees (which
     workspaces exist, `scope: shared` reference counts for `rm`/`down`,
     `list`/`ps --all-workspaces`) can't live there, since each worktree
     has its own `.devx/` with no visibility into siblings. It instead
     lives at a path anchored to `git rev-parse --git-common-dir` — the
     one directory every worktree of the same repo already shares —
     e.g. `<git-common-dir>/devx/workspaces.json`. Still fully local,
     file-based, no daemon/network service; just a shared location
     instead of a per-worktree one for the state that's inherently
     cross-worktree. **Concurrency**: every read-modify-write of
     `workspaces.json` (registering/removing a workspace, bumping a
     `scope: shared` reference count) takes an OS file lock
     (`flock`/`LockFileEx`) on a sibling `workspaces.json.lock` for the
     duration of the update, so two `devx up`/`down`/`workspace rm`
     invocations racing from different worktree windows serialize
     instead of corrupting or lost-updating the file — still no daemon,
     just a short-held advisory lock around an otherwise-atomic
     read-modify-rename write.

This tier is explicitly **not** a security boundary: same kernel, PID
namespace, and network stack, so two workspaces can still see/signal/
resource-starve each other. It solves naming collisions and day-to-day
convenience, not isolation between adversarial or untrusted work — make
that limitation explicit in `devx --help`/docs rather than implying more
safety than it provides.

#### Launching AI coding tools per workspace

Creating the worktree and standing up the stack is only half of "start
working on this issue" — the other half is getting an AI coding tool
(Copilot CLI, Claude Code, Codex CLI, aider, ...) running against it too.
Consistent with "don't abstract the technology away": devx doesn't learn
anything about any specific agent, it just makes "run this command,
scoped to this workspace" a one-liner, the same way `run:` is a plain
shell command for every other task.

- A new `agents: {name: <shell command>}` manifest field (optional, root
  level, alongside `vars:`), e.g.:
  ```yaml
  agents:
    copilot: copilot
    claude: claude --dangerously-skip-permissions
  ```
  Each value is opaque shell text, same spirit as `run:` — devx never
  parses or special-cases a particular tool's flags.
- `devx workspace agent <id> [<name>]` — launches the named agent (or
  the single entry, if `agents:` only has one) **interactively**
  (stdin/stdout passed straight through, same as `interactive: true`
  tasks — these are REPLs, not captured-output recipes), with cwd set to
  that workspace's worktree and the same `${VAR}` substitution applied
  to its command text as `run:` gets. Env-wise it inherits exactly what
  any devx-launched process does: the real OS process environment,
  including `.env`/`.env.local` (loaded once into the whole process's
  environment at startup — see "Variables" above) — **not** `vars:`
  (textual substitution only, never exported as real env vars) and
  not another task's `env:` (per-task only). In practice this means an
  agent sees whatever secrets/config a repo already puts in `.env`, the
  same as every other task does, with no special-casing needed. No new
  execution primitive — reuses the existing embedded-shell/interactive-
  passthrough path.
- `devx workspace new <id> [--agent <name>]` — the actual "one command
  to start an issue": creates the `git worktree` (destination
  `../<repo-dir-name>-<id>`, new branch `<id>` off current `HEAD`;
  errors instead of silently reusing/overwriting if that branch or path
  already exists — use `devx workspace agent <id>` to re-attach to an
  existing one), runs `devx up --workspace <id>`, then drops straight
  into the named agent. Chains the two steps above so starting work on
  a new issue (worktree + stack + agent) is one command instead of
  three.
- `devx workspace` (bare, no subcommand) — the "I'm already in the
  devcontainer, open a new terminal, go" entrypoint: shorthand for
  `devx workspace new` that fills in gaps interactively instead of
  requiring every argument up front. Prompts for `<id>` if omitted (used
  as both the workspace id and the new branch name); prompts to pick an
  `agents:` entry only if more than one is configured and `--agent`
  wasn't given — with exactly one entry it's used automatically, no
  prompt, matching the doc's existing "zero config for the common case"
  philosophy. Prompting is TTY-only: in a non-interactive context
  (scripted, or an agent driving devx itself) it errors asking for the
  explicit `devx workspace new <id> --agent <name>` form instead of
  hanging on a prompt that can never be answered.
- Credentials/auth (API keys, OAuth tokens an agent CLI stores under
  `$HOME`) are fine to share across workspaces in Tier 1 — same
  container, same account, no isolation needed there. Per-workspace
  conversation/session state (whatever an agent persists under its own
  cwd, e.g. a `.copilot/`/`.claude/` directory) is already isolated for
  free, since cwd is the worktree and worktrees don't share files.

#### Cleaning up workspaces

Workspaces accumulate — issues get closed, PRs merge, worktrees get
deleted by hand outside devx. Two commands, both on-demand only (no
background/automatic cleanup — devx has no daemon, consistent with the
rest of this design):

- `devx workspace clean` — bulk-removes *stale* workspaces: a worktree
  directory that no longer exists (deleted outside devx, e.g. a plain
  `git worktree remove`) has its orphaned containers/volumes/ports
  reclaimed; a worktree whose branch has been merged or deleted upstream
  is a removal candidate. Any workspace with uncommitted or unpushed
  changes is skipped (warned, not deleted) unless `--force`.
- `devx workspace list --stale` — dry-run preview of exactly what
  `clean` would remove, without removing anything, so it's never a
  surprise.

#### Removing your own current workspace

A child process can't change its parent shell's working directory —
that's a Unix constraint, not a devx choice — so `devx workspace rm <id>`
or `devx workspace clean` targeting the workspace you're currently `cd`'d
into needs explicit handling instead of silently leaving the shell
sitting in a now-deleted directory:

- Detected by comparing `$PWD` against the resolved worktree path of the
  workspace(s) being removed. The actual teardown (`git worktree
  remove`, container/volume cleanup) is run from a safe cwd internally
  (the main repo root) so git itself doesn't error on "working tree in
  use."
- Always prints a plain warning regardless of shell integration:
  `Removed workspace <id> (you were inside it) — your shell is now in a
  deleted directory; run: cd <path>` — so nothing is silently confusing
  even with zero setup.
- **Optional shell integration** (documented, opt-in, never auto-
  installed): a small `devx()` shell function wrapping the real binary,
  which — after it exits — checks whether devx left a cd-target sentinel
  and, if so, `cd`s there automatically. Same trick tools like `z`/
  `direnv`/`rbenv` use to work around the same constraint. Without
  sourcing it, you get the explicit warning + a path to copy-paste;
  with it, you're dropped straight into the fallback location (the main
  repo root, unless another active workspace is designated to land in
  instead) with no extra step.

### Tier 2 (already possible today, no devx change): per-worktree devcontainer

For issues that need real process/network isolation but should stay
local: build the devcontainer image once, then run `devcontainer up
--workspace-folder <worktree>` per issue and attach a separate window to
each resulting container. Same image/build cache reused (cheap to start),
but genuine isolation (separate PID/network namespace per container).
Costs more local CPU/RAM per concurrent issue than Tier 1; devx itself
needs no changes to support this — it already works per-container exactly
as it does today.

### Tier 3 (existing tools, no devx change): DevPod / GitHub Codespaces

For full isolation, or to move the resource cost off the local machine
entirely: DevPod (reads the same `devcontainer.json`, decouples
"workspace" from "where it runs" — Docker, k8s, or cloud) or GitHub
Codespaces (one cloud codespace per issue, prebuilds for fast cold start).
Switching issues becomes switching tabs/windows instead of managing local
containers. No devx changes needed — devx still runs the same `devx.yaml`
stack inside whichever container it ends up in.

### Tier 4 (future, separate project, not building yet): `dever-labs/loom`

[`dever-labs/loom`](https://github.com/dever-labs/loom) ("AI Sandbox") is
a separate, much larger project — a central web UI with pluggable
runners/executors (persistent, desktop, and an on-demand Kubernetes
executor pool with gVisor/Kata-level isolation), plus a `remote-ide-access`
component purpose-built for exactly the gap this tier is meant to close:
a session-scoped SSH bastion issuing short-lived certs, with one-click
**"Open in IDE"** deep links for VS Code Remote-SSH and JetBrains Gateway
(no devcontainer.json-specific IDE integration — it's a plain remote SSH
host, which Gateway handles more reliably), SFTP for anything else, and
automatic port-forwarding links for dev servers running inside the
session. That combination — Codespaces-grade isolation *and* first-class
native-IDE integration instead of a browser-only editor — is the explicit
goal there.

devx's role relative to loom: devx is the thing that runs **inside** a
loom session (or a devcontainer, or bare metal) to stand up the actual
`devx.yaml`-defined stack — loom owns sandbox lifecycle/isolation/IDE
access, devx owns "what does this project's dev environment look like."
No coupling needed beyond that; this is referenced here only so the
tiering story is complete, not as a devx implementation dependency.

## Alternatives considered (and rejected)

- **Closed `ExecSpec` union** (`ContainerExec | ProcessExec | CommandExec`
  + an `Executor` interface matching it one-to-one). Rejected: still "N
  files per technology," just a smaller N — adding a technology still
  means touching the union definition and every exhaustive switch over it.
- **`driver:`/`with: map[string]any`** manifest fields (explicit driver
  selection + duck-typed config bag per task). Rejected: once `run:` is
  plain shell, there's nothing left for `driver:`/`with:` to usefully
  select — it became schema for its own sake, competing with the "minimize
  manifest surface" goal without adding real capability.
- **devx parsing/executing real `.mk` Makefile syntax**, with devx-specific
  behavior layered on as `# devx:` comment pragmas. Prototyped and
  confirmed technically workable (pragmas are inert shell comments to
  plain `make`), but rejected: the actual ask was "our own `devx.yaml` is
  too complex," not "stop using YAML" — devx.yaml stays devx's own format.
- **Before/after hooks** (`# devx:after X` / `# devx:before X`, or an
  equivalent YAML field) for "run this once, tied to another task's
  lifecycle, without being a prerequisite anyone declares." Rejected: adds
  a second ordering concept alongside `deps:` for marginal benefit: if
  something must run once a service is up, it's either a normal `deps:`
  relationship (make it a prerequisite) or a task invoked explicitly — not
  a feature devx needs to own.
- **`tasks:` wrapper key** around the flat task map. Rejected purely for
  leanness — every top-level key already unambiguously being a task
  (barring `vars:`/`includes:`) removes a level of nesting for no loss of
  clarity.
- **Make's pattern rules / wildcard targets** (`%.o: %.c`). Reviewed and
  rejected: would let one YAML entry generate many tasks by filename
  pattern, but pushes `devx.yaml` toward "a tiny programming language,"
  directly against the minimize-manifest-surface goal. A hand-written
  `includes:` file (or a small code-generation step outside devx) covers
  the rare case of truly repetitive tasks.
- **Make functions/conditionals** (`$(shell ...)`, `$(wildcard ...)`,
  `ifeq`). Reviewed and rejected for the same reason — `run:` is already
  a full shell command; anything conditional belongs inside the recipe
  script, not the manifest.
- **Order-only prerequisites** (Make's `|`, a dependency that must exist
  but doesn't force a re-run if it changes). Reviewed and rejected: only
  meaningful alongside an incremental-rebuild model for *every* task;
  devx's `sources:`/`generates:` skip is opt-in per task, so there's no
  general "did any prerequisite change" question this would answer.
- **Double-colon rules** (multiple independent recipes for the same
  target). Reviewed and rejected: no identified use case in a dev-env
  manifest; one task name should mean one recipe.

## Implementation plan

This is a breaking rewrite, not a migration: `Service`/`Dep`/`Script`/
`Hook` and the `profiles:`/`scripts:`/`hooks:` manifest keys are deleted,
not kept around behind a compatibility shim. There is no intermediate
dual-schema state to support — every `devx.yaml` in this repo's
`examples/` and any existing user manifests are rewritten to the new
shape as part of landing this.

1. `internal/config`: replace `Manifest`'s `Service`/`Dep`/`Script`/
   `Hook` fields with `Vars map[string]string`, `Includes
   map[string]string`, `Agents map[string]string` (optional, see
   "Launching AI coding tools per workspace"), and `Tasks map[string]Task`
   (every other top-level key), where `Task` unmarshals from either a
   YAML scalar (shorthand) or a mapping via a custom `UnmarshalYAML`.
   Delete the old types and `internal/config/validate.go`'s bespoke
   validation for them outright.
2. `internal/task`: `Task`, `Driver`/`Handle`, `ReadyCheck`, the port
   sniffer (parses `-p`/`--port` out of `run:`, or reads `port:`) feeding
   readiness + detached inference, and the scheduler wrapping
   `internal/graph.TopoSort`.
3. `processDriver` wrapping `internal/process.Supervisor` (near-zero
   behavior change) + `commandDriver` for one-shot tasks, including the
   one-shot-vs-detached blocking distinction noted on `Driver.Start`
   above. Both execute `run:` via an embedded `mvdan.cc/sh` interpreter
   by default, exec'ing the real `shell:`-named binary instead when that
   field is set.
4. Rewrite `cmd_up.go`/`cmd_status.go`/`cmd_logs.go`/`cmd_exec.go`/
   `cmd_down.go` onto the unified scheduler; delete the separate
   sequential process-start path in `cmd/devx/process.go`, the K8s-only
   `runUpK8s` path, and `internal/providers`'s compose-shaped provider
   data — common dep kinds (postgres/redis/etc.) become default
   `run:`/`ready:` templates devx ships itself.
5. **Adopt `spf13/cobra`** to drive `devx --help`/`devx tasks`/
   `devx tasks --json`/`devx <task> --help` and every other `cmd_*.go`
   entrypoint, rather than hand-maintaining `fmt.Println` text to match
   Cobra's own conventions — this rewrite is already touching every
   command, the best (and cheapest) time to make the switch, and it
   replaces the hand-rolled `cmd_completion.go` for free. Generate task
   listing and per-task help (name, `desc:` or truncated `run:`, `group:`
   section headers, `deps:`/`ready:` annotations) directly from the
   parsed `Manifest`; wire up bare `devx <task>` as shorthand for `devx
   run <task>`.
6. Scheduler: parallel execution of independent graph branches by default
   (Make's `-j` behavior, not opt-in); `--dry-run` (print resolved graph +
   substituted commands) and `--keep-going` flags on `up`/`run`; multi-
   target args (`devx up <name...>`, `devx run <name...>`) and `--group`
   selection reusing the `group:` field.
7. `env:` (raw pass-through to the subprocess, values resolved via the
   same `${VAR}`-substitution precedence chain as `vars:` — CLI override
   > inherited env > `.env`/`.env.local` > `vars:` defaults — never
   written as literal secrets) and `os:` (skip on non-matching platforms,
   with dependents treated as satisfied) as task fields, enforced before
   dispatching to a `Driver`; validation rejects `interactive: true`
   combined with `detached: true`, and `generates:` (without a paired
   one-shot task) combined with `detached: true` — `sources:` alone is
   valid on detached tasks (see point 12 below).
8. `interactive: true` support in `processDriver`/`commandDriver`:
   attach stdin/stdout/stderr directly instead of capturing for `logs`.
9. `sources:`/`generates:` glob matching + mtime comparison on one-shot
   tasks, gating whether `commandDriver` actually runs a task's recipe
   or reports "up to date."
10. `.env`/`.env.local` loading (root manifest's own directory only,
    not re-checked per `includes:`-file) into the inherited environment
    at startup, ahead of `vars:` resolution; `timeout:` enforcement
    around recipe execution and `ready:` waits (conservative built-in
    default when unset); `restart:` policy handling in `processDriver`/
    `commandDriver`'s supervision loop — fixed exponential backoff
    capped at 30s, `crashed` after 5 consecutive failures (mirroring
    Kubernetes's `CrashLoopBackOff`, not configurable initially) — plus
    the new `crashed`/`restarting` states in `devx status`.
11. `devx up`/`devx run` idempotency: detect already-running/ready
    detached tasks before starting them, skip with a clear "already
    running" report instead of restarting; secret redaction in
    `--dry-run`/`devx tasks --json` output for `env:` values.
12. `devx run <name> --watch`: file-watcher on a task's `sources:` globs
    with a fixed ~300ms debounce window (not configurable initially —
    a single built-in constant, matching common fsnotify/chokidar
    defaults), restarting the task on change (error if `sources:` isn't
    set — nothing to watch); `devx up --watch` as the same mechanism
    applied automatically to every `sources:`-bearing task in the
    resolved closure, isolated per task (one task's rebuild failure is
    reported inline, the rest of the stack keeps running); `devx exec
    <name> -- <cmd...>` carried over unchanged onto the new
    `Handle.Exec`.
13. `dir:` resolution (default: the defining `devx.yaml` file's own
    directory, including per-`includes:`-file; override via `dir:`) as
    the working directory for `run:` and the base for relative
    `sources:`/`generates:` globs.
14. Driver-aware `Stop()`: `processDriver` sends SIGTERM to the process
    group, waits `stopTimeout:` (conservative default when unset), then
    SIGKILL; `commandDriver` for a container-shaped `run:` instead calls
    `docker stop [-t <stopTimeout>] <name>` (name parsed the same way as
    port inference, or `containerName:` when parsing fails), falling
    back to `docker kill` on `devx down --force`. `devx up`'s default
    attached mode (interleaved, name-prefixed logs; Ctrl-C triggers the
    same stop path) and `devx up -d`/`devx logs` (no args, same
    combined-stream engine) reuse this.
15. `requires:` checked by `devx doctor` (extended to walk every task's
    `requires:`, not just devx's own built-in tool checks) and by
    `devx validate`/before `up`/`run` dispatch a recipe, producing a
    named "`<task>` needs `<tool>`" error instead of a generic shell
    "command not found"; `devx validate` also flags two tasks resolving
    to the same inferred/declared port.
16. **Workspace support** (see "Parallel-issue workflows"): `scope:
    shared | workspace` field (default `workspace`); workspace id
    resolution (`--workspace <id>` flag, defaulting to the current `git
    worktree` path when unset); `${workspace}` and the new built-in
    `${port}` token folded into the existing `${VAR}` substitution pass
    (rejecting both on `scope: shared` tasks); `${port}` resolving to a
    free ephemeral port allocated per workspace, fed back into the same
    port-sniffing path `port:` already uses for inference — devx never
    rewrites a hardcoded literal port it wasn't given a `${port}`
    reference for; a workspace registry at
    `<git-common-dir>/devx/workspaces.json` (shared across worktrees of
    the same repo, still local/file-based, no daemon) tracking active
    workspaces and `scope: shared` reference counts, read/written by
    `up`, `down`, `workspace rm`/`clean`/`list`, and `ps
    --all-workspaces` so `down`/`rm` only stop a shared task once no
    other workspace still references it.
17. **`agents:` map + agent attach** (see "Launching AI coding tools per
    workspace"): `devx workspace agent <id> [<name>]` resolves cwd to
    that workspace's worktree, resolves env the same way a task does,
    then execs the named `agents:` command with stdin/stdout/stderr
    passed straight through (reusing the `interactive: true` code path,
    not a new one); `devx workspace new <id> [--agent <name>]` chains
    `git worktree add` + `devx up --workspace <id>` + the above into one
    command; bare `devx workspace` wraps `new` with TTY-only interactive
    prompts for `<id>`/agent selection when omitted, erroring instead of
    blocking when stdin isn't a TTY.
18. **`devx workspace clean`/`list --stale`** (see "Cleaning up
    workspaces"): staleness check (worktree directory missing, or
    branch merged/deleted via `git branch --merged`/checking the remote)
    plus a dirty-worktree guard (`git status --porcelain`) gating
    deletion behind `--force`.
19. **Current-workspace-safe `rm`/`clean`** (see "Removing your own
    current workspace"): compares `$PWD` against the workspace(s) being
    torn down, runs the actual `git worktree remove`/container cleanup
    from the main repo root instead of the (about to be deleted) cwd,
    always prints the explicit `cd <path>` fallback warning, and writes
    an optional cd-target sentinel for the documented (opt-in, not
    auto-installed) shell wrapper function to pick up.

Each step keeps `go build ./... && go test ./...` green; the full
rewrite lands once all steps are done rather than being individually
releasable (there's no old behavior left to fall back to mid-rewrite).

## Open questions — resolved during review

All 18 questions raised across earlier review passes were worked through
and resolved; the decisions are reflected inline in the sections above.
Recorded here for traceability, not as still-open:

1. **Port inference misses custom launchers** → `devx validate` warns
   (see "Readiness" under "Inference").
2. **Which fields get `${VAR}`, how to escape a literal `${`** →
   enumerated explicitly (`run`, `ready.*`, `env`, `dir`,
   `sources`/`generates`, `containerName`, `agents:` entries); `$$` is
   Make's own escape for a literal `$` (see "Variables").
3. **Version/surface coupling — own doc or folded in?** → folded into
   the implementation plan per `Driver`, no separate doc.
4. **Warn on unreachable one-shot tasks?** → no; `lint`/`test`/`build`
   are intentionally ad hoc in most manifests, a default warning here
   would be noise, not signal.
5. **mtime vs. content hash for `sources:`/`generates:`** → mtime,
   matching Make/Taskfile's own default; content-hash left as a possible
   future opt-in (`method: checksum`), not adopted now.
6. **`--keep-going` default** → unchanged (fail-fast), already matches
   Make's own default (`-k` is opt-in there too).
7. **Adopt `spf13/cobra`?** → yes (see implementation plan step 5) —
   best timing, since this rewrite already touches every `cmd_*.go`
   entrypoint and currently hand-duplicates what Cobra gives for free.
8. **`--watch` debounce window** → fixed ~300ms built-in constant, not a
   new manifest field (see implementation plan step 12).
9. **`restart: on-failure` backoff/cap** → fixed exponential backoff
   capped at 30s, `crashed` after 5 consecutive failures, mirroring
   Kubernetes's `CrashLoopBackOff` (see "Reliability").
10. **`.env` discovery scope with `includes:`** → root manifest's own
    directory only, matching Compose's own single-`.env`-file behavior
    (see "Variables").
11. **`up --watch` failure isolation** → per-task; one task's rebuild
    failure is reported inline, the rest of the stack keeps running
    (see "`devx up --watch`" in "CLI surface").
12. **Implicit `dir:` surprise** → resolved `dir:` is always shown in
    `devx tasks --json`/`--dry-run`, even when inferred (see
    "Reliability").
13. **`Stop()` name-parsing failure for containers** → added
    `containerName:` override field, identical shape to `port:`;
    `devx validate` warns when it's missing and unparseable (see
    "Graceful stop, driver-aware" under "Reliability").
14. **`devx up`'s default-attached mode is a behavior change** → kept as
    the default (matches Compose precedent; no back-compat constraint
    exists for this rewrite); `-d`/`--detach` is the explicit opt-out for
    scripts/agents.

Four further questions surfaced while documenting "Parallel-issue
workflows" (workspaces/agents) and were resolved in the same review pass:

15. **How does workspace-scoped port allocation actually work, given
    `run:` is opaque shell text devx never rewrites?** → it doesn't
    retroactively make every task workspace-safe; a new built-in
    `${port}` substitution token (same family as `${workspace}`) must be
    explicitly referenced in `run:`/`ready:` to get a per-workspace
    ephemeral port. Hardcoded literal ports are untouched and only get
    `devx validate`'s existing collision warning (see "Tier 1" under
    "Parallel-issue workflows").
16. **Does `scope: shared` reference counting need cross-worktree state,
    given `.devx/` is cwd-relative per worktree?** → yes; added a
    workspace registry anchored at `git rev-parse --git-common-dir`
    (shared by every worktree of the same repo, still local/file-based,
    no daemon) alongside the untouched per-worktree `.devx/` runtime
    state, and extended `devx down` (not just `workspace rm`) to respect
    the reference count (see "Tier 1" under "Parallel-issue workflows").
17. **What env does `devx workspace agent` actually give the agent
    process — the same as a task's `env:`, or something else?** → the
    real inherited process environment, including `.env`/`.env.local`
    (loaded once globally, same as any task); explicitly *not* `vars:`
    (textual substitution only, never exported as real env vars) and not
    another task's `env:` (per-task only) (see "Launching AI coding tools
    per workspace").
18. **What exactly does `devx workspace new <id>` do to git, and what
    happens on a name collision?** → `git worktree add` at
    `../<repo-dir-name>-<id>`, new branch `<id>` off the current `HEAD`;
    errors instead of silently reusing/overwriting if that branch or
    path already exists, pointing at `devx workspace agent <id>` to
    re-attach to an existing one instead (see "Launching AI coding tools
    per workspace").
