---
title: RBAC requirements
sidebar_position: 2
---

# RBAC requirements

`chaos_zookoo` is **RBAC-native**: it holds no privileges beyond what you
grant its `ServiceAccount`. This page is the reference you hand to the
platform team when they ask "what do you need me to allow?".

## Principle

- **Scope to a `Role`, not a `ClusterRole`**, unless you explicitly want
  the agent to act cluster-wide. Bind the `Role` in the target namespace
  only.
- **Grant the union of verbs** used by your enabled modules and matchers
  — no more.
- **Use a dedicated `ServiceAccount`** for the agent. Don't piggyback on
  `default`.

## Matrix by module

| Module         | Resource                | Verbs                       |
| -------------- | ----------------------- | --------------------------- |
| `Killing`      | `pods`                  | `get`, `list`               |
| `Killing` (evict)  | `pods/eviction`     | `create`                    |
| `Killing` (delete) | `pods`              | `delete`                    |
| `GorillaKill`  | `pods`                  | `get`, `list`, `delete`     |
| `Rollout`      | `deployments` / `daemonsets` / `statefulsets` | `patch` |

## Matrix by matcher

| Matcher field        | Resource          | Verbs        |
| -------------------- | ----------------- | ------------ |
| `labels`             | `pods`            | `list`       |
| `podName`            | `pods`            | `get`        |
| `deploymentName`     | `deployments`     | `get`        |
| `daemonsetName`      | `daemonsets`      | `get`        |
| `statefulsetName`    | `statefulsets`    | `get`        |

Combine the two matrices — if your scenario uses `Killing` with
`strategy: evict` + `labels:` + `deploymentName:`, you need:

- `pods` → `get`, `list`
- `pods/eviction` → `create`
- `deployments` (apps) → `get`

## Full-featured example

A `Role` covering all built-in modules, all matchers, and both eviction
and hard-delete strategies:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: chaos-zookoo
  namespace: chaos-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: chaos-zookoo
  namespace: my-target-namespace
rules:
  # Pod targeting + Killing + GorillaKill
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "delete"]

  # Killing strategy: evict
  - apiGroups: [""]
    resources: ["pods/eviction"]
    verbs: ["create"]

  # Workload matchers + Rollout
  - apiGroups: ["apps"]
    resources: ["deployments", "daemonsets", "statefulsets"]
    verbs: ["get", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: chaos-zookoo
  namespace: my-target-namespace
subjects:
  - kind: ServiceAccount
    name: chaos-zookoo
    namespace: chaos-system
roleRef:
  kind: Role
  name: chaos-zookoo
  apiGroup: rbac.authorization.k8s.io
```

Repeat the `Role` + `RoleBinding` per target namespace. Or, if you
explicitly want the agent to act cluster-wide, swap them for a
`ClusterRole` + `ClusterRoleBinding` — at your own review.

## Minimal example

The *smallest* possible `Role` — `Killing` with `strategy: evict`,
targeting only pods by label, in one namespace:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: chaos-zookoo-min
  namespace: my-target-namespace
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["list"]
  - apiGroups: [""]
    resources: ["pods/eviction"]
    verbs: ["create"]
```

## What the agent will never need

This is the explicit list of privileges `chaos_zookoo` does **not**
require — handy during security reviews:

- No `nodes` access.
- No `secrets` access.
- No `serviceaccounts/token` creation.
- No `MutatingWebhookConfiguration` / admission webhook registration.
- No `*` verbs. Ever.
- No `/exec`, `/attach`, `/portforward` on pods.
- No privileged pod securityContext, host namespaces, or volume
  mounts — the chart runs a plain container.
