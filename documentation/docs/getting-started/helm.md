---
title: Deploy with Helm
sidebar_position: 4
---

# Deploy with Helm

`chaos_zookoo` ships a Helm chart under `helm/` in the repository.
It deploys a single `Deployment`, mounts your scenarios via a `ConfigMap`,
and optionally sidecars a Grafana Alloy exporter.

## Prerequisites

- Helm 3.x
- A `ServiceAccount` with the appropriate RBAC already created in the target
  namespace — see [RBAC requirements](../deployment/rbac.md). The chart does
  **not** create a `Role` or `RoleBinding` for you.

## Install

```bash
helm install chaos-zookoo ./helm \
  --namespace chaos-system --create-namespace \
  --values my-values.yaml
```

## Kubernetes credentials

The chart expects three values to build the out-of-cluster REST config:

```yaml
k8s:
  host: "https://api.my-cluster.example.com"
  clusterCert: "<base64-encoded-CA-cert>"
  token: "<bearer-token>"
```

These are stored in a `Secret` created by the chart (`<release>-secret`).
Prefer passing them via `--set-string` or a Sealed Secret rather than
committing plain values to source control.

## Scenarios

Each entry under `configs` is a scenario object. The chart serialises them
and concatenates them with `---` into the `ConfigMap` mounted at
`/app/config.yaml`.

```yaml
configs:
  - kind: Killing
    name: kill-frontend
    namespace: production
    schedule:
      interval: 5m
      initialDelay: 30s
    selector:
      labels:
        app: frontend
    minAvailable: 2
    dryRun: false

  - kind: GorillaKill
    name: mass-kill-workers
    namespace: production
    schedule:
      interval: 1h
    selector:
      labels:
        app: worker
```

:::note
`toYaml` renders keys in alphabetical order — this is cosmetic only, the
binary reads fields by name regardless of order.
:::

## ServiceAccount

By default the chart references an existing `ServiceAccount` (falls back to
`default`). Set `serviceAccount.name` to point to your own SA.

```yaml
serviceAccount:
  create: false
  name: "chaos-zookoo"  # must already exist with the right Role bound
```

Set `create: true` to let the chart generate the `ServiceAccount`. The
chart will **not** create a `Role` or `RoleBinding` either way — grant them
separately (see [RBAC requirements](../deployment/rbac.md)).

## Using testkit — required environment variables

`testkit` (the post-run Prometheus assertion middleware) calls the Grafana
HTTP API to evaluate queries. The following environment variables **must** be
present in the container when any scenario declares a `testing:` block:

| Variable        | Description                                                          |
| --------------- | -------------------------------------------------------------------- |
| `GRAFANA_URL`   | Base URL of your Grafana instance, e.g. `https://grafana.example.com` |
| `GRAFANA_TOKEN` | Service-account token with at least `Viewer` rights on the datasource |

Pass them via `extraEnv`:

```yaml
extraEnv:
  - name: GRAFANA_URL
    value: "https://grafana.example.com"
  - name: GRAFANA_TOKEN
    valueFrom:
      secretKeyRef:
        name: chaos-zookoo-grafana   # see extraSecrets below
        key: GRAFANA_TOKEN
```

:::caution
Never put `GRAFANA_TOKEN` as a plain `value:` in a values file that reaches
source control. Use `secretKeyRef` pointing at a secret created either by
`extraSecrets` (below) or by an external secrets operator.
:::

## extraSecrets — create secrets from the chart

`extraSecrets` lets the chart render one or more `Secret` resources in the
same namespace as the release.

```yaml
extraSecrets:
  - name: chaos-zookoo-grafana
    stringData:
      GRAFANA_TOKEN: "glsa_xxxxxxxxxxxxxxxxxxxx"
```

Use `data` instead of `stringData` if the value is already base64-encoded:

```yaml
extraSecrets:
  - name: chaos-zookoo-grafana
    data:
      GRAFANA_TOKEN: "Z2xzYV94eHh4eHh4eHh4eHh4eHh4eA=="
```

:::tip Production recommendation
In production, replace `extraSecrets` with an external secrets operator
(e.g. External Secrets Operator + Vault / AWS Secrets Manager) and keep
`extraSecrets: []`.
:::

## Full testkit example

```yaml
# my-values.yaml

k8s:
  host: "https://api.prod.example.com"
  clusterCert: "<base64-CA>"
  token: "<service-account-token>"

serviceAccount:
  create: false
  name: "chaos-zookoo"

extraSecrets:
  - name: chaos-zookoo-grafana
    stringData:
      GRAFANA_TOKEN: "glsa_xxxxxxxxxxxxxxxxxxxx"

extraEnv:
  - name: GRAFANA_URL
    value: "https://grafana.example.com"
  - name: GRAFANA_TOKEN
    valueFrom:
      secretKeyRef:
        name: chaos-zookoo-grafana
        key: GRAFANA_TOKEN

configs:
  - kind: Killing
    name: kill-frontend
    namespace: production
    schedule:
      interval: 5m
    selector:
      labels:
        app: frontend
    minAvailable: 2
    testing:
      datasource: prometheus-prod
      wait: 2m
      rules:
        - expr: 'sum(rate(http_requests_total{job="frontend",status=~"5.."}[2m])) / sum(rate(http_requests_total{job="frontend"}[2m])) < 0.01'
          description: "error rate below 1%"
```

## Full `values.yaml` reference

```yaml
image:
  repository: neryolab/chaos-zookoo
  tag: "0.1.0"

serviceAccount:
  create: false  # true = chart creates the SA (no RBAC)
  name: ""       # existing SA name; defaults to "default" when empty

k8s:
  host: ""
  clusterCert: ""
  token: ""

extraSecrets: []
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
    port: 9090
  exporter:
    enabled: false
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

## Upgrade

```bash
helm upgrade chaos-zookoo ./helm -f my-values.yaml
kubectl -n chaos-system rollout restart deployment/chaos-zookoo
```

The `ConfigMap` mounts via `subPath`, so a scenario change requires a pod
restart — `helm upgrade` alone is not enough.
