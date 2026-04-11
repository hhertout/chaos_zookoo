# Chaos Zookoo

**Chaos Zookoo** is a Kubernetes chaos engineering agent that operates exclusively through the Kubernetes API server — no cluster-admin, no DaemonSets, no node-level privileges.

Every disruption is expressed as a regular API call (`EvictV1`, `Pods.Delete`, `Deployments.Patch`, ...) via `client-go`. The agent runs as a single long-running process authenticated as a `ServiceAccount`, which means **the cluster's RBAC is the security model**: whatever the ServiceAccount can do, the agent can do; whatever it can't, the agent can't.

This is the whole point. Platform teams can grant fine-grained, auditable chaos permissions instead of deploying a framework with cluster-admin rights.

## How it works

Configuration is YAML-driven, similar to Chaos Mesh CRDs. Each YAML document declares a `kind:` that maps to a **module**. Modules run on their own schedule (`interval`, `cron`, or `once`) and can be wrapped with orthogonal **middlewares** — synthetic HTTP load, post-run observability assertions — without the module knowing they exist.

```
YAML config(s)
  └── each document → module (Killing, GorillaKill, Rollout, ...)
        └── optional middlewares (load:, testing:)
              └── orchestrator schedules one goroutine per module
```

## Modules

| Kind | What it does |
| --- | --- |
| `Killing` | Picks **one random pod** from the matched pool and evicts or deletes it per tick. Respects a `minAvailable` safety floor. |
| `GorillaKill` | Deletes **every** matched pod in one pass. Simulates a full-zone outage. Can run once or periodically. |
| `Rollout` | Triggers a `rollout restart` on targeted Deployments, DaemonSets, or StatefulSets. Equivalent to `kubectl rollout restart` on a schedule. |

## Prerequisites

- Go 1.21+
- A Kubernetes cluster reachable from your machine (or a `ServiceAccount` token for in-cluster use)
- `golangci-lint` and `gofumpt` for development

## Installation

### From source

```bash
git clone https://github.com/hhertout/chaos_zookoo.git
cd chaos_zookoo
make build
# binary is at ./bin/chaos_zookoo
```

### Helm (in-cluster)

```bash
helm install chaos-zookoo ./helm \
  --set config.k8sHost=https://your-api-server \
  --set config.k8sToken=<sa-token> \
  --namespace chaos-zookoo --create-namespace
```

## Quick start

1. Copy `.env.example` to `.env` and fill in your cluster credentials:

```bash
cp .env.example .env
```

```dotenv
K8S_HOST=https://your-api-server:6443
K8S_CLUSTER_CERT=<base64-encoded-CA>
K8S_TOKEN=<service-account-token>
```

2. Write a config file (see `examples/` for annotated samples):

```yaml
kind: Killing
name: kill-my-app
metadata:
  namespace: default
scenario:
  interval: 60s
  minAvailable: 1
  strategy: evict
  matchers:
    labels:
      app: my-app
```

3. Run the agent:

```bash
./bin/chaos_zookoo --config ./examples
# or point to a single file
./bin/chaos_zookoo -C ./examples/killing.yaml
```

Config source priority: `--config` flag > `$CHAOS_CONFIG_DIR` > `./configs`.

## Configuration reference

### Common fields

Every module document shares the following top-level fields:

```yaml
kind: <Killing|GorillaKill|Rollout>   # required
name: <string>                         # required — appears in every log line
metadata:
  namespace: <string>                  # required
```

### Scheduling

`interval` and `cron` are mutually exclusive. `wait` adds an initial delay before the first execution (must be less than `interval`; ignored with `cron`).

```yaml
scenario:
  interval: 5m          # Go duration: 30s, 5m, 1h...
  # cron: "*/5 * * * *" # standard 5-field cron expression (alternative)
  wait: 10s             # optional — delay before first execution
  dryRun: false         # log what WOULD happen without touching the API
```

### Matchers

All pod-targeting modules accept the same matcher block. Multiple matchers are unioned:

```yaml
matchers:
  labels:               # all pods with these labels (AND'd)
    app: my-app
  # podName: my-pod-abc123
  # deploymentName: my-deployment
  # daemonsetName: my-daemonset
  # statefulsetName: my-statefulset
```

`Rollout` only accepts workload matchers (`deploymentName`, `daemonsetName`, `statefulsetName`).

### Module: Killing

```yaml
kind: Killing
name: kill-my-app
metadata:
  namespace: default
scenario:
  interval: 60s
  minAvailable: 1       # pods that must stay alive — kill is a no-op if floor reached
  strategy: evict       # evict (respects PDBs) | delete (hard delete)
  matchers:
    labels:
      app: my-app
```

### Module: GorillaKill

```yaml
kind: GorillaKill
name: gorilla-my-app
metadata:
  namespace: default
scenario:
  when: once            # once | periodic
  # interval: 10m       # required when when: periodic
  matchers:
    labels:
      app: my-app
```

### Module: Rollout

```yaml
kind: Rollout
name: rollout-my-deployment
metadata:
  namespace: default
scenario:
  interval: 1h
  matchers:
    deploymentName: my-deployment
```

### Middleware: Synthetic load (`load:`)

Fires an HTTP burst **in parallel** with the module's run. Useful to generate realistic traffic during the disruption:

```yaml
load:
  vus: 10               # virtual users (concurrent goroutines)
  duration: 30s         # how long the burst lasts
  requests:
    method: POST
    url: https://my-service.example.com/api/endpoint
    interval: 1s
    contentType: application/json
    body: '{"key": "value"}'
```

### Middleware: Post-run assertion (`testing:`)

After each module run, queries a Prometheus datasource via Grafana and records the result on the `chaos_test_success{name="<module>"}` gauge (1 = pass, 0 = fail). Requires `GRAFANA_URL` and `GRAFANA_TOKEN` environment variables.

```yaml
testing:
  client: grafana
  specs:
    datasourceKind: prometheus      # default
    datasourceId: my-ds-uid
    query: sum(kube_pod_status_ready{namespace="default",condition="true"})
    wait: 30s                       # delay after run before querying (default: 1m)
    timeWindow: 10m                 # Prometheus lookback window (default: 10m)
    operator: sup                   # eq | neq | inf | sup (default: eq)
    threshold: 2
```

## Metrics

The agent exposes Prometheus metrics on `METRICS_ADDR` (default `:9090`):

| Metric | Description |
| --- | --- |
| `chaos_test_success{name}` | Result of the last `testing:` assertion (1 = pass, 0 = fail) |
| `chaos_loading_http_active` | In-flight HTTP requests from `load:` bursts |
| `chaos_load_requests_total` | Total HTTP requests sent by `load:` |
| `chaos_load_request_duration_seconds` | Histogram of `load:` request latencies |

## Make targets

| Target | Description |
| --- | --- |
| `make build` | Compile `./cmd/chaos_zookoo` → `bin/chaos_zookoo` |
| `make run` | Build and run (reads `.env` and `CHAOS_CONFIG_DIR`) |
| `make test` | `go test -v -race ./...` |
| `make test-cover` | Race-enabled coverage + `go tool cover -func` |
| `make lint` | `golangci-lint run ./...` |
| `make fmt` | `gofumpt -w .` |
| `make check` | Full CI gate: tidy + fmt + vet + lint + test |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for environment setup, conventions, and how to add a new module.

## License

MIT
