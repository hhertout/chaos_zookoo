---
title: Scheduling
sidebar_position: 2
---

# Scheduling

The orchestrator schedules modules — it doesn't decide *when* a module
runs. Each module returns a `Schedule` struct that the orchestrator
obeys.

```go
type Schedule struct {
    Mode         ScheduleMode   // ScheduleOnce | SchedulePeriodic | ScheduleCron
    Interval     time.Duration
    InitialDelay time.Duration
    CronExpr     string         // ScheduleCron only
}
```

## The three modes

### `SchedulePeriodic`

The default for `Killing` and `Rollout`, and an opt-in for
`GorillaKill` via `scenario.when: periodic`.

1. If `InitialDelay > 0`: the module waits, then ticks once immediately.
2. A `time.Ticker(Interval)` then fires every `Interval` until
   cancellation.

Consequence: with `wait: 10s` and `interval: 60s`, the first tick is at
`t=10s`, then `t=70s`, `t=130s`, …

### `ScheduleOnce`

Used by `GorillaKill` with `scenario.when: once`.

1. The module waits `InitialDelay` (may be zero).
2. One tick runs.
3. The goroutine exits. The orchestrator reaps it as done.

There is no "retry on failure" for one-shot modules — if the single tick
errors, the scenario is considered finished.

### `ScheduleCron`

Available on all periodic modules (`Killing`, `Rollout`, `GorillaKill`
with `when: periodic`) via `scenario.cron`.

The expression follows the standard 5-field cron syntax (minute, hour,
day-of-month, month, day-of-week):

```yaml
scenario:
  cron: "0 2 * * *"   # every day at 02:00
```

At each tick, the orchestrator computes the next scheduled time using
[`gronx`](https://github.com/adhocore/gronx) and sleeps until it arrives.
This is more precise than `SchedulePeriodic` for wall-clock schedules
like "every night at 2am".

`cron` and `interval` are **mutually exclusive** — validation rejects
both being set.

## `wait` vs `interval`

For periodic modules, YAML validation enforces:

```
0 <= wait < interval
```

This prevents configurations like "wait 5m, interval 1m" that would
behave non-obviously.

For `GorillaKill when=once`, `wait` is unbounded — "run the first
time in 24 hours" is a valid scenario.

`wait` has no effect when `cron` is set — the cron expression already
encodes the exact schedule.

## Concurrency model

- **One goroutine per module.** Independent modules run in parallel.
- **Ticks of the same module are serialized.** Two ticks never overlap —
  the orchestrator takes its mutex for the duration of `Run`, and the
  ticker channel is drained naturally.
- **A slow `Run` delays future ticks.** If a module's `Run` takes longer
  than `Interval`, the next tick fires immediately after, without
  queueing. This is standard `time.Ticker` behavior — the ticker channel
  has capacity 1.

Implication: keep `Run` fast. Use `testkit`'s `time.AfterFunc`-style
deferral for long waits. Don't block on long operations inside `Run`.

## Graceful shutdown

On `SIGINT` / `SIGTERM`:

1. The root context is canceled — every `Run(ctx)` sees its ctx done.
2. `orch.Stop()` closes a `stopCh` and waits for every module loop to
   return.
3. Cross-cutting supervisors (`loadkit.Supervisor`, `testkit.Runner`)
   drain their in-flight work.
4. The metrics server shuts down.

Modules that ignore their context will block shutdown — always thread
`ctx` through API calls.

## Startup ordering

Modules start in the order they're declared in YAML (directory-loaded
configs are lexicographically sorted by filename). There is **no**
inter-module dependency graph — if `A` must run before `B`, model that
with `wait:`, not with a hidden ordering assumption.
