---
title: Introduction
sidebar_position: 1
---

# chaos_zookoo

`chaos_zookoo` is a Kubernetes chaos-engineering agent in the spirit of
[Chaos Mesh](https://chaos-mesh.org/), but deliberately scoped down to the
**Kubernetes API server**.

Every disruption is expressed as a regular API call through `client-go`
(`EvictV1`, `Pods.Delete`, `Deployments.Patch`, …). There is **no**
privileged node-level component, no DaemonSet poking at `/proc`, no sidecar
injection — the agent is a single long-running process authenticated as a
`ServiceAccount`.

## Why not Chaos Mesh?

Chaos Mesh is powerful but needs privileged workloads on every node to do its
kernel-level tricks (network partitions, IO faults, kernel panics). That's
the right tool if you need that level of control — and the wrong tool if your
platform team isn't ready to grant cluster-admin to a chaos framework.

`chaos_zookoo` trades that reach for **RBAC-nativeness**:

- **The cluster's RBAC is the security model.** Whatever the
  `ServiceAccount` can do, the agent can do. Nothing more.
- **No privileged pods. No host namespaces. No webhooks.** Just a pod
  talking to `kube-apiserver`.
- **Auditability.** Every action is a standard API call visible in audit
  logs the same way any other client is.
- **Small blast radius.** A compromised agent is bounded by its `Role` /
  `ClusterRole`.

If what you need is "randomly evict pods", "periodically restart a
deployment", or "nuke every pod matching a label on day one" — you're in the
right place.

## What does it do today?

Three built-in modules:

- **[Killing](./modules/killing.md)** — random single-pod kill per tick,
  respects a `minAvailable` floor, supports `evict` (PDB-aware) or `delete`.
- **[GorillaKill](./modules/gorillakill.md)** — mass kill of every matching
  pod, either once at startup or on an interval.
- **[Rollout](./modules/rollout.md)** — patches a workload's pod template
  with a restart annotation, equivalent to `kubectl rollout restart` on a
  schedule.

Two orthogonal middlewares that can wrap any module:

- **[loadkit](./middlewares/loadkit.md)** — fires a synthetic HTTP burst
  **in parallel** with the chaos action to observe behavior under load.
- **[testkit](./middlewares/testkit.md)** — **after** the action, queries
  Grafana/Prometheus and exposes a pass/fail gauge for your dashboards.

## How it works, in one diagram

```text
   YAML file(s)
        │
        ▼
┌───────────────────┐
│ internal/config   │  splits documents, extracts `kind:`, parses cross-cutting
└───────────────────┘
        │
        ▼
┌───────────────────┐     per-kind   ┌────────────────┐
│ builders[kind]    ├───────────────▶│ ChaosModule    │
└───────────────────┘                └────────────────┘
        │                                      │
        ▼                                      │ wrapped by
┌───────────────────┐                          ▼
│ testkit.Middleware│                 ┌────────────────┐
│ loadkit.Middleware│                 │ Orchestrator   │───▶ goroutine per module
└───────────────────┘                 └────────────────┘
                                              │
                                              ▼
                                      kube-apiserver
```

## Next steps

- [Install chaos_zookoo](./getting-started/installation.md) locally or via Helm.
- [Write your first config](./getting-started/configuration.md).
- Understand [how modules are scheduled](./concepts/scheduling.md).
