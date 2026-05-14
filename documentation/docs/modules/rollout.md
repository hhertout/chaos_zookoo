---
title: Rollout
sidebar_position: 4
---

# Rollout

Triggers a rollout restart on targeted workloads by patching the pod
template with a `chaos-zookoo/restartedAt` annotation. Equivalent to
`kubectl rollout restart` on a schedule.

## When to use it

- You want **controlled churn** — the controller manages the rollout
  (surge/unavailable, readiness gates, `minReadySeconds`), not you.
- You want to validate the deployment strategy itself, not just pod
  resilience.
- Cleaner than killing pods when you want smooth replacement instead of
  abrupt loss.

## Minimal config

```yaml
kind: Rollout
metadata:
  name: rollout-my-deployment
  namespace: default
scenario:
  interval: 1h
  matchers:
    deploymentName: my-deployment
```

## Full reference

```yaml
kind: Rollout
metadata:
  name: rollout-my-deployment           # required
  namespace: default                    # required
scenario:
  interval: 1h                          # required — Go duration (mutually exclusive with cron)
  # cron: "0 4 * * 1"                  # alternative — e.g. every Monday at 04:00
  wait: 5m                              # optional — first-tick delay, < interval (ignored with cron)
  dryRun: false                         # optional — log only
  matchers:                             # required — at least one workload target
    deploymentName: my-deployment
    # daemonsetName: my-daemonset
    # statefulsetName: my-statefulset
```

### Field details

| Field                     | Type       | Default | Notes                                                                 |
| ------------------------- | ---------- | ------- | --------------------------------------------------------------------- |
| `scenario.interval`       | duration   | —       | Cadence between restarts. Mutually exclusive with `cron`. One is required. |
| `scenario.cron`           | string     | —       | Standard 5-field cron expression. Mutually exclusive with `interval`. |
| `scenario.wait`           | duration   | `0`     | Delay before the first tick. Must be `< interval`. Ignored with `cron`. |
| `scenario.dryRun`         | bool       | `false` | When true, logs the intended patch without applying it.               |
| `scenario.matchers`       | object     | —       | At least one of `deploymentName`, `daemonsetName`, `statefulsetName`. |

:::caution
`Rollout` operates on **workload owners**, not individual pods. Pod-level
matchers (`labels`, `podName`) are ignored. If no workload matcher is set,
the scenario fails validation at load time.
:::

## Behavior

Each tick builds a strategic-merge patch equivalent to:

```json
{
  "spec": {
    "template": {
      "metadata": {
        "annotations": {
          "chaos-zookoo/restartedAt": "<RFC3339 timestamp>"
        }
      }
    }
  }
}
```

Then applies it to every workload listed in `matchers`:

| Matcher field        | API call                                                  |
| -------------------- | --------------------------------------------------------- |
| `deploymentName`     | `AppsV1().Deployments(ns).Patch(...)`                     |
| `daemonsetName`      | `AppsV1().DaemonSets(ns).Patch(...)`                      |
| `statefulsetName`    | `AppsV1().StatefulSets(ns).Patch(...)`                    |

The actual rollout is delegated to the controller — `chaos_zookoo` just
bumps the template. Surge / unavailable / readiness are honored because
that's what the controller does natively.

## RBAC

For each workload kind you target:

| Workload       | Required verbs                                  |
| -------------- | ----------------------------------------------- |
| `deployments`  | `patch` (and `get` if you also run `testkit`)   |
| `daemonsets`   | `patch`                                         |
| `statefulsets` | `patch`                                         |

See [RBAC reference](../deployment/rbac.md).
