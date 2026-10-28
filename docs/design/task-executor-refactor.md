# Design: unified task graph & pluggable executors

Status: proposal — not yet implemented. Written to align on direction before
any code changes.

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
