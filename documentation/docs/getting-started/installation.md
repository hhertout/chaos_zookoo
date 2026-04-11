---
title: Installation
sidebar_position: 1
---

# Installation

`chaos_zookoo` is distributed as:

- a Go binary (built from source) for local runs against a cluster,
- a container image + Helm chart for in-cluster deployment.

## Prerequisites

- **Go 1.25+** if you build from source.
- **A reachable Kubernetes cluster** and an out-of-cluster kubeconfig (or,
  in-cluster, a `ServiceAccount` with appropriate RBAC — see
  [RBAC requirements](../deployment/rbac.md)).
- **Grafana** (optional) if any of your modules declare a `testing:` block.

## Build from source

```bash
git clone https://github.com/hhertout/chaos_zookoo.git
cd chaos_zookoo
make build
# → ./bin/chaos_zookoo
```

The `Makefile` exposes the standard quality gates:

| Target              | What it does                                    |
| ------------------- | ----------------------------------------------- |
| `make build`        | Produces `bin/chaos_zookoo`.                    |
| `make test`         | `go test -v -race ./...`                        |
| `make lint`         | `golangci-lint run ./...`                       |
| `make fmt`          | `gofumpt -w .`                                  |
| `make check`        | Full CI gate: tidy + fmt + vet + lint + test.   |

## Run locally

Drop a `.env` alongside the binary (see [`.env.example`](https://github.com/hhertout/chaos_zookoo/blob/main/.env.example)):

```bash
# Kubernetes connection (out-of-cluster)
K8S_HOST=https://api.my-cluster.example.com
K8S_TOKEN=<bearer-token>
K8S_CLUSTER_CERT=<base64-encoded-CA-cert>

# Where to find scenarios
CHAOS_CONFIG_DIR=./examples

# Prometheus /metrics (default :9090)
METRICS_ADDR=:9090

# Only required if any module declares a testing: block
GRAFANA_URL=
GRAFANA_TOKEN=
```

Then:

```bash
./bin/chaos_zookoo                      # reads $CHAOS_CONFIG_DIR or ./configs
./bin/chaos_zookoo --config ./examples  # explicit directory
./bin/chaos_zookoo -C ./my.yaml         # single file, multi-doc is fine
```

## Deploy in-cluster (Helm)

The chart lives under `helm/` in the repository. See the dedicated
**[Deploy with Helm](./helm.md)** page for the full walkthrough: credentials,
scenario configs, `extraEnv` for testkit, `extraSecrets` for sensitive values,
and the complete `values.yaml` reference.
