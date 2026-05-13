# NodeDrain Module — Design Spec

**Date:** 2026-05-13  
**Kind:** `NodeDrain`  
**Package:** `pkg/nodedrain`  
**Status:** Approved

---

## Overview

`NodeDrain` simulates a rolling cluster upgrade by sequentially draining all pods in a namespace from each node, one node at a time. Unlike `GorillaKill` (which targets specific workloads), `NodeDrain` is node-centric: for each node in scope, it kills every pod in the namespace that is scheduled on that node, then waits for the cluster to recover before moving to the next node. Specific pods or workloads can be protected via a `guard` block.

The security model is unchanged: all operations go through the Kubernetes API as regular calls (`EvictV1` or `Pods.Delete`). The cluster RBAC is the only privilege boundary.

---

## YAML API

```yaml
kind: NodeDrain
name: simulate-upgrade

metadata:
  namespace: production

scenario:
  # Execution mode: once | periodic
  when: once

  # Required when when=periodic. Mutually exclusive with cron.
  # interval: 6h
  # cron: "0 3 * * 1"

  # Delay before the first execution. Ignored with cron.
  # For when=periodic, must be strictly less than interval.
  # wait: 30s

  # If true, log what would be killed without touching the API.
  # Output is identical to a real run minus the mutating call.
  dryRun: false

  specs:
    # How pods are removed:
    #   evict  (default): uses the Eviction API, respects PodDisruptionBudgets.
    #   delete: hard delete, bypasses PDBs.
    strategy: evict

    # Label selector to restrict which nodes are drained.
    # If absent, all schedulable nodes in the cluster are targeted
    # (nodes tainted node.kubernetes.io/unschedulable are excluded).
    nodeSelector:
      kubernetes.io/role: worker

    # Maximum time to wait for namespace pods to recover before
    # moving to the next node. A timeout is a warning, not a fatal error:
    # the module continues with the next node. Default: 5m.
    readinessTimeout: 5m

    # Minimum number of Running pods in the namespace required to
    # consider the node absorbed. Default: 1.
    minReady: 2

  # Pods and workloads to never kill, regardless of which node they are on.
  # Uses the same matchers.Matchers shape as other modules.
  # Optional — omit entirely if no exclusions are needed.
  guard:
    matchers:
      labels:
        app: critical-infra
      deploymentName: monitoring-agent
      # podName: my-fixed-pod
      # daemonsetName: my-ds
      # statefulsetName: my-sts
```

### Validation rules

| Field | Rule |
|---|---|
| `name` | Required, non-empty |
| `metadata.namespace` | Required |
| `scenario.when` | Required: `once` or `periodic` |
| `scenario.interval` / `scenario.cron` | Required when `when=periodic`, mutually exclusive |
| `scenario.wait` | Optional; for `when=periodic` must be < `interval`; unbounded for `when=once`; forbidden with `cron` |
| `specs.strategy` | `evict` or `delete`; default `evict` |
| `specs.readinessTimeout` | Valid Go duration, > 0; default `5m` |
| `specs.minReady` | >= 0; default `1` |
| `specs.nodeSelector` | Optional map; absent = all schedulable nodes |
| `guard.matchers` | Optional; parsed via `matchers.Matchers` |

---

## Module behaviour — `Run()`

```
Run(ctx):
  1. List nodes matching specs.nodeSelector
     (or all nodes without taint node.kubernetes.io/unschedulable if absent)
     Sort nodes alphabetically by name for deterministic ordering.

  2. For each node (sequential):
     a. List all pods in namespace scheduled on this node
        (field selector: spec.nodeName=<node>)
     b. Resolve guard pods via matchers.CollectPods(guard.matchers)
        Build a name-keyed exclusion set.
     c. targets = pods on node − exclusion set
     d. If targets is empty:
          log.Warn "no targets on node <node>, skipping"
          continue to next node
     e. Log "draining node <node>: <N> pods" (identical in dryRun)
     f. If not dryRun:
          For each target pod: call remover.Remove(ctx, namespace, pod.Name)
          Increment chaos_pods_affected_total per killed pod
     g. Poll until namespace has >= specs.minReady Running pods
        OR specs.readinessTimeout exceeded:
          if timeout: log.Warn "readiness timeout on node <node>, continuing"
          else:        log.Info "node <node> recovered"

  3. Log "node drain simulation complete"
```

**Key invariants:**
- Node order is alphabetically sorted by `node.Name` — reproducible across runs.
- A `readinessTimeout` breach emits a warning and moves on; it does not abort the simulation. This mirrors real upgrade behaviour where the orchestrator continues to the next node after a grace period.
- `dryRun: true` produces the exact same log output minus the mutating API call and the metrics increment.
- The guard exclusion set is recomputed per node (pods may move between nodes between ticks in periodic mode).
- Polling uses a ticker (e.g. every 5s) against `Pods.List` with `status.phase=Running` field selector in the namespace.

---

## Struct layout

### `pkg/nodedrain/config.go`

```go
type Strategy string

const (
    StrategyEvict  Strategy = "evict"
    StrategyDelete Strategy = "delete"
)

type When string

const (
    WhenOnce     When = "once"
    WhenPeriodic When = "periodic"
)

type Config struct {
    Kind     string          `yaml:"kind"`
    Name     string          `yaml:"name"`
    Metadata module.Metadata `yaml:"metadata"`
    Scenario Scenario        `yaml:"scenario"`

    interval time.Duration
    wait     time.Duration
    cronExpr string
}

type Scenario struct {
    When        When   `yaml:"when"`
    RawInterval string `yaml:"interval"`
    RawCron     string `yaml:"cron"`
    RawWait     string `yaml:"wait"`
    DryRun      bool   `yaml:"dryRun"`
    Specs       Specs  `yaml:"specs"`
    Guard       Guard  `yaml:"guard"`
}

type Specs struct {
    Strategy            Strategy          `yaml:"strategy"`
    NodeSelector        map[string]string `yaml:"nodeSelector"`
    RawReadinessTimeout string            `yaml:"readinessTimeout"`
    MinReady            int               `yaml:"minReady"`

    readinessTimeout time.Duration
}

func (s Specs) ReadinessTimeout() time.Duration { return s.readinessTimeout }

type Guard struct {
    Matchers matchers.Matchers `yaml:"matchers"`
}
```

### `pkg/nodedrain/module.go`

- `Module` struct holds `client`, `name`, `namespace`, `specs`, `guard`, `dryRun`, scheduling fields, `remover PodRemover`.
- `PodRemover` interface (same shape as in `killing`) defined locally — `evictRemover` and `deleteRemover`.
- Private `drainNode(ctx, nodeName string) error` encapsulates steps 2a–2g.
- Private `waitForRecovery(ctx, nodeName string) error` encapsulates the polling loop.

### `pkg/nodedrain/register.go`

```go
func Build(client kubernetes.Interface, data []byte) (module.ChaosModule, error)
```

### `pkg/nodedrain/module_test.go`

- Table-driven tests for `ParseConfig`: valid configs, each validation error case.
- `Run` tests using `fake.NewSimpleClientset`:
  - Happy path: two nodes, pods killed sequentially, recovery polling resolves.
  - DryRun: no API mutations, same logs.
  - Guard: guarded pods survive.
  - ReadinessTimeout exceeded: warn logged, simulation continues to next node.
  - No nodes matched: graceful no-op.

---

## Registration

In `cmd/chaos_zookoo/main.go`, add to the `builders` map:

```go
"NodeDrain": nodedrain.Build,
```

---

## Example file

`examples/nodedrain.yaml` — fully annotated, same commentary density as existing examples.

---

## RBAC requirements

| Operation | API group | Resource | Verb |
|---|---|---|---|
| List nodes | `""` | `nodes` | `list` |
| List pods by node | `""` | `pods` | `list` |
| Evict pods | `policy` | `pods/eviction` | `create` |
| Delete pods | `""` | `pods` | `delete` |
| List pods (recovery poll) | `""` | `pods` | `list` |

Guard resolution also requires read access to `deployments`, `daemonsets`, `statefulsets` in `apps` if those matchers are used.

---

## Middleware compatibility

`testing:` and `load:` middleware blocks apply without any code changes in the module. The wrap order in `main.go` remains `orch.Register(testMw(loadMw(m)))`.

---

## Out of scope

- Node cordoning / uncordoning (would require node `patch` verb — deliberately excluded).
- Waiting for pods to be rescheduled on specific nodes (we only check Running count in namespace).
- Parallel node draining (`parallelism: N`) — not needed for the sequential simulation use case.
