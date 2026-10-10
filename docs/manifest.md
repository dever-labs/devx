# devx.yaml reference

`devx.yaml` is the single manifest file that describes your project's services, dependencies, and profiles.

A JSON Schema is maintained at [`schemas/devx.schema.json`](../schemas/devx.schema.json). `devx init`
writes a `# yaml-language-server: $schema=...` header into generated manifests, which gives you
autocomplete and inline validation in any editor that supports the [yaml-language-server](https://github.com/redhat-developer/yaml-language-server)
protocol (VS Code with the YAML extension, Neovim, etc.) — add the same header to an existing
`devx.yaml` to opt in.

---

## Top-level structure

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/dever-labs/devx/main/schemas/devx.schema.json
version: 1

project:
  name: my-app
  defaultProfile: local

registry:
  prefix: ""

profiles:
  local:
    # ...
  ci:
    # ...
  k8s:
    runtime: k8s
    # ...
```

### `version`

Always `1`.

### `project`

| Field | Type | Description |
|---|---|---|
| `name` | string | Project name — used as the Docker Compose project name |
| `defaultProfile` | string | Profile used when `--profile` is not specified |

### `registry`

| Field | Type | Description |
|---|---|---|
| `prefix` | string | Registry prefix prepended to all images (e.g. `myregistry.azurecr.io`). Leave empty for Docker Hub. |

---

## Profiles

Each key under `profiles` is a named environment. Profiles contain two sections:

- `services` — your application containers
- `deps` — infrastructure dependencies (databases, caches, etc.)

```yaml
profiles:
  local:
    services:
      api:
        # ...
    deps:
      db:
        # ...
```

### `runtime`

Set `runtime: k8s` on a profile to use `kubectl` instead of Docker Compose:

```yaml
profiles:
  k8s:
    runtime: k8s
    services:
      api:
        image: myregistry.azurecr.io/my-app/api:latest
```

Omit `runtime` (or leave it empty) to use Docker Compose (default).

---

## Services

Services are your application containers.

```yaml
services:
  api:
    image: myimage:tag
    build:
      context: ./src/api
      dockerfile: Dockerfile
    ports:
      - "8080:80"
    env:
      DB_HOST: db
      APP_ENV: development
    command: ["./server", "--port", "8080"]
    workdir: /app
    mount:
      - "./src/api:/app:ro"
    dependsOn:
      - db
      - cache
    health:
      httpGet: http://localhost:8080/health
      interval: 5s
      retries: 10
```

| Field | Type | Description |
|---|---|---|
| `image` | string | Docker image to use. Mutually exclusive with `build`. |
| `build.context` | string | Build context path (relative to `devx.yaml`). |
| `build.dockerfile` | string | Path to Dockerfile relative to `build.context`. Defaults to `Dockerfile`. |
| `ports` | list | Port mappings in `"hostPort:containerPort"` format. |
| `env` | map | Environment variables injected into the container. |
| `command` | list | Override the container entrypoint command. |
| `workdir` | string | Working directory inside the container. |
| `mount` | list | Bind mounts in `"hostPath:containerPath[:options]"` format. Not supported in k8s render. |
| `dependsOn` | list | Service or dep names that must start first. |
| `health.httpGet` | string | URL polled after `devx up` until it returns 2xx. Blocks until healthy or timeout (2 min). |
| `health.interval` | string | Poll interval for health check (default `5s`). |
| `health.retries` | int | Maximum number of health check attempts. |

> **`image` vs `build`:** Use `image` for pre-built images. Use `build` for services built from local source. When `build` is set, `image` is ignored for Compose but **must** be set for k8s rendering.

---

## Process services

Set `kind: process` to run a host command (supervised by devx) instead of a container. Use it for repos that are not containerised, or to run one service natively while its dependencies stay in containers.

```yaml
services:
  web:
    kind: process
    run: yarn dev            # or command: ["yarn", "dev"]
    workdir: ./packages/web
    ports: ["3000"]
    env:
      PORT: "3000"
    dependsOn: [db]
    ready:
      http: http://localhost:3000/health   # or tcp: localhost:3000
      timeout: 2m
      interval: 2s
    debug: { port: 9229, type: node }
```

| Field | Description |
|---|---|
| `kind` | `container` (default) or `process`. Not allowed with the `k8s` runtime. |
| `run` / `command` | Shell string or argv list; exactly one is required for processes. |
| `workdir` | Host working directory. |
| `ready.http` / `ready.tcp` | Readiness probe; `devx up` waits for it before starting dependents. Works for containers too. Processes with `ports` and no `ready` get a TCP probe by default. |
| `ready.interval` / `ready.timeout` | Probe cadence and overall limit (default 2 minutes). |
| `debug.port` / `debug.type` | Used by `devx debug-config` (`node`, `python`, `go`, `java`). |

Processes cannot set `image`, `build` or `mount`. Containers that depend on a process reach it at `host.docker.internal`; processes reach published dependency ports on `localhost`. A process that exits before becoming ready fails `devx up` immediately. Use `devx up --attach web` to run one process in the foreground with your terminal attached.

---

## Deps

Deps are managed infrastructure containers (databases, caches). devx handles the image and standard configuration for each `kind`.

```yaml
deps:
  db:
    kind: postgres
    version: "16"
    env:
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: appdb
    ports:
      - "5432:5432"
    volume: "db-data:/var/lib/postgresql/data"

  cache:
    kind: redis
    version: "7"
    ports:
      - "6379:6379"
```

| Field | Type | Description |
|---|---|---|
| `kind` | string | Dependency type. See supported kinds below. |
| `version` | string | Image tag / version of the dependency. |
| `env` | map | Environment variables (e.g. credentials). |
| `ports` | list | Port mappings. |
| `volume` | string | Single named volume mount in `"volumeName:containerPath"` format. |

### Supported dep kinds

| Kind | Image | Notes |
|---|---|---|
| `postgres` | `postgres:<version>` | Set `POSTGRES_PASSWORD` and `POSTGRES_DB` in `env`. |
| `redis` | `redis:<version>` | No required env vars. |

---

## Health checks

> `ready` (see [Process services](#process-services)) supersedes `health.httpGet`, which remains supported.

When a service has a `health.httpGet` URL, `devx up` polls it after containers start. The command blocks until all health checks pass (up to 2 minutes), then prints the service links.

```yaml
services:
  api:
    image: myimage:tag
    health:
      httpGet: http://localhost:8080/healthz
```

The URL must be reachable from the **host machine**, so use the published host port (not the container port).

---

## Kubernetes

When `runtime: k8s` is set, `devx render k8s` and `devx up/down` use `kubectl` instead of Docker Compose.

**Constraints for k8s profiles:**

- `build` services must also set `image` — devx does not build images for k8s.
- `mount` (bind mounts) are not supported — use ConfigMaps or PersistentVolumes instead.
- Deps are rendered as Deployments + Services, same as regular services.

`devx render k8s --write` emits `.devx/k8s.yaml` with:
- A `Deployment` for each service and dep
- A `ClusterIP` Service for each container with ports defined

---

## Hooks

Hooks let you run commands at lifecycle points around `devx up` and `devx down`. Each hook is either an `exec` (runs inside a container) or a `run` (runs on the host). Hooks execute sequentially and stop on the first failure.

```yaml
profiles:
  local:
    hooks:
      afterUp:
        - exec: "migrate up"
          service: api
        - run: "./scripts/seed.sh"
      beforeDown:
        - exec: "migrate down"
          service: api
    services:
      api:
        image: myimage:tag
```

### Hook fields

| Field | Type | Required | Description |
|---|---|---|---|
| `exec` | string | one of exec/run | Command run inside `service` via `docker compose exec`. |
| `service` | string | when exec | The service name to exec into. |
| `run` | string | one of exec/run | Host shell command — runs via `sh -c` (Linux/macOS) or `cmd /c` (Windows). |

### Hook lifecycle points

| Key | When it runs |
|---|---|
| `afterUp` | After all containers are up and health checks pass |
| `beforeDown` | Before containers are stopped |

### Common patterns

**Database migrations** (exec into app container):
```yaml
hooks:
  afterUp:
    - exec: "migrate up"
      service: api
  beforeDown:
    - exec: "migrate down"
      service: api
```

**Seed data from a host script**:
```yaml
hooks:
  afterUp:
    - run: "./scripts/seed.sh"
```

**Multiple steps** (run in order, stop on first failure):
```yaml
hooks:
  afterUp:
    - exec: "migrate up"
      service: api
    - exec: "python manage.py loaddata fixtures/dev.json"
      service: api
    - run: "./scripts/notify-slack.sh"
```

---

## Full example

```yaml
version: 1

project:
  name: my-app
  defaultProfile: local

registry:
  prefix: ""

profiles:
  local:
    services:
      api:
        build:
          context: ./src/api
        ports:
          - "8080:80"
        env:
          APP_ENV: development
          DB_HOST: db
          DB_PORT: "5432"
          DB_NAME: appdb
          DB_USER: postgres
          DB_PASSWORD: postgres
          REDIS_URL: redis://cache:6379
        dependsOn:
          - db
          - cache
        health:
          httpGet: http://localhost:8080/health

    hooks:
      afterUp:
        - exec: "migrate up"
          service: api
        - run: "./scripts/seed.sh"
      beforeDown:
        - exec: "migrate down"
          service: api

    deps:
      db:
        kind: postgres
        version: "16"
        env:
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: appdb
        ports:
          - "5432:5432"
        volume: "db-data:/var/lib/postgresql/data"
      cache:
        kind: redis
        version: "7"
        ports:
          - "6379:6379"

  ci:
    services:
      api:
        image: myregistry.azurecr.io/my-app/api:latest
        env:
          APP_ENV: test
          DB_HOST: db
          DB_PORT: "5432"
          DB_NAME: appdb
          DB_USER: postgres
          DB_PASSWORD: postgres
        dependsOn:
          - db
    deps:
      db:
        kind: postgres
        version: "16"
        env:
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: appdb
        ports:
          - "5432:5432"

  k8s:
    runtime: k8s
    services:
      api:
        image: myregistry.azurecr.io/my-app/api:latest
        ports:
          - "80:8080"
        env:
          APP_ENV: production
          DB_HOST: db
          DB_PORT: "5432"
    deps:
      db:
        kind: postgres
        version: "16"
        env:
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: appdb
        ports:
          - "5432:5432"
```
