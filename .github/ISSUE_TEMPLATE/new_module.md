---
name: New module proposal
about: Propose a new chaos module (new `kind:` in YAML)
title: "feat(module): "
labels: new-module
assignees: ""
---

## Module summary

**Kind value** (the `kind:` key in YAML): `<kind>`

<!-- One sentence: what disruption does this module introduce? -->

## Motivation

<!-- Why is this useful? What failure mode or resilience scenario does it test? -->

## API design

<!-- Sketch the YAML config a user would write. Mirror the style of `examples/*.yaml`. -->

```yaml
kind: <kind>
name: my-scenario
namespace: default
# ...
```

## Kubernetes API calls involved

<!-- List the client-go calls (e.g. Pods.Evict, Deployments.Patch). This determines the RBAC the SA needs. -->

## Safety considerations

<!-- Minimum-availability floor? Dry-run semantics? Any risk of cascading failures? -->

## Out of scope

<!-- Anything explicitly NOT part of this proposal. -->

## Checklist (to be filled in before merging)

- [ ] `pkg/<kind>/config.go` — structs, `ParseConfig`, validation, defaults
- [ ] `pkg/<kind>/module.go` — `Module`, `New`, `Name`/`Schedule`/`Run`
- [ ] `pkg/<kind>/register.go` — `Build` matching `module.Builder`
- [ ] `pkg/<kind>/module_test.go` — table-driven config tests + `Run` scenarios
- [ ] Registered in `cmd/chaos_zookoo/main.go`
- [ ] Example added under `examples/<kind>.yaml`
- [ ] `testing:` and `load:` middleware blocks work without module-side changes
- [ ] `make check` passes
