---
title: Helm chart
sidebar_position: 1
---

# Helm deployment

The repository ships a Helm chart under `helm/`. It deploys a single
`Deployment` running `chaos_zookoo`, mounts the scenarios as a `ConfigMap`,
and optionally sidecars a Grafana Alloy exporter for OTLP-based metric shipping.

## Install

```bash
helm install chaos-zookoo ./helm \
  --namespace chaos-system --create-namespace \
  --values my-values.yaml
```

For the full installation walkthrough (credentials, testkit env vars,
`extraSecrets`), see **[Deploy with Helm](../getting-started/helm.md)**.

## Minimal `values.yaml`

```yaml
k8s:
  host: "https://api.my-cluster.example.com"
  clusterCert: "<base64-CA>"
  token: "<bearer-token>"

configs:
  - kind: Killing
    name: kill-api
    namespace: api
    schedule:
      interval: 60s
    selector:
      labels:
        app: api
    minAvailable: 2
```

Each entry under `configs` is a scenario object — the chart serialises it
and concatenates all documents with `---` into `/app/config.yaml`.

## Full `values.yaml` reference

```yaml
image:
  repository: neryolab/chaos-zookoo
  tag: "0.2.0"

# ServiceAccount: create=true lets the chart generate the SA.
# The chart never creates a Role or RoleBinding — add them separately.
serviceAccount:
  create: false
  name: ""  # existing SA name; defaults to "default" when empty

# Kubernetes API credentials (stored in a Secret by the chart).
k8s:
  host: ""
  clusterCert: ""
  token: ""

# Secrets created in the release namespace (e.g. for GRAFANA_TOKEN).
extraSecrets: []

# Extra env vars injected into the container.
# Required for testkit: GRAFANA_URL and GRAFANA_TOKEN.
extraEnv: []

extraContainers: []
extraVolumes: []
extraVolumeMounts: []

resources:
  limits:   {cpu: 100m, memory: 128Mi}
  requests: {cpu: 100m, memory: 128Mi}

readinessProbe: {}
livenessProbe:  {}

monitoring:
  prometheus:
    port: 9090           # maps to METRICS_ADDR, exposed on the Service
  exporter:
    enabled: false       # flip on to sidecar grafana/alloy for OTLP
    scrapeInterval: 30s
    image:
      repository: grafana/alloy
      tag: latest
      pullPolicy: IfNotPresent
    endpoint:
      url: http://otel-collector:4317
      insecure: false
    resources:
      limits:   {cpu: 300m, memory: 256Mi}
      requests: {cpu: 100m, memory: 128Mi}

configs: []
```

## What the chart creates

| Template                        | Kind         | Purpose                                                        |
| ------------------------------- | ------------ | -------------------------------------------------------------- |
| `templates/serviceaccount.yml`  | `ServiceAccount` | Created only when `serviceAccount.create: true`.           |
| `templates/secret.yml`          | `Secret`     | Holds `K8S_HOST`, `K8S_CLUSTER_CERT`, `K8S_TOKEN`.            |
| `templates/extra-secrets.yml`   | `Secret`     | One per `extraSecrets` entry — only when list is non-empty.    |
| `templates/configmap.yml`       | `ConfigMap`  | Concatenated scenarios mounted at `/app/config.yaml`.          |
| `templates/service.yml`         | `Service`    | Exposes the `/metrics` port for scraping.                      |
| `templates/deployment.yml`      | `Deployment` | Runs the agent, mounts the ConfigMap, injects credentials.     |
| `templates/alloy-configmap.yml` | `ConfigMap`  | Alloy sidecar config — only when `monitoring.exporter.enabled`. |

## ServiceAccount and RBAC

The chart never creates a `Role` or `RoleBinding`. Set `serviceAccount.create: true`
to generate the `ServiceAccount` object, then bind it manually to a `Role`
scoped to the namespaces you actually target.

:::caution
Cluster-wide `ClusterRole` bindings are intentionally out of scope. Chaos
permissions deserve explicit review by the platform team.
:::

See [RBAC requirements](./rbac.md) for the minimal permission set.

## Upgrading scenarios without restart

The pod mounts `ConfigMap` data via `subPath`, so **changes do not
propagate without a pod restart**. After editing `values.configs`:

```bash
helm upgrade chaos-zookoo ./helm -f my-values.yaml
kubectl -n chaos-system rollout restart deployment/chaos-zookoo
```
