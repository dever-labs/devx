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
directly** — no `tasks:`/`services:`/`profiles:` wrapper. `vars:` and
`includes:` are the only other reserved top-level keys.

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
instead of capturing for `logs`), `sources: [glob, ...]` / `generates:
[glob, ...]` (optional, skip re-running if `generates:` is newer than
every matched `sources:` file — see "Incremental skip" below). No
`driver:`/`with:` field — `run:` is plain shell, technology-neutral by
construction. No before/after hooks — `deps:` is the only ordering
primitive; there is no separate "runs after, without being a declared
prerequisite" concept (see "Alternatives considered").

### Inference, not required config

- **Readiness**: if `run:` exposes a port (parsed from `-p`/`--port`, or
  an explicit `port:` field), default to an HTTP GET on
  `http://localhost:<port>/healthz` with short retry/timeout; an outright
  connection refusal (no HTTP server there at all) falls back to a plain
  TCP connect on the same port; no known port → ready immediately. No
  Docker-specific healthcheck coupling — identical whether `run:` launches
  a container or a bare process. `ready:` only needs to be written to
  override this (non-standard path, or a `cmd:` check).
- **Detached vs one-shot**: a task with a port or a `ready:` is almost
  certainly long-running, so default `detached: true`; otherwise default
  `detached: false` (block until exit code). Named `detached` (the term
  the ecosystem already uses for this — Docker's own `-d`/`--detach`
  flag) rather than `background`/`service`, so it needs no definition.
  `detached:` is only written to override the inference.
- **Variables**: `${VAR}` substitution resolved by devx (not shell
  expansion, since it applies to non-shell fields like `ready.http` too).
  Precedence mirrors Make: CLI override (`devx up db PORT=5555`) >
  inherited process environment > `vars:` defaults.
- **Composition**: `includes: {name: path}` merges another `devx.yaml`'s
  tasks into **one** graph with namespaced task names (`infra:db`), not a
  recursive sub-process per file — see "Alternatives considered" for why
  recursive sub-invocation is the wrong model to copy from Make.

### Additional optional fields (from Make / modern task runners)

Reviewed against Make and modern runners (Taskfile, Just) for what else
genuinely earns its keep in a dev-environment context — see "Explicitly
not adopted" in "Alternatives considered" for what was reviewed and left
out:

- **`env: {KEY: value}`** — raw environment variables passed to the
  task's subprocess only. Deliberately separate from `vars:`: `vars:` are
  resolved by devx itself via `${VAR}` substitution into manifest string
  fields (so they can appear in `ready.http`, not just `run:`); `env:` is
  passed straight through to the OS process environment, untouched by
  substitution, which is where host-provided secrets belong (consistent
  with the repo's existing convention that secrets are never written into
  `devx.yaml` — `env:` values are typically `${HOST_ENV_VAR}` references
  resolved from the *inherited* shell environment, not literals).
- **`os: [linux, darwin, windows]`** — skip this task entirely (not an
  error) when the host OS isn't in the list. Relevant because devx already
  does cross-platform tool detection/`doctor`; some recipes (Homebrew
  installs, WSL-specific steps) are legitimately OS-specific.
- **`interactive: true`** — attach the task's stdin/stdout/stderr directly
  to the terminal instead of capturing output for `devx logs`, for tasks
  that need a live TTY (a `psql` shell, an interactive debugger). Mirrors
  the gap already implied by today's `Service.Debug` field.
- **`sources: [glob, ...]` / `generates: [glob, ...]`** — Make's actual
  core feature (skip a recipe if the target is already up to date versus
  its prerequisites), applied to one-shot tasks instead of files-as-targets
  generically. If every `generates:` glob match has a newer mtime than
  every `sources:` glob match, `devx run`/`up` skips the recipe (reports
  "up to date", same spirit as Make's own skip message). Opt-in — a task
  with neither field always runs. High value for expensive one-shot steps
  (codegen, compiling a tool, migrations keyed off a migrations
  directory) that are wasteful to always re-run.

```yaml
# --- env: vs vars: ---
db:
  run: docker run --rm -d --name db -p ${PORT}:5432 -e POSTGRES_PASSWORD=${DB_PASSWORD} ${IMAGE}
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
```

### `devx --help` reflects the manifest, not a hardcoded list

Today's CLI help is static — it lists subcommands (`up`, `status`, etc.)
but has no knowledge of what's actually defined in a given repo's
`devx.yaml`. With one flat task namespace, devx can generate task help
directly from the manifest, the same way Make's `##@`/`##` + `awk`
convention does, but built into the binary instead of hand-authored:

```
$ devx --help
...
Tasks (from devx.yaml):
  lint      golangci-lint run
  test      go test ./...
  db        Postgres for local dev
  web       App dev server
  api       Backend API                           ready: http localhost:9000/status
  migrate   Apply DB migrations                    deps: db

$ devx run migrate --help
migrate
  deps: db
  run:  ./scripts/migrate.sh up
```

- `desc:` is shown when present; otherwise the (possibly truncated) `run:`
  command itself is shown, so every task is listed even with zero extra
  config — nobody has to write `desc:` just to appear in `--help`.
- Listing is alphabetical by default; an optional `group: <string>` field
  (unset by default) lets a manifest opt into Make's `##@`-style section
  headers in the output, without being required for `--help` to work at
  all.
- `includes:`-merged tasks show under their namespaced name (`infra:db`),
  so `--help` reflects the real, fully-merged graph, not just the local
  file.
- Same data backs `devx --json` output per the repo's existing convention
  (`cmd_status.go`/`cmd_doctor.go` pattern) — `devx tasks --json` dumps the
  same structured list for tools/agents instead of scraping `--help` text.

## CLI surface

Commands and flags this adds/changes, reviewed against Make's own CLI
(`make target1 target2`, `-n`, `-k`, `-j`) and the gaps that review
surfaced:

| Command | Behavior |
|---|---|
| `devx up` | Transitive closure of every `detached: true` task's `deps:` (see worked example below for the exact rule). Independent one-shot branches run in parallel, not strictly sequential — mirrors Make's `-j` by default, not an opt-in flag. |
| `devx up <name...>` | **New.** Partial up — only the named tasks + their transitive deps, not the whole closure. Mirrors `make a b c`. |
| `devx up --dry-run` | **New.** Print the resolved graph and every substituted `run:`/`ready:` command, execute nothing. Mirrors `make -n`. |
| `devx up --keep-going` | **New.** Default is fail-fast (stop everything on first error, closest to least-surprising for a dev-env bootstrap). `--keep-going` continues unrelated branches of the graph on failure — mirrors `make -k`. |
| `devx up --group <name>` | **New.** Only tasks whose `group:` matches (+ their deps) — reuses the `group:` field already defined for `--help`, instead of inventing a second selection mechanism (e.g. tags). |
| `devx run <name...>` | Run one-shot task(s) to completion, in the order given, respecting `deps:`. Multi-target, like `devx up <name...>`. |
| `devx run <name> --dry-run` | Same dry-run semantics as `up`. |
| `devx status` | Unchanged in spirit — detached tasks: running/ready/crashed; one-shot tasks with `sources:`/`generates:`: up to date / stale, last run time, exit code. |
| `devx logs <name>` | Unchanged — tails captured output. For `interactive: true` tasks, `logs` has nothing captured (documented, not an error) since output went straight to the terminal when it ran. |
| `devx down` | Unchanged — stops every running detached task in reverse dependency order. |
| `devx tasks` / `devx tasks --json` | **New.** Dumps the full resolved task list (name, `desc`, `group`, `deps`, `ready`, `detached`, `os`, `interactive`) — the structured data backing `--help`, consumable by tools/agents per the repo's `--json` convention. |
| `devx --help` / `devx run <name> --help` | Generated from the manifest — see above. |
| `devx validate` | Unchanged command, extended checks: open questions #1 and #4 (unreachable one-shot tasks, uninferrable readiness) surface here as warnings, not errors. |

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
  run: docker run --rm -d --name db -p ${PORT}:5432 -e POSTGRES_PASSWORD=${DB_PASSWORD} ${IMAGE}
  env:
    DB_PASSWORD: ${HOST_DB_PASSWORD}   # from the inherited shell env, never a literal in devx.yaml

# ---- service needing a readiness override: TCP-open isn't "ready" here ----
redis:
  group: Services
  desc: Redis cache
  run: docker run --rm -d --name redis -p ${REDIS_PORT}:6379 redis:7
  ready: { cmd: ["redis-cli", "-p", "${REDIS_PORT}", "ping"] }

# ---- service needing BOTH overrides: `run:` has no port/flag for
#      inference to key off at all ----
queue:
  group: Services
  desc: Message queue (wrapper script hides the real port)
  run: ./scripts/start-queue.sh
  ready: { tcp: localhost:5672 }
  detached: true

# ---- data bootstrap: one-shot, chained, pulled into `up` only because
#      `api` (a detached task) transitively depends on them ----
migrate:
  group: Data
  desc: Apply DB migrations
  deps: [db]
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
  ready: { http: localhost:5173 }   # override: `npm run dev` has no literal port for inference
  detached: true                    # override: same reason — nothing to infer detached from
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
devx up --group Services       # db, redis, queue, api, web:app + their deps
devx up --dry-run              # print the resolved graph + substituted
                                # commands, run nothing
devx up --keep-going           # if `redis` fails to start, `db`/`queue`/
                                # `api` still proceed; only redis's branch stops
devx run build                 # skip, printing "up to date", if ./bin/devx
                                # is newer than every cmd/**/*.go + go.sum
devx run psql                  # attaches the terminal directly (interactive: true)
devx run install-tools         # no-op on Linux/CI (os: [darwin]), runs on macOS
devx tasks --json               # structured list for tools/agents
```

## Internal architecture

The manifest redesign is paired with an execution-model redesign so
"adding a technology" stays a one-file change on the Go side too:

```go
type Task struct {
    Name        string
    Deps        []string
    Run         string
    Ready       ReadyCheck
    Detached    bool
    Env         map[string]string
    OS          []string // empty = all platforms
    Interactive bool
    Sources     []string // glob patterns
    Generates   []string // glob patterns
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

type Driver interface {
    Start(ctx context.Context, t Task) (Handle, error)
}

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

## Migration plan

1. `internal/config`: `Manifest` gains `Vars map[string]string`,
   `Includes map[string]string`, `Tasks map[string]Task`, where `Task`
   unmarshals from either a YAML scalar (shorthand) or a mapping via a
   custom `UnmarshalYAML`. Additive — old `profiles:`/`scripts:`/`hooks:`
   keep working until step 4.
2. `internal/task`: `Task`, `Driver`/`Handle`, `ReadyCheck`, the port
   sniffer (parses `-p`/`--port` out of `run:`, or reads `port:`) feeding
   readiness + detached inference, and the scheduler wrapping
   `internal/graph.TopoSort`.
3. `processDriver` wrapping `internal/process.Supervisor` (near-zero
   behavior change) + `commandDriver` for one-shot tasks. Ship as one PR —
   delivers dependency-graph-aware `scripts:` (today's biggest concrete
   gap) standalone.
4. Migrate `Service`/`Dep`/`Script`/`Hook` onto the new `Task` model one
   profile/kind at a time; delete the old types and their bespoke
   validation (`internal/config/validate.go:90-114`) once callers migrate.
5. Migrate `cmd_up.go`/`cmd_status.go`/`cmd_logs.go`/`cmd_exec.go`/
   `cmd_down.go` onto the unified scheduler; retire the separate
   sequential process-start path in `cmd/devx/process.go` and the
   K8s-only `runUpK8s` path.
6. Collapse common dep kinds (postgres/redis/etc.) into default
   `run:`/`ready:` templates generated by devx itself; keep
   `internal/providers`'s external fetch mechanism only for
   externally-sourced/long-tail kinds, clearly labeled as the non-default
   path.
7. `devx --help`/`devx tasks`/`devx tasks --json`: generate task listing
   (name, `desc:` or truncated `run:`, `group:` section headers, `deps:`/
   `ready:` annotations) directly from the parsed `Manifest`, replacing the
   static subcommand-only help text for the task-listing portion.
8. Scheduler: parallel execution of independent graph branches by default
   (Make's `-j` behavior, not opt-in); `--dry-run` (print resolved graph +
   substituted commands) and `--keep-going` flags on `up`/`run`; multi-
   target args (`devx up <name...>`, `devx run <name...>`) and `--group`
   selection reusing the `group:` field.
9. `env:` (raw pass-through, resolved from inherited process environment
   only — never literals) and `os:` (skip on non-matching platforms) as
   task fields, enforced before dispatching to a `Driver`.
10. `interactive: true` support in `processDriver`/`commandDriver`:
    attach stdin/stdout/stderr directly instead of capturing for `logs`.
11. `sources:`/`generates:` glob matching + mtime comparison, gating
    whether `commandDriver`/`processDriver` actually runs a task's recipe
    or reports "up to date."

Each step keeps `go build ./... && go test ./...` green and ships
independently.

## Open questions

1. Port inference from parsing `run:` text (`-p 5432:5432`, `--port 9000`)
   is necessarily heuristic — covers `docker run`/most CLI flag
   conventions, but will miss custom launchers. Silent fallback to "no
   readiness check" for those, or should `devx validate` warn ("`web`
   looks long-running but no port was detected — add `ready:` explicitly")?
2. `${VAR}` substitution applies to specific fields (`run`, `ready.http`,
   `ready.tcp`, etc.) rather than the whole document — which fields need
   it enumerated, and is plain string replacement sufficient, or do we
   need escaping for a literal `$` inside a recipe?
3. Version/surface coupling (CLI-shelling vs SDKs, pinned minimum
   versions) — own design doc once this lands, or folded into the
   migration plan per `Driver` as each backend is implemented?
4. The "`up` = transitive closure of detached tasks' deps" rule (see the
   worked example above) means a one-shot task like `infra:create-bucket`
   silently never runs on `up` unless something detached depends on it —
   is that surprising enough to warrant `devx validate` flagging one-shot
   tasks that are unreachable from any detached task, in case it's a
   forgotten `deps:` edge rather than an intentionally ad hoc command?
5. `sources:`/`generates:` freshness is mtime-based like Make, which is
   simple but has Make's own known gotchas (clock skew, `git checkout`
   resetting mtimes to checkout time rather than content-change time) — is
   mtime comparison good enough for a dev-env tool, or is a content hash
   (slower, but immune to those gotchas) worth the extra implementation
   cost given these are typically small, local dev-loop tasks?
6. `--keep-going`'s default being off (fail-fast) is a judgment call for
   "what's least surprising when bootstrapping a dev environment" — should
   it default on instead, so e.g. a flaky `redis` doesn't block an
   unrelated `db`/`api` from coming up during `devx up`?
