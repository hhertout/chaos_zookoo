---
title: testkit
sidebar_position: 3
---

# testkit

`testkit` is the post-run verification middleware. After the chaos action
completes, it waits a configurable delay, queries an observability backend
(currently Grafana/Prometheus), and emits a pass/fail gauge.

Put `chaos_test_success{name="<module>"}` on your chaos dashboard — it's
the single signal that tells you "the scenario ran, and the SLO held".

## Config block

```yaml
testing:
  client: grafana                       # required — only "grafana" today
  specs:
    datasourceKind: prometheus          # optional — default: prometheus
    datasourceId: prom-uid              # required — Grafana datasource UID
    query: sum(rate(http_requests_total{code=~"5.."}[5m]))
    wait: 1m                            # optional — default: 1m
    timeWindow: 10m                     # optional — default: 10m
    operator: inf                       # optional — default: eq
    threshold: 1                        # required for numeric operators
```

## Field reference

| Field                         | Type       | Default      | Notes                                                                   |
| ----------------------------- | ---------- | ------------ | ----------------------------------------------------------------------- |
| `testing.client`              | enum       | —            | Required. Only `grafana` is supported.                                  |
| `testing.specs.datasourceKind`| enum       | `prometheus` | Only `prometheus` is supported.                                         |
| `testing.specs.datasourceId`  | string     | —            | Required. Grafana datasource UID (not name).                            |
| `testing.specs.query`         | string     | —            | Required. PromQL expression evaluated through the datasource proxy.     |
| `testing.specs.wait`          | duration   | `1m`         | Delay between the module run and the evaluation. Must be `> 0` and `<= scenario.interval`. |
| `testing.specs.timeWindow`    | duration   | `10m`        | Prometheus lookback used for `start`/`end` of `query_range`.            |
| `testing.specs.operator`      | enum       | `eq`         | One of `eq`, `neq`, `inf` (`<`), `sup` (`>`).                           |
| `testing.specs.threshold`     | number     | `0`          | The value the query result is compared to.                              |

## Environment

The Grafana client is configured from environment variables:

```bash
GRAFANA_URL=https://grafana.example.com
GRAFANA_TOKEN=<bearer-token>
```

If `GRAFANA_URL` is unset, `testkit` refuses to build any middleware and
the agent **fails at startup** — declaring a `testing:` block without
Grafana is treated as a misconfiguration, not a soft warning.

## Behavior

1. On each `Run`, the module's action executes normally.
2. `testkit` schedules a deferred evaluation using `time.AfterFunc` with
   delay = `wait`. No goroutine stays parked during the wait.
3. When the timer fires:
   - The querier runs a Prometheus `query_range` through Grafana's
     datasource proxy (`/api/datasources/proxy/uid/<id>/api/v1/query_range`).
   - Time range is `[now - timeWindow, now]`, with `step = timeWindow`
     (one sample).
   - The **last numeric value of the first series** is compared to
     `threshold` using `operator`.
4. The result is published as `chaos_test_success{name="<module>"}`:
   - `1` if the assertion passes
   - `0` if the assertion fails or the query errored

## The gauge contract

| Value | Meaning                                                              |
| ----- | -------------------------------------------------------------------- |
| `1`   | Query succeeded **and** `operator(value, threshold)` is true.        |
| `0`   | Either the query failed, no querier was configured, or the check failed. |
| *absent* | The module never ran at least once since the agent started.       |

This is by design — a flat `0` on your dashboard is always a failure,
whether the backend was unreachable or the SLO was breached. Alert on it.

## Example: "no 5xx during a rollout"

```yaml
kind: Rollout
name: rollout-checkout
metadata: {namespace: checkout}
scenario:
  interval: 1h
  matchers: {deploymentName: checkout}

testing:
  client: grafana
  specs:
    datasourceId: prod-prom
    query: sum(rate(http_requests_total{service="checkout",code=~"5.."}[5m]))
    wait: 5m
    timeWindow: 10m
    operator: inf
    threshold: 1   # tolerate < 1 req/s of 5xx
```

## Limitations

- Single value, single series. The current querier takes the last point
  of the first series — add labels to your query if the result is
  multi-series.
- Grafana only. A generic Prometheus client (no Grafana proxy) is a
  plausible next step but not shipped.
