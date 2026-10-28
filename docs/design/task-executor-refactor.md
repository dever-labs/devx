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
config needed) or an object, needed **only** when a task has `deps:` or an
explicit `ready:`/`detached:` override. Task names are **top-level keys
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
{http|tcp|cmd}` (override only), `detached: true|false` (override only).
No `driver:`/`with:` field — `run:` is plain shell, technology-neutral by
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

## Internal architecture

The manifest redesign is paired with an execution-model redesign so
"adding a technology" stays a one-file change on the Go side too:

```go
type Task struct {
    Name     string
    Deps     []string
    Run      string
    Ready    ReadyCheck
    Detached bool
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
