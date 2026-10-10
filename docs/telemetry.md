# Telemetry stack

devx starts a full observability stack alongside your services by default. It requires no configuration — dashboards, log collection, and metrics scraping are all pre-wired.

---

## Components

| Container | Image | Role |
|---|---|---|
| **Grafana** | `grafana/grafana:10.4.3` | Dashboard UI. Published on a random host port. |
| **Loki** | `grafana/loki:2.9.2` | Log storage and query engine. Internal only. |
| **Prometheus** | `prom/prometheus:v2.50.1` | Metrics storage and query engine. Internal only. |
| **Grafana Alloy** | `grafana/alloy:v1.1.1` | Collects logs from running Docker containers and ships them to Loki. |
| **cAdvisor** | `gcr.io/cadvisor/cadvisor:v0.49.1` | Collects container CPU and memory metrics. |
| **docker-meta exporter** | `python:3.12-alpine` | Exposes per-container network metrics and metadata for Prometheus. |

All telemetry containers run on the same Docker network as your services (`devx_default`) and are labelled so they appear in all dashboards.

---

## Accessing Grafana

After `devx up`, the Grafana URL is printed along with your other services:

```
Available services:
  Api      http://localhost:8080
  Grafana  http://localhost:54231
```

Grafana is pre-configured with:
- **Anonymous access enabled** — no login required
- Loki and Prometheus datasources pre-provisioned
- All four dashboards pre-loaded

---

## Dashboards

### Container Logs

Real-time log viewer across all services.

- **Service filter** — multi-select dropdown populated from running containers
- **Text search** — full-text filter applied to log content
- **Log rate chart** — lines/sec per service over time
- **Log stream** — live log panel with label metadata

### Container Resources

Per-container resource usage powered by cAdvisor and docker-meta exporter.

- **CPU Usage %** — per-service CPU utilisation over time
- **Memory Usage** — RSS memory per container
- **Memory Cache** — page cache per container
- **Network Rx / Tx** — bytes received and transmitted per container per second

> Network metrics come from the docker-meta exporter (Docker Stats API), not cAdvisor, because Docker Desktop does not expose per-container network namespaces to cAdvisor.

### Log Analytics

Aggregated log statistics for trend analysis.

- **Error count** and **Warning count** stats (last 5 minutes)
- **Log volume by service** — stacked bar chart
- **Error rate** — errors/sec per service over time
- **Recent errors** — log stream filtered to ERROR/FATAL/PANIC/exception

### Service Health

High-level health overview.

- **Active services** stat — number of distinct services emitting logs
- **Total errors / warnings** in the selected window
- **Top by log volume** — bar gauge of noisiest services
- **Top by error count** — bar gauge of most error-prone services
- **Top CPU consumers** — bar gauge (requires cAdvisor)
- **Top memory consumers** — bar gauge (requires cAdvisor)
- **Recent errors** — combined error log stream

---

## How log collection works

Grafana Alloy uses Docker service discovery to find running containers, then reads logs directly from the Docker daemon (no file tailing). Each log line is labelled with:

- `compose_project` — from `com.docker.compose.project`
- `compose_service` — from `com.docker.compose.service`

Only containers belonging to a Compose project are collected (system containers are excluded).

---

## How container metrics work

### CPU and memory (cAdvisor)

cAdvisor reads cgroup metrics and exposes them as Prometheus metrics. Each time series has an `id` label containing the full container ID (e.g. `id="/docker/1d37337f..."`).

### Metadata enrichment (docker-meta exporter)

Because cAdvisor's `id` label is a raw hash, dashboards use a `group_left` PromQL join to attach human-readable names:

```promql
sum by (compose_service) (
  container_memory_usage_bytes{id=~"/docker/.+"}
  * on(id) group_left(compose_service)
  docker_container_info
)
```

The docker-meta exporter queries the Docker API and exposes:

```
docker_container_info{id="/docker/<hash>", name="my-app-api-1",
                      compose_service="api", compose_project="my-app"} 1
```

### Network metrics (docker-meta exporter)

Per-container network stats are collected from the Docker Stats API (`GET /containers/{id}/stats`). All containers are polled in parallel and results are cached, so Prometheus scraping is fast. Metrics exposed:

```
docker_container_network_rx_bytes_total{compose_service="api", interface="eth0"} 12345
docker_container_network_tx_bytes_total{compose_service="api", interface="eth0"} 67890
```

---

## Enabling telemetry

```sh
devx up --telemetry
```

Telemetry is off by default; without the flag the telemetry containers are not started. (`--no-telemetry` is still accepted but is now a no-op.)

---

## Resource usage

The telemetry stack is lightweight by design:

| Container | Typical memory |
|---|---|
| Grafana | ~150 MB |
| Loki | ~50 MB |
| Prometheus | ~50 MB |
| Alloy | ~30 MB |
| cAdvisor | ~30 MB |
| docker-meta exporter | ~20 MB |

Total: ~330 MB. It is opt-in, so single-service repos pay no overhead by default.
