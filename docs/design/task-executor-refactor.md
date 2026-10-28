# Design: unified task graph & pluggable executors

Status: proposal — not yet implemented. Written to align on direction before
any code changes.

> **Revision 2** — v1 of this doc (below, mostly unchanged) proposed a
> `Task{ExecSpec}` model with a closed `ContainerExec | ProcessExec |
> CommandExec` union and an `Executor` interface. On review that still has
> real coupling left in it. See "Revision 2: what's still coupled, and the
> open-driver-registry alternative" at the end — it supersedes the exec-kind
> union and the handling of providers/hooks described below. Sections before
> it are kept for the problem statement and migration-plan framing, which
> still hold.
>
> **Revision 3** — Revision 2 fixed *architectural* coupling (one driver =
> one file) but still invented a new YAML schema (`tasks:`/`driver:`/
> `with:`) that's its own thing to learn. An earlier draft of this revision
> went as far as proposing devx parse/execute real `.mk` Makefile syntax
> directly — that was explored, built, and rejected: `devx.yaml` stays
> `devx.yaml`, a plain YAML file. What Revision 3 actually keeps from the
> Make research is the *philosophy* (flat namespace, deps + a shell recipe,
> infer rather than require config, convention-based defaults with explicit
> override) applied to a much leaner `devx.yaml` shape. See "Revision 3:
> a leaner devx.yaml, not a new file format" at the end — it supersedes the
> `tasks:`/`driver:`/`with:` manifest surface proposed in Revision 2;
> Revision 2's `Driver`/`Handle` interface split and the two-axes-of-coupling
> framing still hold as the *internal* (Go-side) architecture.

## Problem

devx currently has **three parallel orchestration models** that each
re-derive the same information (dependency order, env wiring, ports,
readiness) from `config.Profile` independently:

| Model | Where | Scheduling | Readiness | Env wiring |
|---|---|---|---|---|
| Container services + deps | `internal/compose` → `docker compose up -d` | delegated to compose's native `depends_on` | `waitForHealth` (cmd/devx/process.go) builds probes per container service after `rt.Up` returns | `resolveConnections` mutates `Service.Env` before render |
| Process services | `internal/process` (Supervisor) | `cmd/devx/process.go`: `graph.Build` + `graph.TopoSort`, then started **sequentially**, blocking on `ready.Wait` per step | same `ready.Probe` type, but driven manually in a loop | same `resolveConnections`, rewritten for host access via `hostVals` |
| Kubernetes | `internal/k8s/render.go` | implicit — whatever order `Deployment`/`Service` objects are applied in | none today | re-derived a third time from `Service.Env` |
| **`scripts:`** (the current Make-like escape hatch) | `cmd/devx/cmd_run.go` | **none** — flat list, no `dependsOn`, not reachable from the dependency graph at all | none | none |

Consequences:

1. Adding "a new technology" means writing a new renderer (compose-shaped,
   k8s-shaped, or a provider plugin) that duplicates dependency/env/readiness
   logic instead of composing with what exists.
2. `kind: <provider>` deps (`internal/providers`) couple a dep to a specific
   versioned plugin binary fetched from GitHub Releases at runtime
   (`internal/providers/fetch.go`) — this is the "strong coupling to a
   specific version" pain point: every provider release pins devx to that
   provider's binary/API shape, and the network fetch is a point of failure
   (seen first-hand: 404s on arm64 in this session).
3. `scripts:` — the part of devx closest in spirit to Make — is an island.
   It cannot declare `dependsOn` another script or a service, cannot be
   waited on by `devx up`, and isn't visible to `status`/`logs`/`down`. You
   get Make's command-authoring ergonomics but none of its dependency
   *execution* model, and none of devx's readiness/env ergonomics either.
4. `graph.Build`/`graph.TopoSort` (generic, already works on an abstract
   `Node{Name, Kind, DependsOn}`) is only consulted for **process** ordering
   and cycle validation — it is not the execution engine for containers or
   k8s, so it's a partial abstraction already pointing at the right answer.

## Goal

Keep devx's opinionated dev-env ergonomics (single `up`, readiness gating,
service links, `status`/`logs`/`exec`/`down` working uniformly) while:

- Decoupling the core from any one technology's version/CLI surface.
- Letting users define arbitrary custom commands with dependencies and have
  them participate in the *same* graph, scheduling, readiness, and `status`
  output as built-in services — i.e. actually deliver on the `scripts:`
  promise.
- Making "support a new technology" an **executor implementation**, not a
  change to every command in `cmd/devx`.

## Proposed model

### 1. `Task` — the one node type for everything

```go
// internal/task/task.go
type Task struct {
    Name      string
    DependsOn []string
    Env       map[string]string
    Ready     *Ready            // reuse internal/ready.Probe shape
    Exec      ExecSpec          // "how to run" — opaque to the graph/scheduler
}

// ExecSpec is a closed set of execution shapes, not an open technology list.
// Exactly one of these is set.
type ExecSpec struct {
    Container *ContainerExec // image/build, ports, mounts, command — today's Service{Kind:"container"} + Dep
    Process   *ProcessExec   // command/run, workdir — today's Service{Kind:"process"}
    Command   *CommandExec   // today's Script — a plain shell recipe, now first-class in the graph
}
```

`Task` subsumes `Service`, `Dep`, and `Script`. The manifest's YAML shape
barely changes (see "Migration" below) — this is an internal model the
loader builds from the existing `profiles.*.services` / `deps` / top-level
`scripts` sections, so `devx.yaml` stays close to what users already write.

### 2. One scheduler, not three

```go
// internal/task/scheduler.go
type Scheduler struct{ Executors map[ExecKind]Executor }

func (s *Scheduler) Up(ctx context.Context, tasks []Task) error
```

- Builds the graph once (reusing/extending `internal/graph`).
- Walks it in dependency order, **starting independent branches
  concurrently** (today's process path is needlessly serial; containers
  within the same compose project already start in parallel — the scheduler
  should too, modulo executor batching, see below).
- After each task's executor reports "started", runs its `Ready` probe
  (`internal/ready`, unchanged) before unblocking dependents — one readiness
  code path for containers, processes, *and* commands.
- A `CommandExec` task can `dependsOn` a container or process task and will
  simply wait for its readiness like anything else — this is the Make-like
  flow the user wants, but with devx's service primitives (ports, env,
  readiness) available to lean on instead of hand-rolled shell waits.

### 3. `Executor` — the only technology-coupled layer

```go
// internal/task/executor.go
type Executor interface {
    Start(ctx context.Context, t Task) error
    Stop(ctx context.Context, t Task) error
    Status(ctx context.Context, t Task) (Status, error)
    Logs(ctx context.Context, t Task, opts LogOptions) (io.ReadCloser, error)
    Exec(ctx context.Context, t Task, argv []string) (int, error)
}
```

- `ContainerExecutor` — wraps today's `internal/runtime.Runtime`
  (docker/podman) + `internal/compose`. For efficiency it can still batch
  all `ContainerExec` tasks into one `docker compose up -d` call (compose's
  own `depends_on` keeps working), but it now *implements* the same
  interface the scheduler calls for process/command tasks, so `status`,
  `logs`, `exec`, `down` stop having bespoke per-kind branches in
  `cmd/devx/cmd_status.go` etc.
- `ProcessExecutor` — thin wrapper around the existing, already-generic
  `internal/process.Supervisor`. Almost no change needed.
- `CommandExecutor` — runs a shell command (host or, later, inside a
  container via `docker compose exec`), fulfilling the `scripts:`/Make role
  natively in the graph instead of as a disconnected CLI command.
- `K8sExecutor` — becomes a real option *alongside* container/process rather
  than a wholly separate code path with its own `runUpK8s`.
- New technologies (nerdctl, a remote executor, …) are added by implementing
  this interface — zero changes to `cmd/devx` or the scheduler.

### 4. Providers become optional data, not required control flow

`kind: postgres` today requires fetching a provider binary at `up` time.
Proposed: a provider only ever contributes *defaults* merged into a
`ContainerExec` (default image, env var names, a healthcheck) at manifest
load time (or `devx providers install`, ahead of time) — never something the
scheduler/executor depends on at runtime. A dep with no `kind` (user
supplies `image` directly) works identically; `kind` is sugar, not a
requirement. This removes the runtime version coupling called out in #2.

## Manifest surface — intentionally small diff

```yaml
scripts:
  - name: migrate
    run: ./scripts/migrate.sh
    dependsOn: [db]          # NEW — can depend on any service/dep/script
    ready: {}                 # NEW (optional) — gate downstream tasks on this script's own success
profiles:
  local:
    services:
      api:
        dependsOn: [migrate]  # NEW — services can depend on scripts too
```

Two additions to `Script` (`dependsOn`, optional `ready`) and relaxing
`DependsOn` validation to allow referencing script names. Everything else
(`Service`, `Dep`, `Hook`) is unchanged on the YAML surface — the
`Task`/`Executor` split is an internal refactor of `cmd/devx` and
`internal/compose`/`internal/k8s`/`internal/process`, not a schema rewrite.

## Migration plan (incremental, not a rewrite)

1. Introduce `internal/task` + `Executor` interface; adapt
   `process.Supervisor` as `ProcessExecutor` (smallest, cleanest fit — no
   behavior change).
2. Fold `scripts:` into the graph as `CommandExecutor` tasks; add
   `dependsOn` to `Script`. Ship this alone as a first PR — it already
   delivers the "Make-like but structured" ask independent of the bigger
   container/k8s consolidation.
3. Wrap `internal/runtime.Runtime` + `internal/compose` behind
   `ContainerExecutor`; migrate `cmd_up.go`/`cmd_status.go`/`cmd_logs.go`/
   `cmd_exec.go`/`cmd_down.go` to call the scheduler instead of branching on
   `svc.IsProcess()` by hand.
4. Wrap `internal/k8s` behind `K8sExecutor`; retire `runUpK8s`'s separate
   code path.
5. Decouple `internal/providers` from runtime execution (defaults-only, as
   above).

Each step keeps `go build ./... && go test ./...` green and is independently
shippable/reviewable — no big-bang rewrite.

## Open questions for review

1. Should `CommandExec` support running inside an already-started container
   (like `scripts:`'s current `container: auto|true|false` against a
   devcontainer) from day one, or ship host-only first and extend later?
2. Parallel scheduling changes current behavior (process services today
   start strictly sequentially) — do we want concurrency by default, or an
   opt-in `parallel: true` per task until it's proven stable?
3. Keep `internal/graph` as the shared topo-sort, or fold it into
   `internal/task` since nothing else will use it standalone afterward?

---

## Revision 2: what's still coupled, and the open-driver-registry alternative

### Self-critique of v1

Re-reading v1 against the actual code, four coupling points survive it:

1. **`ExecSpec` is still a closed sum type.** `Container | Process | Command`
   is smaller than today's situation but has the identical shape as the
   problem: adding Kubernetes as a fourth case means touching the `ExecSpec`
   struct, the scheduler's dispatch, and every exhaustive switch over it —
   the same "new technology = edit N files" pattern, just with N reduced
   from ~6 to ~3. It doesn't fix the *pattern*, only its current severity.

2. **Providers still leak one backend's schema into "data".** v1 said
   providers become "defaults, not control flow" but didn't look closely
   enough: `internal/providers.ComposeFragment.Healthcheck` (`providers.go:66-73`)
   *is* `internal/compose.Healthcheck` by another name — a provider's output
   is already compose-shaped. A `K8sExecutor` can't consume it; it would need
   its own provider protocol, or providers need to return something
   backend-neutral. v1 didn't actually resolve this, it deferred it.

3. **"Where does this run: host or container?" is encoded three
   different, incompatible ways today, and v1 only touches one of them:**
   - `Service.Kind` (`"container"` | `"process"`) — `manifest.go:113`
   - `Hook.Exec`+`Hook.Service` (container) vs `Hook.Run` (host), with its
     own mutual-exclusion validation (`validate.go:90-114`) — this is a
     second, parallel implementation of the same container-vs-host
     decision, with its own rules (`background` only valid for `Run`, `name`
     only valid for `Run`, etc.)
   - `Script.Container` (`"auto"` | `"true"` | `"false"`) — a *third*
     mechanism, using `docker ps` label-matching and the external
     `devcontainer` CLI (`cmd_run.go:189-217`), unrelated to compose `exec`
     or to `Hook.Exec`.

   v1's `CommandExec` would become a *fourth* place this gets decided if we
   don't also fold `Hook` into the same model. Hooks were not mentioned in
   v1's migration plan at all — that's a gap, not a deferred nice-to-have:
   `Hook` is structurally just "a `Task` that runs once and blocks a phase",
   and shipping `Task` without absorbing `Hook` leaves a fourth orchestration
   model standing, not three.

4. **Readiness is closed, too.** `ready.Probe` only knows `HTTP`/`TCP`
   (`ready/ready.go`). Any new readiness signal (a shell command exit code,
   a log line pattern, a Kubernetes `readinessProbe`, a Docker `HEALTHCHECK`)
   needs a new field on `Probe` and a new branch in `ready.Wait`'s poll loop
   — same closed-union problem at a different layer.

The common root cause: **every layer models "the N ways to do X" as a closed
Go struct/union with one field per technology**, instead of as an open,
string-keyed registry of small, swappable implementations behind one
interface. Closed unions are why "add a technology" always touches multiple
files — there is always a `switch`/`if` somewhere that enumerates every kind
by name.

### The fix: one `Task`, an open driver registry, backend-neutral readiness

```go
// internal/task/task.go
type Task struct {
    Name      string
    DependsOn []string
    Env       map[string]string
    Ports     []string          // host-reachable ports, for status/links — backend-agnostic
    Ready     ReadyCheck         // see below — one shape for every backend
    Driver    string             // e.g. "container", "process", "command", "k8s" — NOT a Go type switch
    With      map[string]any     // driver-specific config, validated by the driver itself
    Transient bool               // true for one-shot tasks (today's Hooks): run to completion, don't stay "up"
}
```

- `Task` is the **only** node type. `Service`, `Dep`, `Script`, and `Hook`
  all become `Task` values at manifest-load time — four YAML shapes mapping
  onto one internal struct, not four runtime concepts. `Transient` is what
  replaces `Hook` entirely: an `afterUp` hook is just a `Task{Transient:
  true, DependsOn: [all long-running tasks]}`; `beforeDown` is a transient
  task the scheduler runs during `Stop`. One validation path
  (container-vs-host is resolved once, by the driver, not three times).

- **Drivers are a compile-time registry, not runtime plugins:**

  ```go
  // internal/task/driver.go
  type Driver interface {
      Validate(with map[string]any) error
      Start(ctx context.Context, t Task) (Handle, error)
  }

  type Handle interface {
      Status(ctx context.Context) (Status, error)
      Stop(ctx context.Context) error
      Logs(ctx context.Context, opts LogOptions) (io.ReadCloser, error)
      Exec(ctx context.Context, argv []string) (int, error)
  }

  var drivers = map[string]Driver{
      "container": containerdriver.New(), // wraps internal/runtime + internal/compose
      "process":   processdriver.New(),   // wraps internal/process (near-zero change)
      "command":   commanddriver.New(),   // today's scripts:, now graph-aware
      "k8s":       k8sdriver.New(),
  }
  ```

  This is the crucial difference from v1 and from today's provider plugins:
  registering a new driver is **one `map` entry and one small file**, with
  no change to `Task`, the scheduler, or any CLI command — the thing v1
  still got wrong for `ExecSpec`. It's also explicitly *not* the external,
  versioned-binary plugin model (`internal/providers`) — drivers ship in the
  devx binary, so there's no network fetch, no version-pinning between a
  provider release and devx's expectations, and no 404s. External,
  dynamically-loaded drivers remain a possible *future* extension (e.g. via
  Go plugins or a subprocess protocol) but are not required for the common
  case, unlike today.

- **Readiness becomes backend-neutral and driver-agnostic:**

  ```go
  type ReadyCheck struct {
      HTTP    string   // unchanged
      TCP     string   // unchanged
      Command []string // NEW — exit-code-based; works for process, container (via driver's Exec), k8s, anything
  }
  ```

  A `Command` check expressed as `["pg_isready", "-U", "postgres"]` is run
  through whatever the driver's `Exec` means for that backend (a host
  process call, a `docker compose exec`, a `kubectl exec`) — one readiness
  model, one poll loop in `internal/ready`, no backend-specific healthcheck
  type. This also directly replaces `providers.ComposeFragment.Healthcheck`:
  a provider contributes a `ReadyCheck{Command: [...]}` default, which every
  driver can execute, not just the container one.

- **Providers, revised:** a provider is reduced to a small, static,
  **in-binary** table for the common dep kinds (`postgres`, `redis`,
  `mysql`, `mongo`, …) — default image + `ReadyCheck.Command` + output var
  names. This covers the 80% case with *zero* network dependency and zero
  version coupling (it ships and versions with devx itself). The external,
  fetched-binary provider mechanism (`internal/providers/fetch.go`) is kept
  **only** as an escape hatch for long-tail/private dep kinds, clearly
  documented as the exception, not the default path every `kind:` goes
  through today.

- **Mixed backends stop being a scheduler concern.** Because batching is a
  driver-internal optimization (the `container` driver may still coalesce
  all its tasks into one `docker compose up -d`), nothing stops a profile
  mixing `driver: container` (docker), a future `driver: podman`, and
  `driver: k8s` tasks side by side — the scheduler only ever talks to the
  per-task `Handle`, never to a backend directly.

### Revised, smaller manifest surface

```yaml
# deps/services/scripts collapse onto one shape at parse time; YAML stays close to today:
services:
  api:
    driver: container        # replaces kind: container (default if omitted + image/build set)
    image: ...
services:
  web:
    driver: process           # replaces kind: process
    run: yarn dev
scripts:
  migrate:
    driver: command           # today's scripts:, now dependsOn-able
    run: ./scripts/migrate.sh
    dependsOn: [db]
hooks: # REMOVED — express as a transient command task instead:
  # afterUp: [{run: "./seed.sh"}]  →
scripts:
  seed:
    driver: command
    run: ./seed.sh
    dependsOn: [api, db]
    transient: true            # run once, don't block `down`/`status` as "running"
```

Net schema change is still small, but it **removes** a whole top-level
concept (`hooks:`) instead of only adding one (`dependsOn` on scripts) — a
real reduction in surface area, which is the thing the "keeps expanding"
concern is actually about.

### How this answers the original concern, point by point

- *"Strong coupling to a specific version"* → common dep kinds become
  in-binary drivers with no version-pinned network fetch; the versioned
  external-plugin path becomes optional, not default.
- *"Expanding becomes a new feature all the time"* → a new technology is one
  `Driver` implementation registered in a map; no change to `Task`, the
  scheduler, validation, or any `cmd/devx/*.go` file. Compare to today,
  where a new `kind` or runtime touches `compose.go`, `k8s/render.go`,
  `cmd_up.go`, `cmd_status.go`, and `validate.go`.
- *"Make gives structure for your own commands/flow but is too vague"* →
  `scripts:` tasks get `dependsOn`, readiness gating, and participate in
  `status`/`logs`/`down` for free (one scheduler), while still being "just a
  shell command" to author — Make's authoring simplicity, devx's execution
  structure.

### Updated migration plan

1. `internal/task`: `Task`, `Driver`/`Handle` interfaces, scheduler,
   backend-neutral `ReadyCheck` (additive — `HTTP`/`TCP` unchanged, `Command`
   new).
2. `processdriver` wrapping `internal/process.Supervisor` (near-zero
   behavior change) + `commanddriver` for `scripts:` with `dependsOn`. Ship
   as one PR — delivers the Make-parity ask standalone.
3. Fold `Hook` into `Task{Transient: true}` via `commanddriver`; delete
   `config.Hook`/`Hooks` and their bespoke validation once callers migrate.
4. `containerdriver` wrapping `internal/runtime` + `internal/compose`;
   migrate `cmd_up.go`/`cmd_status.go`/`cmd_logs.go`/`cmd_exec.go`/
   `cmd_down.go` onto the scheduler.
5. `k8sdriver` wrapping `internal/k8s`; retire `runUpK8s`'s separate path.
6. Collapse common dep kinds into in-binary driver defaults; keep
   `internal/providers` only for externally-sourced kinds, clearly labeled
   as the non-default path.

Each step keeps `go build ./... && go test ./...` green and ships
independently.

### Open questions (updated)

1. Where should `With map[string]any` validation live — per-driver
   (duck-typed, decentralized) or a light JSON-schema-per-driver registered
   alongside each `Driver`? The latter keeps `devx validate`/editor
   `$schema` support working without a central switch statement.
2. `Transient` tasks replacing `Hook.Background` — should a transient task
   streaming output (today's background run hooks) be a first-class driver
   behavior, or a `With` option on `commanddriver` specifically?
3. Is a compile-time driver registry sufficient long-term, or do we want to
   reserve the `Driver` interface's shape now so an *optional* out-of-process
   driver protocol (for private/long-tail tech) can be added later without
   another interface break?

---

## Revision 3: a leaner devx.yaml, not a new file format

### Course correction

A draft of this revision proposed that devx literally parse and execute
real `.mk` Makefile syntax (pragmas-in-comments, `include`, tabs, the lot).
That was built as a working proof of concept and explicitly rejected: the
target was never "stop using `devx.yaml`" — it's "our `devx.yaml` is too
complex." Make stays a research input (its philosophy), not a dependency or
a file format devx parses.

### What carries over from the Make research

- **One flat namespace**, no wrapper key. Every task name is a top-level
  key directly in `devx.yaml` — not nested under `tasks:`/`profiles:`/
  `services:`. (`vars:` and `includes:` are the only other reserved
  top-level keys.)
- **A task is a shell recipe + deps**, nothing else required. `Service`/
  `Dep`/`Script`/`Hook` collapse into one shape.
- **Shorthand for the common case**: a task can be a bare string (just the
  command — no deps, no ready, no detached) exactly like Make's "recipe
  with no prerequisites." The full object form is only needed when a task
  has dependencies or needs a readiness/detached override.
- **Infer, don't require** — config fields exist only to *override* a
  sensible convention-based default, not to be filled in by hand for every
  task. Applied here to the two fields that kept creeping back in:
  - **Readiness**: if a task's `run:` exposes a port (parsed from `-p`/
    `--port`, or an explicit `port:` field), default `ready` to an HTTP GET
    on `http://localhost:<port>/healthz` with short retry/timeout; if that
    gets an outright connection refusal (no HTTP server there at all), fall
    back to a plain TCP connect on the same port; no known port → ready
    immediately. No Docker-specific healthcheck coupling — this works
    identically whether `run:` launches a container or a bare process.
    `ready:` is only written to override the inferred default (non-standard
    path, or a `cmd:` check).
  - **Detached vs one-shot**: if a task has a port or a `ready:`, it's
    almost certainly long-running, so default `detached: true`; otherwise
    default `detached: false` (run to completion, block on exit code).
    `detached:` is only written to override that inference. Named
    `detached` (not `background`/`service`) to match the term the ecosystem
    already uses for exactly this (Docker's own `-d`/`--detach` flag) —
    unambiguous without needing a definition.
- **Variables**: top-level `vars:` block, `${VAR}` substitution resolved by
  devx itself (not shell-expanded, since not every field is a shell
  string). Precedence mirrors Make exactly: CLI override
  (`devx up db PORT=5555`) > inherited process environment > `vars:`
  defaults.

### Shape

```yaml
vars:
  PORT: 5432
  IMAGE: postgres:16

lint: golangci-lint run
test: go test ./...

db:
  run: docker run --rm -d --name db -p ${PORT}:5432 ${IMAGE}
  # no ready:/detached: written — inferred: port 5432 known, no HTTP ->
  # TCP ready check; port known -> detached: true

web:
  run: docker run --rm -d -p 8080:8080 myapp
  # inferred: port 8080 known, HTTP responds -> GET /healthz; detached: true

api:
  run: ./bin/api --port 9000
  ready: { http: localhost:9000/status }   # override: non-standard path

migrate:
  deps: [db]
  run: ./scripts/migrate.sh up
  # inferred: no port -> no ready check; no port/ready -> detached: false
```

This is the entire schema: `vars:` (map), `includes:` (map, namespaced,
merged into one graph — see Revision 2's note on avoiding the
recursive-make trap), and any other top-level key is a task — either a bare
string or `{deps, run, ready, detached}`.

### What this changes in the codebase

- `internal/config`: `Manifest` becomes `{Vars map[string]string, Includes
  map[string]string, Tasks map[string]Task}` where `Task` unmarshals from
  either a YAML scalar (shorthand) or a mapping (full form) via a custom
  `UnmarshalYAML`. `Service`/`Dep`/`Script`/`Hook` and their three
  incompatible "where does this run" mechanisms are retired together.
- `internal/graph`: unchanged — `Task.Deps` is the edge list directly;
  `TopoSort` still drives order. `includes:` targets get namespaced node
  names merged into the same graph (one `TopoSort` call, not one per file).
- Revision 2's `Driver`/`Handle` split, `ReadyCheck{HTTP, TCP, Command}`,
  and "common dep kinds become in-binary driver defaults" still hold as the
  internal execution model. What's new here is *inference* sitting in
  front of `ReadyCheck`/the detached flag, so most tasks never populate
  them explicitly.
- New: a small port-sniffer (parse `-p`/`--port` out of `run:`, or read an
  explicit `port:` field) feeding both the readiness and detached inference
  — this is the one genuinely new piece of logic Revision 3 adds beyond
  "simplify the schema."

### Updated open questions

1. Port inference from parsing `run:` text (`-p 5432:5432`, `--port 9000`)
   is necessarily heuristic — covers `docker run`/most CLI flag
   conventions, but will miss custom launchers. Is a silent fallback to "no
   ready check" acceptable for those, or should devx warn
   ("`web` looks long-running but no port was detected — add `ready:`
   explicitly") during `devx validate`?
2. `${VAR}` substitution is devx's own templating, applied to specific
   string fields (`run`, `ready.http`, `ready.tcp`, `ports`, etc.) rather
   than the whole document — which fields need it enumerated, and is plain
   string replacement sufficient or do we need escaping for a literal `$`
   in a recipe?
3. Version/surface coupling (CLI-shelling to `docker`/`kubectl` vs SDKs) is
   still a separate, ongoing concern from this schema simplification — own
   design doc once this lands, or folded into the migration plan per
   `Driver`?
