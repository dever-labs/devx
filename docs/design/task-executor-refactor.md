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
> one file) but still invented a new YAML schema (`driver:`/`with:`) that
> devs (and the AI assistants writing these manifests) have to learn, when
> they already fluently write Makefiles. Revision 3 drops the custom YAML
> task schema entirely in favor of **real GNU Make syntax** as the authoring
> surface, with devx adding only the handful of things Make genuinely can't
> express, as inert comment pragmas. See "Revision 3: stop inventing a DSL —
> build on real Makefiles" at the end. It supersedes the `tasks:`/`driver:`/
> `with:` manifest surface proposed in Revision 2; Revision 2's `Driver`/
> `Handle` interface split and the two-axes-of-coupling framing still hold
> as the *internal* (Go-side) architecture.

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

## Revision 3: stop inventing a DSL — build on real Makefiles

### Why Revision 2 still didn't land

Revision 2 fixed *architectural* coupling: adding a technology became "one
new `Driver` implementation" instead of "touch every exhaustive switch."
But it kept inventing schema — `tasks:`, `driver:`, `with: map[string]any`,
`transient:` — that nobody needed to learn before, and that stops being a
duck-typed config bag only if we also hand-roll a `$schema`-per-driver
validation story (open question #1, still unresolved). Net effect: smaller
blast radius per change, but still a bespoke YAML dialect competing with a
tool (Make) that already does 80% of this, that the project team already
reaches for, and that AI assistants already write fluently with zero
prompting.

Two follow-up questions from review sharpened this further:

1. **"Aren't we still coupling to technologies via the driver, just one
   level down?"** Yes — drivers still shell out to `docker compose`/
   `kubectl` CLIs (confirmed in `internal/runtime/docker/docker.go` and
   `internal/k8s/kubectl.go`). That's a *second*, independent axis of
   coupling Revision 2 doesn't address:
   - **Architectural coupling** (how many files/how far per-tech knowledge
     spreads) — fixed by the `Driver` split.
   - **Version/surface coupling** (fragility of one external tool's CLI
     flags/output format drifting between versions) — NOT fixed by
     `Driver` alone. Mitigations, independent of the schema question:
     prefer official client SDKs (`client-go`, Docker's Go SDK) over
     CLI-shelling + text-parsing; target the open Compose Specification via
     `compose-spec/compose-go` instead of the `docker` binary;
     capability-probe instead of version-assume; pin minimum supported
     versions and contract-test them in CI.
2. **"AI makes writing a Makefile easy — an abstraction layer over the
   technology is a waste of time."** Correct, and the sharper framing of
   the whole redesign: devx's job was never "hide Docker/K8s behind a nicer
   syntax" — recipes (`docker run ...`, `kubectl apply ...`) are already
   trivial to write and should stay plain shell. devx's job is the small set
   of things *about running a dev environment* that Make has no concept of
   at all: is this thing actually ready (not just "did the command exit
   0"), supervising something long-running without blocking the shell, and
   giving a unified `status`/`logs`/`down` across whatever mix of
   containers/processes/k8s a repo happens to use.

### What's good about Make, concretely (research)

- **One model for everything**: `target: prerequisites` + a shell `recipe`.
  No "kind" field, no union type — category (build-artifact vs always-run
  action) is inferred from behavior (`.PHONY`), not a different schema.
  Directly contradicts today's devx reality, where "where/how does this run"
  is encoded three incompatible ways (`Service.Kind`, `Hook.Exec`/`Run`,
  `Script.Container` auto/true/false) — see `internal/config/manifest.go`
  and `internal/config/validate.go:90-114`.
- **Automatic variables** (`$@`, `$<`, `$^`) and variable flavors (`=`
  recursive, `:=` immediate, `?=` default-if-unset, `+=` append) with a
  well-known precedence: CLI override (`make target VAR=val`) > inherited
  shell environment > Makefile defaults. Nothing to design — it's already
  the mental model most engineers (and CI configs) already use daily.
- **`include`** splices files flatly (no namespacing — a real limitation:
  name collisions are the includer's problem). **Recursive `$(MAKE) -C
  dir`** per sub-project is the traditional alternative, but ["Recursive
  Make Considered Harmful"](https://aegis.sourceforge.net/auug97.pdf)
  (Miller, 1997) shows it's broken: each sub-make has its own separate
  dependency graph, so a target in one submake can't correctly depend on a
  target in another, and `-j` parallelism can't cross the boundary. Modern
  tools (go-task's `includes:`) fix this by merging multiple files into
  **one** unified graph with namespaced task IDs instead of spawning
  sub-processes — the model devx should copy for `include`.
- **Self-documenting `help`**: the `##@ Section` + `## description` comment
  convention + one `awk` one-liner (seen verbatim in
  [`dever-labs/homelab`'s `Makefile`](https://github.com/dever-labs/homelab/blob/main/Makefile))
  turns a flat target list into grouped, discoverable `make help` output for
  free — comments as documentation, not as parsed config.
- That same homelab `Makefile` is also evidence for the "stop abstracting
  the technology" point directly: every target is a one-line dispatch to
  `scripts/minas.sh <subcommand>`; all real logic lives in one script, the
  Makefile is purely a thin, discoverable command list. It does not use
  `deps:`/prerequisites at all — confirming devx's distinct value-add is
  specifically the service-lifecycle case (`up`/readiness/background), not
  "yet another command runner" (Make/the homelab pattern already nails
  that part on its own).

### Proposal: devx reads real Makefiles, adds three pragmas

Primary authoring surface becomes a real `Makefile` (or an included
`devx.mk`), parsed with Make's own grammar — devx does not define a new
file format. On top of the ordinary target/prerequisite/recipe/variable/
`include` model, devx recognizes a small, fixed set of `# devx:` comment
lines **inside a target's own recipe** (never attached to some other
target) for the things Make cannot express:

```makefile
PORT  ?= 5432
IMAGE ?= postgres:16

include web/devx.mk   # merged into ONE graph, namespaced (web:test, …)

.PHONY: help
.DEFAULT_GOAL := help

help: ## Show available tasks
	@awk 'BEGIN {FS = ":.*## "} /^##@/ {printf "\n%s\n", substr($$0,5)} \
	  /^[a-zA-Z0-9_-]+:.*## /{printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ Services
db: ## Postgres ($(IMAGE) on :$(PORT))
	# devx:background
	# devx:ready tcp:localhost:$(PORT)
	docker run --rm -d --name db -p $(PORT):5432 $(IMAGE)

migrate: db ## Apply DB migrations
	./scripts/migrate.sh up
```

Recognized pragmas (deliberately minimal — this is the entire new surface):

- `# devx:background` — start detached/supervised instead of blocking;
  `devx up` runs it async, `devx down` stops it, `devx logs <target>` tails
  captured output. Internally this is `processdriver` from Revision 2,
  unchanged — only how it's declared changes.
- `# devx:ready <tcp:host:port | http:url | cmd:...>` — backend-neutral
  readiness gate before dependents proceed; maps directly onto Revision 2's
  `ReadyCheck{HTTP, TCP, Command}`.
- Nothing else. **Before/after hooks were proposed and explicitly
  rejected** — ordinary prerequisites (`migrate: db`) already express "run
  before"; there is no separate "after" pragma. If something needs to run
  once a service is up without being a prerequisite anyone declares, that's
  just another target a human or `devx up` invokes explicitly — not a
  feature devx needs to own.

Because pragmas are plain `#` comments inside a tab-indented recipe line,
they're inert shell comments to `make` itself — verified by running a
trimmed version of the example above with plain `make -f devx.mk db`: the
pragma lines print as literal shell comments, then `docker run` executes
normally. **devx is a strict superset of Make** — the same file works with
zero devx installed, and `devx up`/`status`/`logs`/`down` add readiness/
supervision/unified-visibility on top by reading the same pragmas.

### What this changes in the codebase

- `internal/config`: gains a Make parser (or a thin wrapper around
  `compose-spec`-style parsing — likely a small from-scratch parser, since
  GNU Make's grammar for `target`/`prereqs`/tab-recipe/`include`/variable
  flavors is compact) instead of extending `Manifest`/`Service`/`Dep`/
  `Script`/`Hook`. `devx.yaml` is not removed — it still owns `tools:`,
  `setup:`, `envFiles:`, and profile/runtime selection, none of which is a
  task-graph concept.
- `internal/graph`: unchanged — Make's own prerequisite list *is* the
  `dependsOn` edge list; `TopoSort` still drives execution order.
  `include` targets get namespaced node names (`web:test`) merged into the
  same graph, not a separate `TopoSort` call per file.
- Revision 2's `Driver`/`Handle` interface split, `ReadyCheck`, and the
  "common dep kinds become in-binary driver defaults" point all still hold
  as the internal execution model — only the authoring surface (Make
  pragmas vs `driver:`/`with:` YAML) changed.
- `Service`/`Dep`/`Script`/`Hook` and their three incompatible "where does
  this run" mechanisms (`Kind`, `Exec`/`Run`, `Container` auto-detection)
  are retired together, replaced by: a Make target is a task; `# devx:
  background` is the only "runs detached" signal needed.

### Updated open questions

1. How far does devx's Make parser need to go — full GNU Make grammar
   (pattern rules, `ifeq`, functions like `$(wildcard …)`), or a documented
   subset sufficient for dev-env manifests (targets, prerequisites, tab
   recipes, `=`/`:=`/`?=`/`+=` variables, `include`, `.PHONY`)? Supporting
   the full grammar is more work for marginal benefit if nobody writes
   `ifeq` in a dev-env file in practice.
2. Should devx *shell out to* a real `make` binary for execution (simplest,
   100% semantic fidelity, but adds `make` as a runtime dependency devx
   currently doesn't have) or parse-and-execute itself (no new runtime
   dependency, but devx now owns Make-compatible execution semantics,
   e.g. recipe lines run in one shell vs one shell per line, `.ONESHELL`)?
3. Version/surface coupling (axis 2 above) is a separate, ongoing effort
   from this redesign — does it get its own design doc once this one lands,
   or get folded in as each `Driver` is implemented in the migration plan?
