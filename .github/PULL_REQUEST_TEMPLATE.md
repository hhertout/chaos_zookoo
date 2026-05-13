## What

<!-- One or two sentences: what does this PR do? -->

## Why

<!-- Motivation: which issue, failure scenario, or gap does this address? Link with `Closes #<n>` if applicable. -->

## How

<!-- Notable implementation choices, trade-offs, or anything a reviewer should pay close attention to. Skip if the diff is self-explanatory. -->

## Type of change

- [ ] Bug fix
- [ ] New module (`kind: <kind>`)
- [ ] New middleware (`kind: <kind>`)
- [ ] New cross-cutting concern (config, orchestrator, metrics)
- [ ] Enhancement to existing feature
- [ ] Cross-cutting concern (config, orchestrator, metrics)
- [ ] Docs / examples only
- [ ] Chore (deps, CI, tooling)

## Checklist

- [ ] `make check` passes locally (`tidy + fmt + vet + lint + test`)
- [ ] New or changed behaviour is covered by tests (`fake.Clientset` for K8s surface)
- [ ] `dryRun: true` produces the same logs as a real run, minus the mutating call
- [ ] No `//nolint` added without a documented reason
- [ ] Documentation added or updated (/documentation)

**If this adds a new module:**
- [ ] 4-file layout: `config.go`, `module.go`, `register.go`, `module_test.go`
- [ ] Registered in `cmd/chaos_zookoo/main.go`
- [ ] Example added under `examples/<kind>.yaml`
- [ ] `testing:` and `load:` middleware blocks work without module-side changes

**If this changes the YAML config surface:**
- [ ] `examples/` updated
- [ ] Backwards-compatible or breaking change is called out explicitly below

## Breaking changes

<!-- None, or describe what operators need to change in their YAML / Helm values. -->
