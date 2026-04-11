---
title: Matchers
sidebar_position: 1
---

# Matchers

Matchers are how every module describes **what** to act on. The shape is
shared across `Killing`, `GorillaKill`, and `Rollout` — learn it once, use
it everywhere.

## The five selectors

```yaml
scenario:
  matchers:
    labels:                 # 1) label selector (AND'd)
      app: my-app
      tier: api
    podName: my-pod-abc     # 2) exact pod name
    deploymentName: my-dep  # 3) deployment → pods via .spec.selector
    daemonsetName: my-ds    # 4) daemonset  → pods via .spec.selector
    statefulsetName: my-ss  # 5) statefulset→ pods via .spec.selector
```

| Field              | Targets                | Resolution                                                             |
| ------------------ | ---------------------- | ---------------------------------------------------------------------- |
| `labels`           | Pods                   | `LIST pods` with label selector (AND over the map).                    |
| `podName`          | A single pod           | `GET pod/<name>`.                                                      |
| `deploymentName`   | A deployment's pods    | `GET deployment/<name>`, then `LIST pods` with its `.spec.selector`.   |
| `daemonsetName`    | A daemonset's pods     | `GET daemonset/<name>`, then `LIST pods` with its `.spec.selector`.    |
| `statefulsetName`  | A statefulset's pods   | `GET statefulset/<name>`, then `LIST pods` with its `.spec.selector`.  |

## Combining selectors

Multiple selectors are **unioned** — the module acts on pods matching
*any* of them:

```yaml
matchers:
  labels: {app: api}
  deploymentName: worker    # pods of the worker deployment are also targeted
```

Duplicates are de-duplicated by pod `.metadata.name` within the namespace.
If you set every selector, you get the union of all their results.

## Scope

- All matchers are evaluated inside `metadata.namespace`. There is no
  cluster-scoped targeting.
- Pods that are already `Terminating` are still returned by the API and
  therefore still counted — that's the cluster's behavior, not the
  agent's.

## Module-specific rules

| Module         | Accepted matchers                                                      |
| -------------- | ---------------------------------------------------------------------- |
| `Killing`      | Any combination. At least one field required.                          |
| `GorillaKill`  | Any combination. At least one field required.                          |
| `Rollout`      | **Workload matchers only** (`deploymentName`, `daemonsetName`, `statefulsetName`). Pod-level matchers are ignored. At least one workload required. |

## RBAC

Each matcher maps to read verbs you need in the target namespace:

| Matcher field        | Required verbs                                    |
| -------------------- | ------------------------------------------------- |
| `labels`             | `pods` → `list`                                   |
| `podName`            | `pods` → `get`                                    |
| `deploymentName`     | `deployments` → `get`; `pods` → `list`            |
| `daemonsetName`      | `daemonsets` → `get`; `pods` → `list`             |
| `statefulsetName`    | `statefulsets` → `get`; `pods` → `list`           |

Combine with the action verbs documented on each module page. A complete
least-privilege `Role` is in [RBAC reference](../deployment/rbac.md).

## Anti-patterns

- **Matching nothing** is not an error. The module emits a
  `no_match` warning and no-ops the tick. That's intentional — the
  workload may simply be scaled to zero at the time of the tick.
- **Matching too much** is on you. There is no global kill-switch for "if
  this matches more than N pods, abort". Use `minAvailable` (on `Killing`)
  or narrow your selectors.
- **Stale caches.** There is no informer cache; every tick hits the API.
  Fine for minute-scale cadences, deliberate for freshness.
