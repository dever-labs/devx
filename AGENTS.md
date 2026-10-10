# AGENTS.md

Guidance for AI coding agents (and humans in a hurry) working in this repository.

## What this repo is

`devx` is a single Go binary (`cmd/devx`) that orchestrates local dev environments
from a declarative `devx.yaml` manifest. Core logic lives under `internal/`; the
CLI is a thin layer in `cmd/devx/cmd_*.go` (one file per subcommand).

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
cmd/devx/           CLI entry point; one cmd_<name>.go per subcommand
internal/config/    devx.yaml parsing and validation
internal/compose/   Docker Compose rendering + telemetry stack
internal/runtime/   container runtime abstraction (Docker, Podman)
internal/k8s/       Kubernetes manifest rendering
internal/graph/     dependency graph / topological sort
internal/lock/      lockfile (devx.lock) management
internal/doctor/    tool/health checks for `devx doctor`
internal/setup/     tool install, host-side setup steps, envFiles generation
internal/detect/    tool versions/JS workspaces inferred from repo files
internal/process/   detached process supervisor (state in .devx/processes.json)
internal/ready/     HTTP/TCP readiness probes
internal/ai/         LLM client (openai, anthropic, azure-openai, ollama)
internal/providers/ third-party dependency provider plugins
internal/plugins/   plugin discovery/execution
docs/               user-facing docs (install.md, manifest.md, telemetry.md)
examples/           complete working project examples
schemas/            JSON Schema for devx.yaml (validate with any editor via the
                    `# yaml-language-server: $schema=...` header devx writes into devx.yaml)
tests/integration/  end-to-end CLI tests (build tag: integration)
```

## Conventions

- No external formatter config beyond `gofmt`/`go vet` — match existing style in the file you edit.
- Commands that produce machine-readable output expose a `--json` flag that `json.MarshalIndent`s
  a result struct and prints it alone to stdout (see `cmd_status.go`, `cmd_doctor.go`). Human-readable
  output goes to stdout, error/diagnostic text goes to stderr. When adding a new command, prefer adding
  `--json` output from the start so agents can consume it without scraping text.
- Secrets are never written to `devx.yaml`; they're read from environment variables at runtime
  (see `internal/ai/ai.go` header comment for the exact env var per provider).
- `schemas/devx.schema.json` is hand-maintained (no generator) and must be kept in sync with
  `internal/config/manifest.go` / `internal/config/tools.go` whenever the manifest shape changes.
- All generated runtime artifacts go under `.devx/` (gitignored) — do not write generated files elsewhere.
- Add/update unit tests for any behavior change; add integration tests under `tests/integration`
  for end-to-end CLI behavior. Update `docs/manifest.md` if `devx.yaml` schema/behavior changes.

## Useful entry points when exploring

- `cmd/devx/main.go` — command dispatch table.
- `cmd/devx/helpers.go` — shared helpers (manifest loading, runtime selection, `.devx` paths).
- `internal/config/` — start here to understand the manifest shape before changing CLI behavior.
