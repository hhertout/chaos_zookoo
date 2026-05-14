---
title: NodeDrain
sidebar_position: 5
---

# NodeDrain

Simulates a rolling cluster upgrade by sequentially draining all pods in a
namespace from each targeted node — one node at a time, waiting for the
cluster to recover before moving to the next.

Unlike [GorillaKill](./gorillakill.md), which targets specific workloads,
`NodeDrain` is node-centric: for every matched node it kills **every** pod
in the namespace scheduled on that node. Specific pods or workloads can be
shielded via a `guard` block.

## When to use it

- You want to simulate a **rolling node restart** (e.g. a Kubernetes version
  upgrade) and validate that your workloads survive the sequential disruption.
- You need **deterministic, reproducible** drain order — nodes are always
  processed alphabetically by name.
- You want to verify that **recovery polling** works: the module waits for a
  minimum number of Running pods before moving to the next node, mirroring
  what a real rolling upgrade orchestrator would do.
- You need to **protect critical pods** (monitoring agents, infra services)
  from being killed while still draining everything else.

For targeting a specific workload across the whole cluster, see
[GorillaKill](./gorillakill.md). For a graceful rollout-restart, see
[Rollout](./rollout.md).

## Minimal config

Run once, drain every schedulable node in the namespace:

```yaml
kind: NodeDrain
name: simulate-upgrade
metadata:
  namespace: default
scenario:
  when: once
```

Restrict to worker nodes only:

```yaml
kind: NodeDrain
name: simulate-upgrade
metadata:
  namespace: production
scenario:
  when: once
  specs:
    nodeSelector:
      kubernetes.io/role: worker
```

On a cron schedule (e.g. weekly upgrade drill):

```yaml
kind: NodeDrain
name: weekly-drain-drill
metadata:
  namespace: staging
scenario:
  when: periodic
  cron: "0 3 * * 1"    # every Monday at 03:00
  specs:
    nodeSelector:
      kubernetes.io/role: worker
    minReady: 3
```

## Full reference

```yaml
kind: NodeDrain
name: simulate-upgrade                  # required — log correlation id
metadata:
  namespace: production                 # required — target namespace

scenario:
  when: once                            # required — "once" | "periodic"
  interval: 6h                          # required when when=periodic and no cron
  # cron: "0 3 * * 1"                  # alternative to interval — standard 5-field cron
  wait: 30s                             # optional — delay before first/only run (ignored with cron)
  dryRun: false                         # optional — log only, no API mutation

  specs:
    strategy: evict                     # optional — "evict" (default) | "delete"
    readinessTimeout: 5m                # optional — max wait per node before moving on (default: 5m)
    minReady: 2                         # optional — Running pod threshold for recovery (default: 1)
    nodeSelector:                       # optional — restrict which nodes are targeted
      kubernetes.io/role: worker

  guard:                                # optional — pods/workloads to never kill
    matchers:
      labels:
        app: critical-infra
      # podName: my-fixed-pod
      # deploymentName: monitoring-agent
      # daemonsetName: my-ds
      # statefulsetName: my-sts
```

### Field details

| Field                        | Type     | Default | Notes                                                                                                    |
| ---------------------------- | -------- | ------- | -------------------------------------------------------------------------------------------------------- |
| `scenario.when`              | enum     | —       | Required. `once` runs a single drain pass, then the module exits. `periodic` repeats on `interval`/`cron`. |
| `scenario.interval`          | duration | —       | Required when `when: periodic` (unless `cron` is set). Mutually exclusive with `cron`.                   |
| `scenario.cron`              | string   | —       | Standard 5-field cron expression. Only valid with `when: periodic`. Mutually exclusive with `interval`.  |
| `scenario.wait`              | duration | `0`     | Delay before the first (or only) run. For `periodic`, must be `< interval`. Ignored with `cron`.         |
| `scenario.dryRun`            | bool     | `false` | When true, produces the same log output as a real run without calling any mutating API.                  |
| `specs.strategy`             | enum     | `evict` | `evict` uses the Eviction API (PDB-aware). `delete` is a hard delete, bypasses PDBs.                    |
| `specs.readinessTimeout`     | duration | `5m`    | Max time to wait for recovery on a node. A timeout is a warning, not a fatal error — the drain continues. |
| `specs.minReady`             | integer  | `0`     | Minimum number of Running pods required before moving to the next node. `0` skips recovery waiting entirely. |
| `specs.nodeSelector`         | map      | —       | Label selector to restrict which nodes are drained. Absent = all schedulable nodes.                     |
| `guard.matchers`             | object   | —       | Pods matching any guard selector are never killed. Recomputed per node. See [Matchers](../concepts/matchers.md). |

## Behavior

1. **List target nodes.** Apply `specs.nodeSelector` (or include all nodes
   if absent). Nodes marked `Unschedulable` (cordoned) are always excluded.
   Results are sorted alphabetically by node name for deterministic ordering.

2. **For each node (sequential):**
   a. List all pods in the namespace scheduled on this node.
   b. Resolve guard exclusions via `guard.matchers`. The exclusion set is
      recomputed per node — pods may move between nodes in periodic mode.
   c. Compute `targets = pods on node − guard set`.
   d. If no targets remain, log a warning and skip to the next node.
   e. Log `"draining node <node>: <N> pods"` (also in `dryRun` mode).
   f. If not `dryRun`, remove each target pod and increment
      `chaos_pods_affected_total`.
   g. Poll until `>= specs.minReady` Running pods exist in the namespace
      **or** `specs.readinessTimeout` is exceeded. A timeout emits a warning
      and moves on — it does not abort the entire simulation.

3. Log `"node drain simulation complete"` with a node count.

### dryRun semantics

`dryRun: true` produces **identical log output** to a real run (same
"pod killed" and "draining node" entries at the same log levels), minus the
mutating API call and the `chaos_pods_affected_total` increment. Use it to
validate selectors and guard rules against a real cluster before arming the
scenario.

## RBAC

| Strategy  | Required verbs                                                         |
| --------- | ---------------------------------------------------------------------- |
| `evict`   | `nodes` → `list`; `pods` → `list`; `pods/eviction` → `create`         |
| `delete`  | `nodes` → `list`; `pods` → `list`, `delete`                           |

Recovery polling also requires `pods` → `list`.

If `guard.matchers` uses workload selectors (`deploymentName`,
`daemonsetName`, `statefulsetName`), add `get`/`list` on `deployments`,
`daemonsets`, `statefulsets` in the `apps` group.

See [RBAC reference](../deployment/rbac.md) for a ready-to-apply `Role`.
