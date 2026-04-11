---
title: Overview
sidebar_position: 1
---

# Modules overview

A **module** is the unit of chaos: a typed scenario, a schedule, and a
`Run(ctx)` method that performs the disruption. Modules are isolated Go
packages under `pkg/<kind>/` — each one independently parses its YAML,
holds its own state, and knows nothing about cross-cutting concerns like
load generation or post-run checks.

## Built-in modules

| Kind            | Does                                                | Schedule                 | Page                                 |
| --------------- | --------------------------------------------------- | ------------------------ | ------------------------------------ |
| `Killing`       | Picks **one random** matching pod and removes it.   | Periodic                 | [Killing](./killing.md)              |
| `GorillaKill`   | Removes **every** matching pod in a single pass.    | Periodic **or** once     | [GorillaKill](./gorillakill.md)      |
| `Rollout`       | Patches a workload's pod template with a restart annotation. | Periodic         | [Rollout](./rollout.md)              |

## The module contract

Every module implements `pkg/module.ChaosModule`:

```go
type ChaosModule interface {
    Name() string
    Run(ctx context.Context) error
    Schedule() Schedule
}
```

- **`Name`** must return the user-provided `name:` field. It's used for
  log correlation and in Prometheus label values.
- **`Schedule`** returns the cadence — the orchestrator doesn't decide it,
  the module does. See [Scheduling](../concepts/scheduling.md).
- **`Run`** executes one tick. It receives a context that is canceled at
  shutdown and must propagate that cancellation to every API call it
  makes.

## Shared scenario conventions

All modules follow the same conventions — if you've configured one, you
know most of the next:

- **`metadata.namespace`** is mandatory. Modules never act cluster-wide.
- **Matchers** (labels, podName, deploymentName, …) come from a shared
  type, documented in [Matchers](../concepts/matchers.md).
- **`scenario.dryRun: true`** produces the same logs as a real run minus
  the mutating API call. Great for validating selectors against a real
  cluster before arming a scenario.
- **`scenario.interval`** and **`scenario.wait`** are Go duration strings
  (`"30s"`, `"5m"`, `"1h"`). `wait` must be strictly less than `interval`
  for periodic modules.
- **Safety floors** (e.g. `minAvailable`) are enforced inside `Run`
  **before** any API mutation. A breach is a logged no-op, not an error.

## Composition

Every module can be wrapped with cross-cutting middlewares. Declaring a
`testing:` or `load:` block on any scenario is enough to opt into them —
no per-module flag, no re-building, no rewriting the scenario.

See [Middlewares overview](../middlewares/overview.md).
