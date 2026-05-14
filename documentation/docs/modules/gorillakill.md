---
title: GorillaKill
sidebar_position: 3
---

# GorillaKill

The "gros bourrin" scenario: no survivors, no PDB negotiation, just a mass
`Delete` of every pod matching the configured selectors.

## When to use it

- You want to simulate a **full-zone outage** or a simultaneous loss of
  every replica of a workload.
- You want to validate cold-start recovery, leader re-election, or cache
  warm-up paths.
- You explicitly **do not** want PDB protection.

For a steady drip of single-pod kills with a safety floor, use
[Killing](./killing.md) instead.

## Minimal config

Run once at startup, kill everything, then idle:

```yaml
kind: GorillaKill
metadata:
  name: gorilla-my-app
  namespace: default
scenario:
  when: once
  matchers:
    labels:
      app: my-app
```

Or on an interval:

```yaml
kind: GorillaKill
metadata:
  name: periodic-zone-loss
  namespace: default
scenario:
  when: periodic
  interval: 10m
  matchers:
    labels:
      app: my-app
```

Or on a cron schedule:

```yaml
kind: GorillaKill
metadata:
  name: nightly-zone-loss
  namespace: default
scenario:
  when: periodic
  cron: "0 3 * * *"    # every night at 03:00
  matchers:
    labels:
      app: my-app
```

## Full reference

```yaml
kind: GorillaKill
metadata:
  name: gorilla-my-app                  # required
  namespace: default                    # required
scenario:
  when: once                            # required — "once" | "periodic"
  interval: 10m                         # required when when=periodic and no cron
  # cron: "0 3 * * *"                  # alternative to interval — standard 5-field cron
  wait: 30s                             # optional — delay before first/only run (ignored with cron)
  dryRun: false                         # optional — log only
  matchers:                             # required — at least one selector
    labels:
      app: my-app
```

### Field details

| Field                  | Type       | Default | Notes                                                                                              |
| ---------------------- | ---------- | ------- | -------------------------------------------------------------------------------------------------- |
| `scenario.when`        | enum       | —       | Required. `once` runs a single time then exits the module loop. `periodic` ticks on `interval` or `cron`. |
| `scenario.interval`    | duration   | —       | Required when `when: periodic` (unless `cron` is set). Mutually exclusive with `cron`.             |
| `scenario.cron`        | string     | —       | Standard 5-field cron expression. Only valid with `when: periodic`. Mutually exclusive with `interval`. |
| `scenario.wait`        | duration   | `0`     | Delay before the first (or only) run. For `periodic`, must be `< interval`. Ignored with `cron`.  |
| `scenario.dryRun`      | bool       | `false` | When true, logs each intended kill without calling the API.                                        |
| `scenario.matchers`    | object     | —       | See [Matchers](../concepts/matchers.md).                                                           |

## Behavior

1. Collect the union of pods matching every configured selector.
2. If no pods match, emit a `gorillakill skipped: no_match` warning and exit.
3. For each matched pod:
   - Log the intended kill (always).
   - If not `dryRun`, call `CoreV1().Pods(ns).Delete(...)` — **no
     eviction, no PDB**.
4. Aggregate per-pod errors using `errors.Join` and return them so the
   orchestrator logs a single run failure.

## Scheduling semantics

- `when: once` returns a `ScheduleOnce` to the orchestrator. The module
  loop runs exactly one tick, then the goroutine exits.
- `when: periodic` returns a `SchedulePeriodic`, same semantics as
  [Killing](./killing.md).

Combined with `wait`, this lets you express "five minutes after the agent
starts, kill every pod of service X" as a one-shot scenario.

## RBAC

| Verb                                      |
| ----------------------------------------- |
| `pods` → `list`, `get`, `delete`          |

Plus read access for the selectors you use
([Matchers RBAC](../concepts/matchers.md#rbac)).
