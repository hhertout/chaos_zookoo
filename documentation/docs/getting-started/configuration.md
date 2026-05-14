---
title: Configuration
sidebar_position: 2
---

# Configuration

`chaos_zookoo` is configured entirely via YAML. The agent reads either:

- a **directory** of `.yaml` / `.yml` files, or
- a **single file** (which may hold multiple `---`-separated documents).

Each YAML document declares a `kind:` that maps to a module type. You can
mix as many documents and kinds as you want — they all run side-by-side
with their own schedules.

## Resolution order

The config source is resolved in this order:

1. `--config` (or `-C`) CLI flag
2. `CHAOS_CONFIG_DIR` environment variable
3. `./configs` (default, relative to the working directory)

## Anatomy of a config document

Every document shares the same outer shape:

```yaml
kind: Killing                # required — the module kind
metadata:
  name: kill-my-app          # required — human-readable identifier
  namespace: default         # required — namespace the module acts on
scenario:
  # ... module-specific fields: interval, matchers, dryRun, strategy, ...

# Optional cross-cutting middlewares:
testing:
  client: grafana
  specs:
    - datasourceId: prom-uid
      query: sum(rate(http_requests_total[5m]))
      operator: sup
      threshold: 10

load:
  vus: 5
  duration: 30s
  requests:
    method: GET
    url: https://my-app.example.com/healthz
```

The top-level keys split into three groups:

| Group         | Keys                         | Owner                          |
| ------------- | ---------------------------- | ------------------------------ |
| Identity      | `kind`, `name`, `metadata`   | Shared across all modules.     |
| Scenario      | `scenario`                   | The module (kind-specific).    |
| Cross-cutting | `testing`, `load`            | Middlewares (kind-independent). |

Cross-cutting keys are parsed independently from the module. A module
author never touches `testing:` / `load:` — adding a new middleware means
adding a new top-level key and a new wrapper, not modifying existing
modules.

## Multi-document files

A single file can hold multiple scenarios:

```yaml
kind: Killing
metadata: {name: kill-api, namespace: api}
scenario:
  interval: 60s
  minAvailable: 2
  matchers:
    labels: {app: api}

---

kind: Rollout
metadata: {name: restart-worker, namespace: workers}
scenario:
  interval: 1h
  matchers:
    deploymentName: worker
```

## YAML validation

Validation happens at load time — the process **fails fast** with a clear
error rather than starting in a degraded state.

Typical failure modes:

- missing required field (`name`, `metadata.namespace`, `scenario.matchers`, …)
- unknown `kind:`
- invalid duration string (`scenario.interval: "5minutes"` → must be Go
  duration notation, e.g. `"5m"`)
- `scenario.wait >= scenario.interval` for periodic modules
- cross-cutting spec incompatible with the scenario cadence (e.g. a
  `load.duration` longer than `scenario.interval`)

## Examples

The repository ships fully annotated samples for each module kind:

- [`examples/killing.yaml`](https://github.com/hhertout/chaos_zookoo/blob/main/examples/killing.yaml)
- [`examples/gorillakill.yaml`](https://github.com/hhertout/chaos_zookoo/blob/main/examples/gorillakill.yaml)
- [`examples/rollout.yaml`](https://github.com/hhertout/chaos_zookoo/blob/main/examples/rollout.yaml)

Point `--config` at the `examples/` directory to see all three run together.
