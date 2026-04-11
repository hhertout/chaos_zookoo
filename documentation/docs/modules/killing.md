---
title: Killing
sidebar_position: 2
---

# Killing

The steady-state "pet killer". Every tick the module collects pods matching
the configured selectors, picks **one** at random, and removes it — respecting
a `minAvailable` floor so it never drains a workload below a safety threshold.

## When to use it

- You want to continuously validate that your workload recovers from a
  random pod loss.
- You care about graceful disruption — the default `strategy: evict` goes
  through the Eviction API and **respects `PodDisruptionBudget`**.
- You need a safety floor. `minAvailable` prevents over-killing.

For mass kills, see [GorillaKill](./gorillakill.md). For graceful
rollout-restart-style churn, see [Rollout](./rollout.md).

## Minimal config

```yaml
kind: Killing
name: kill-my-app
metadata:
  namespace: default
scenario:
  interval: 60s
  minAvailable: 1
  matchers:
    labels:
      app: my-app
```

## Full reference

```yaml
kind: Killing
name: kill-my-app                       # required — log correlation id
metadata:
  namespace: default                    # required — target namespace
scenario:
  interval: 60s                         # required — Go duration (mutually exclusive with cron)
  # cron: "*/5 * * * *"               # alternative to interval — standard 5-field cron
  wait: 10s                             # optional — first-tick delay, < interval (ignored with cron)
  minAvailable: 1                       # required — won't kill below this floor
  dryRun: false                         # optional — log only, no API mutation
  strategy: evict                       # optional — "evict" (default) | "delete"
  matchers:                             # required — at least one selector
    labels:
      app: my-app
    # podName: my-pod-abc123
    # deploymentName: my-deployment
    # daemonsetName: my-daemonset
    # statefulsetName: my-statefulset
```

### Field details

| Field                      | Type       | Default  | Notes                                                                            |
| -------------------------- | ---------- | -------- | -------------------------------------------------------------------------------- |
| `scenario.interval`        | duration   | —        | Cadence between ticks. Mutually exclusive with `cron`. One is required.          |
| `scenario.cron`            | string     | —        | Standard 5-field cron expression. Mutually exclusive with `interval`.            |
| `scenario.wait`            | duration   | `0`      | Delay before the first tick. Must be `< interval`. Ignored when `cron` is set.  |
| `scenario.minAvailable`    | integer    | `0`      | Skip the tick if `len(pods) - minAvailable <= 0`.                                |
| `scenario.dryRun`          | bool       | `false`  | When true, logs the intended victim without calling the API.                     |
| `scenario.strategy`        | enum       | `evict`  | `evict` uses the Eviction API (PDB-aware). `delete` is a hard delete.            |
| `scenario.matchers`        | object     | —        | See [Matchers](../concepts/matchers.md). Must be non-empty.                      |

## Behavior

1. On each tick, collect the union of pods matching every configured
   selector.
2. If no pods match, emit a `killing skipped: no_match` warning and exit.
3. Compute `killable = len(pods) - minAvailable`. If `<= 0`, emit a
   `killing skipped: min_available_floor` warning and exit.
4. Pick a uniform random victim from the pool.
5. Log the intended action (always, including in dry-run).
6. If `dryRun: false`, call the API:
   - `strategy: evict` → `CoreV1().Pods(ns).EvictV1(...)`
   - `strategy: delete` → `CoreV1().Pods(ns).Delete(...)`

Failure modes (e.g. the API call returns an error) are logged but **do not
stop** the module — the next tick runs as scheduled.

## RBAC

| Strategy  | Required verbs                                        |
| --------- | ----------------------------------------------------- |
| `evict`   | `pods` → `list`, `get`; `pods/eviction` → `create`    |
| `delete`  | `pods` → `list`, `get`, `delete`                      |

Plus whatever read access matchers need (see
[Matchers RBAC](../concepts/matchers.md#rbac)).

See [RBAC reference](../deployment/rbac.md) for a ready-to-apply `Role`.
