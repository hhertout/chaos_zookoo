---
title: Overview
sidebar_position: 1
---

# Middlewares overview

A **middleware** is a decorator that wraps a `ChaosModule` and adds a
cross-cutting concern — load generation, observability assertions,
rate-limiting, … — without the module knowing about it.

In Go terms:

```go
type Middleware func(ChaosModule) ChaosModule
```

Middlewares preserve `Name()` and `Schedule()` and only add behavior
around `Run()`. They are declared on the module's YAML as **top-level**
sibling keys of `scenario:`:

```yaml
kind: Killing
metadata: {name: kill-with-load, namespace: default}
scenario:
  interval: 1m
  minAvailable: 1
  matchers: {labels: {app: my-app}}

load:         # loadkit middleware
  vus: 10
  duration: 30s
  requests: {method: GET, url: https://my-app.example.com/}

testing:      # testkit middleware
  client: grafana
  specs:
    - datasourceId: prom-uid
      query: sum(rate(http_requests_total{code=~"5.."}[5m]))
      operator: inf
      threshold: 1
```

Neither `load:` nor `testing:` is mandatory. When absent, the middleware
returns a no-op wrapper — **zero overhead** at runtime.

## Built-in middlewares

| Key        | What it does                                           | When it runs                | Page                                 |
| ---------- | ------------------------------------------------------ | --------------------------- | ------------------------------------ |
| `load`     | Fires a synthetic HTTP burst for `duration`.           | **In parallel** with `Run`. | [loadkit](./loadkit.md)              |
| `testing`  | Queries a Grafana datasource and sets a gauge.         | **After** `Run`, deferred.  | [testkit](./testkit.md)              |

## Composition order

The wrapping order is deliberately set in `main.registerModules`:

```go
orch.Register(testMw(loadMw(m)))
```

Unrolled:

- `m.Run` executes the chaos action.
- `loadMw` fires the HTTP burst **in parallel** with `m.Run`.
- `testMw` schedules the evaluation **after** the wrapped `Run` returns.

Net effect: **load during the action, verification after the whole
thing**. Don't reorder without thinking it through.

## Lifecycle

Both middlewares are attached to process-wide supervisors built in `main`:

- `loadkit.Supervisor` tracks in-flight bursts so they're drained at
  shutdown.
- `testkit.Runner` tracks pending `time.AfterFunc` timers so deferred
  tests are canceled cleanly.

Consequence: when you `SIGTERM` the agent, in-flight load bursts stop
promptly (context canceled), and scheduled tests either run to completion
or are canceled — never orphaned.

## Adding a new middleware

See [Adding a module](../development/adding-a-module.md#adding-a-middleware).
