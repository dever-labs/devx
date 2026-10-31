# Copilot instructions for devx

Guidance for AI coding agents (GitHub Copilot and others) working in this repository.

## What this repo is

`devx` is a single Go binary (`cmd/devx`) that runs a declarative, task-based
`devx.yaml` manifest directly (`run:` commands execute as processes or
containers — devx never renders to docker-compose/Kubernetes/Helm/Terraform).
Core logic lives under `internal/`; the CLI is a thin layer in
`cmd/devx/cmd_*.go` (one file per subcommand).

## Build, test, lint

```sh
go build ./cmd/devx              # build the binary → ./devx
go test ./...                    # unit tests
go test ./tests/integration/... -tags integration -v       # integration tests (needs Docker)
go test ./tests/integration/... -tags integration -short    # integration tests, skip container startup
golangci-lint run                # lint (config: .golangci.yml — govet, ineffassign, staticcheck, misspell)
```

Run the narrowest test command for the change you made (e.g. `go test ./internal/config/...`)
before falling back to the full `go test ./...`.

## Project layout

```
cmd/devx/           CLI entry point; one cmd_<name>.go per subcommand (up/down/run/
                    exec/status/logs/doctor/validate/tasks/init/ai/dev/update/...)
internal/config/    devx.yaml parsing/validation (manifest.go, tools.go, validate.go,
                    envfile.go)
internal/task/      the task execution engine: ${VAR} substitution and readiness/
                    detached/port inference from run: text (resolve.go), the
                    Driver/Handle abstraction over processes and containers
                    (driver.go + driver_unix.go/driver_windows.go), and the
                    scheduler that walks the dependency graph for devx up/run/down
                    (scheduler.go)
internal/graph/     generic dependency graph / topological sort, used by internal/task
internal/runtime/   container runtime abstraction (internal/runtime/docker,
                    internal/runtime/podman)
internal/doctor/    tool/host health checks for `devx doctor`
internal/setup/     tool install, host-side setup steps, envFiles scaffolding
                    (envfiles.go), setup state tracking
internal/detect/    package-manager/JS-workspace/tool-version inference from repo files
internal/localai/   AI backend client + model/config management for `devx ai` (OpenAI,
                    Anthropic, Azure OpenAI, Ollama, local, remote, Copilot)
internal/testexec/  cross-platform fake "runtime binary" harness used by unit tests
internal/ui/        terminal output helpers
internal/util/      small generic helpers (maps, etc.)
docs/               user-facing docs (install.md, manifest.md)
examples/           complete working project examples (examples/basic)
schemas/            JSON Schema for devx.yaml (validate with any editor via the
                    `# yaml-language-server: $schema=...` header devx writes into devx.yaml)
tests/integration/  end-to-end CLI tests (build tag: integration)
```

Note: an earlier design explored rendering devx.yaml to docker-compose/Kubernetes
manifests with provider/plugin dependency management (`devx render`/`devx lock`/
`devx providers`/`devx export`). That direction was abandoned in favor of the
task-based model above — those CLI subcommands exist only as `cmd_removed.go`
stubs that print an explanatory error; don't reintroduce `internal/compose`,
`internal/k8s`, `internal/lock`, `internal/providers`, or `internal/plugins`
packages without first confirming the direction has actually changed again.

## Conventions

- No external formatter config beyond `gofmt`/`go vet` — match existing style in the file you edit.
- Commands that produce machine-readable output expose a `--json` flag that `json.MarshalIndent`s
  a result struct and prints it alone to stdout (see `cmd_status.go`, `cmd_doctor.go`). Human-readable
  output goes to stdout, error/diagnostic text goes to stderr. When adding a new command, prefer adding
  `--json` output from the start so agents can consume it without scraping text.
- Secrets are never written to `devx.yaml`; they're read from environment variables at runtime.
- `schemas/devx.schema.json` is hand-maintained (no generator) and must be kept in sync with
  `internal/config/manifest.go` / `internal/config/tools.go` whenever the manifest shape changes.
- All generated runtime artifacts go under `.devx/` (gitignored) — do not write generated files elsewhere.
- Add/update unit tests for any behavior change; add integration tests under `tests/integration`
  for end-to-end CLI behavior. Update `docs/manifest.md` if `devx.yaml` schema/behavior changes.
- Commit messages must follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat:`, `fix:`, `chore:`, etc.) — releases are automated via release-please, which derives
  version bumps and the changelog directly from these prefixes.
- **One fix/issue per commit, one fix/issue per PR.** Never batch several unrelated fixes or
  issues into a single commit or a single PR, even if squash-merged — release-please generates
  exactly one changelog entry per commit, so batching collapses several distinct fixes into one
  vague changelog line and hides real release scope. If addressing multiple issues, open a
  separate branch/PR per issue and merge each independently.
- Before relying on any description of this codebase's structure (including this file), verify
  directories/packages actually exist (`ls`, `grep`) — this file previously went stale relative
  to a major architecture refactor and was corrected only after a manual review caught it.

## Useful entry points when exploring

- `cmd/devx/main.go` — command dispatch table.
- `cmd/devx/helpers.go` — shared helpers (manifest loading, runtime selection, `.devx` paths).
- `internal/config/` — start here to understand the manifest shape before changing CLI behavior.
- `internal/task/resolve.go`, `internal/task/scheduler.go` — start here to understand how
  devx up/run/down actually execute a manifest.

