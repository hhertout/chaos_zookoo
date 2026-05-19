---
title: testkit
sidebar_position: 3
---

# testkit

`testkit` is the post-run verification middleware. After the chaos action
completes, it waits a configurable delay, queries an observability backend,
and emits a pass/fail gauge.

Put `chaos_test_success{name="<module>", namespace="<namespace>"}` on your
chaos dashboard — it's the single signal that tells you "the scenario ran,
and the SLO held".

## Config block

`specs` accepts a **list of one or more assertions**. The metric is set to
`1` only when every assertion passes; a single failure sets it to `0`.

Two client backends are supported: **Grafana** (queries Prometheus through
Grafana's datasource proxy) and **Prometheus** (queries the Prometheus HTTP
API directly).

### client: grafana

```yaml
testing:
  client: grafana
  specs:
    - datasourceKind: prometheus        # optional — default: prometheus
      datasourceId: prom-uid            # required — Grafana datasource UID
      query: sum(rate(http_requests_total{code=~"5.."}[5m]))
      wait: 1m                          # optional — default: 1m
      timeWindow: 10m                   # optional — default: 10m
      operator: inf                     # optional — default: eq
      threshold: 1
    - datasourceId: prom-uid            # second assertion
      query: up{service="my-app"}
      operator: eq
      threshold: 1
```

### client: prometheus

```yaml
testing:
  client: prometheus
  specs:
    - query: sum(kube_pod_status_ready{namespace="default",condition="true"})
      wait: 2m
      timeWindow: 10m
      operator: sup
      threshold: 3
```

No `datasourceId` needed — the Prometheus URL is configured via environment
variable (see [Environment](#environment) below).

## Field reference

| Field                             | Type     | Default      | Notes                                                                        |
| --------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------- |
| `testing.client`                  | enum     | —            | Required. `grafana` or `prometheus`.                                         |
| `testing.specs[*].datasourceKind` | enum     | `prometheus` | Only `prometheus` is supported. Ignored when `client: prometheus`.           |
| `testing.specs[*].datasourceId`   | string   | —            | Required when `client: grafana` (Grafana datasource UID). Ignored otherwise. |
| `testing.specs[*].query`          | string   | —            | Required. PromQL expression.                                                 |
| `testing.specs[*].wait`           | duration | `1m`         | Delay before this assertion runs. Must be `> 0` and `<= scenario.interval`. |
| `testing.specs[*].timeWindow`     | duration | `10m`        | Prometheus lookback used for `start`/`end` of `query_range`.                |
| `testing.specs[*].operator`       | enum     | `eq`         | One of `eq`, `neq`, `inf` (`<`), `sup` (`>`).                               |
| `testing.specs[*].threshold`      | number   | `0`          | The value the query result is compared to.                                   |

## Environment

### Grafana client

```bash
GRAFANA_URL=https://grafana.example.com
GRAFANA_TOKEN=<bearer-token>
```

### Prometheus client

```bash
PROMETHEUS_URL=http://prometheus.monitoring.svc:9090

# Bearer token (takes precedence over basic auth when both are set):
PROMETHEUS_TOKEN=<bearer-token>

# Basic auth (used when PROMETHEUS_TOKEN is unset):
PROMETHEUS_USERNAME=<username>
PROMETHEUS_PASSWORD=<password>
```

If the relevant URL env var is unset, the agent **fails at startup** when a
module declares the matching `client:` — a misconfigured backend is treated as
a hard error, not a soft warning.

Both clients can coexist: set both `GRAFANA_URL` and `PROMETHEUS_URL` if
different modules use different backends.

## Behavior

1. On each `Run`, the module's action executes normally.
2. `testkit` schedules a **single** deferred evaluation using `time.AfterFunc`
   with delay = `max(wait)` across all assertions. No goroutine stays
   parked during the wait.
3. When the timer fires, all assertions run **sequentially**:
   - Each querier runs a Prometheus `query_range` call (either through
     Grafana's datasource proxy or directly against Prometheus).
   - Time range is `[now - timeWindow, now]`, step = `timeWindow` (one sample).
   - The **last numeric value of the first series** is compared to
     `threshold` using `operator`.
   - A query error counts as a failure; remaining assertions still run.
4. The metric is emitted **once** after all assertions complete:
   - `1` if every assertion passes
   - `0` if any assertion fails or errors

## The gauge contract

| Value    | Meaning                                                              |
| -------- | -------------------------------------------------------------------- |
| `1`      | All assertions passed.                                               |
| `0`      | At least one assertion failed, errored, or no querier was configured. |
| *absent* | The module never ran at least once since the agent started.          |

This is by design — a flat `0` on your dashboard is always a failure,
whether the backend was unreachable or the SLO was breached. Alert on it.

## Example: "no 5xx and service up during a rollout" (Grafana)

```yaml
kind: Rollout
metadata: {name: rollout-checkout, namespace: checkout}
scenario:
  interval: 1h
  matchers: {deploymentName: checkout}

testing:
  client: grafana
  specs:
    - datasourceId: prod-prom
      query: sum(rate(http_requests_total{service="checkout",code=~"5.."}[5m]))
      wait: 5m
      timeWindow: 10m
      operator: inf
      threshold: 1   # tolerate < 1 req/s of 5xx
    - datasourceId: prod-prom
      query: up{service="checkout"}
      wait: 2m
      operator: eq
      threshold: 1   # service must be up
```

## Example: "pods recover after a kill" (direct Prometheus)

```yaml
kind: Killing
metadata: {name: kill-api, namespace: api}
scenario:
  interval: 5m
  minAvailable: 1
  matchers:
    labels: {app: api}

testing:
  client: prometheus
  specs:
    - query: sum(kube_pod_status_ready{namespace="api",condition="true"})
      wait: 2m
      timeWindow: 5m
      operator: sup
      threshold: 2   # at least 3 ready pods after recovery
```

## Limitations

- Single value, single series. The current querier takes the last point
  of the first series — add labels to your query if the result is
  multi-series.
