---
title: Running locally
sidebar_position: 3
---

# Running locally

A local run is the fastest loop for iterating on a scenario. You authenticate
with a plain bearer token against a remote cluster — no kubeconfig gymnastics,
no cluster-admin required.

## Point the agent at a cluster

Fill in `.env`:

```bash
K8S_HOST=https://api.my-cluster.example.com
K8S_TOKEN=$(kubectl -n chaos-system create token chaos-zookoo)
K8S_CLUSTER_CERT=$(kubectl config view --raw \
  -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')
```

:::note
`K8S_CLUSTER_CERT` must be the **base64-encoded PEM** of the cluster CA.
That's the raw value stored in kubeconfig — don't decode it manually.
:::

## A minimal scenario

Create `local/kill-demo.yaml`:

```yaml
kind: Killing
name: kill-demo
metadata:
  namespace: default
scenario:
  interval: 30s
  minAvailable: 1
  dryRun: true   # start safe
  strategy: evict
  matchers:
    labels:
      app: demo
```

Launch it:

```bash
CHAOS_CONFIG_DIR=./local ./bin/chaos_zookoo
```

You should see structured logs like:

```json
{"level":"info","ts":"...","msg":"loaded config entries","kinds":1}
{"level":"info","ts":"...","msg":"orchestrator started","modules":1}
{"level":"info","ts":"...","msg":"module scheduled",
 "module":"kill-demo","mode":"periodic","interval":"30s"}
{"level":"info","ts":"...","msg":"pod killed",
 "kind":"killing","name":"kill-demo","namespace":"default",
 "pod":"demo-7f9bcd-xyz12","strategy":"evict","dryRun":true}
```

Everything is `dryRun: true` above — no pods are actually evicted. Flip it
off once you're happy with the targeting.

## Metrics

The agent exposes Prometheus metrics on `METRICS_ADDR` (default `:9090`):

```bash
curl -s localhost:9090/metrics | grep '^chaos_'
```

See [Metrics](../concepts/metrics.md) for the full list.

## Shutdown

`SIGINT` / `SIGTERM` triggers a graceful shutdown:

1. The context passed to every module is canceled.
2. The orchestrator waits for each module loop to exit.
3. In-flight load bursts and scheduled post-run tests are drained.
4. The metrics server stops.

No signal handling you need to care about — `make run` / `Ctrl-C` does the
right thing.
