# devx

A cross-platform dev orchestrator CLI for any repo. Single-binary, container-first, and designed for offline workflows.

## 5-minute quickstart

1. Ensure Docker Engine (or Podman) is installed and running.
2. Run `devx init` to create a starter devx.yaml.
3. Edit devx.yaml for your repo services.
4. Run `devx up`.
5. Use `devx status`, `devx logs`, and `devx exec` for inspection.

## Commands

- `devx init`
- `devx up [--profile local|ci|k8s] [--build] [--pull] [--no-telemetry]`
- `devx down [--volumes]`
- `devx status`
- `devx logs [service] [--follow] [--since 10m] [--json]`
- `devx exec <service> -- <cmd...>`
- `devx doctor [--fix]`
- `devx render compose [--write]`
- `devx lock update`

## Telemetry (Aspire-lite)

devx starts a lightweight telemetry UI stack (Grafana + Loki + Prometheus) by default. Only Grafana is published, and it uses a random host port to avoid conflicts. Loki and Prometheus stay internal.

Disable telemetry with `devx up --no-telemetry`. Use `devx status` to see the Grafana URL.

## Offline mode

- Configure `registry.prefix`.
- Run `devx lock update` to resolve image digests.
- Commit devx.lock into your repo.
- `devx up` will use digest-pinned images when devx.lock is present.

## Build

```sh
go build ./cmd/devx
```

## Cross-compile

```sh
./scripts/build.sh
```

```powershell
.\scripts\build.ps1
```
