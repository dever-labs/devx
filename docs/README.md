# devx docs

## Quickstart

1. Install the devx binary for your OS/arch.
2. In your repo, run `devx init`.
3. Edit devx.yaml to match your services.
4. Run `devx up` (use `--no-telemetry` to disable the telemetry UI).
5. Check `devx status`.

## Offline / Airgapped

- Set `registry.prefix` in devx.yaml.
- Run `devx lock update` while you have registry access.
- Commit devx.lock so airgapped environments can use digest-pinned images.

## Generated Artifacts

devx writes generated files to `.devx/`.

- `.devx/compose.yaml`
- `.devx/state.json`
- `.devx/telemetry/*` (when telemetry dep is enabled)

## Telemetry UI

devx starts Grafana + Loki + Prometheus automatically. Grafana is published on a random host port to avoid conflicts; Loki and Prometheus are internal only.

Disable telemetry with `devx up --no-telemetry`. Check `devx status` for the published Grafana port.
