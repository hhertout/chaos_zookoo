---
title: Architecture
sidebar_position: 1
---

# Architecture

A high-level map of the codebase for contributors. For end-user
documentation, start at the [Introduction](../intro.md).

## Repository layout

```
cmd/chaos_zookoo/        # main + out-of-cluster REST config builder
internal/config/         # YAML loader + cross-cutting concerns parser
internal/orchestrator/   # schedules module.Run() per-module goroutine
pkg/module/              # the ChaosModule contract + Builder/Middleware types
pkg/matchers/            # selector model + pod collection
pkg/killing/             # module: random single-pod kill
pkg/gorillakill/         # module: mass kill every matching pod
pkg/rollout/             # module: rollout restart via annotation patch
pkg/testkit/             # middleware: post-run observability check
pkg/loadkit/             # middleware: synthetic HTTP load
pkg/metrics/             # Prometheus registry + /metrics server
helm/                    # Helm chart
examples/                # annotated YAML for each module kind
```

## The core contract

Everything in `pkg/` orbits around one interface in `pkg/module`:

```go
type ChaosModule interface {
    Name() string
    Run(ctx context.Context) error
    Schedule() Schedule
}

type Builder    func(client kubernetes.Interface, data []byte) (ChaosModule, error)
type Middleware func(ChaosModule) ChaosModule
```

- **`Schedule`** is chosen by the module, not the orchestrator.
- **`Builder`** is the registration point for a new kind. `main` holds
  `map[kind]Builder`.
- **`Middleware`** is a decorator over `ChaosModule`. It must preserve
  `Name()` and `Schedule()`, and only wrap `Run`.

## Config flow

```
YAML file(s)
    └── config.LoadEntries            → map[kind][][]byte   (splits on "\n---")
         └── builders[kind].Build      → ChaosModule        (module-specific parse)
         └── config.ParseCrossCutting  → Testing + Load specs
             └── testkit.NewMiddleware(...)
             └── loadkit.NewMiddleware(...)
                └── orch.Register(testMw(loadMw(m)))
```

**Invariant:** each YAML document is parsed twice — once by the module
for its own fields, once by `internal/config` for cross-cutting blocks
(`testing:`, `load:`). This keeps module packages unaware of
cross-cutting concerns. When adding a new cross-cutting concern, extend
`internal/config/crosscutting.go` and a new middleware package under
`pkg/` — never teach an existing module about it.

## Orchestrator

One goroutine per registered module. The orchestrator:

- owns a `stopCh` and a `WaitGroup`,
- coordinates graceful shutdown from `SIGINT` / `SIGTERM` via
  `context.Context`,
- serializes ticks of the same module (one `Run` at a time),
- but runs different modules in parallel.

`execute()` takes `o.mu` for the duration of `Run` — two modules' ticks
cannot overlap. Keep `Run` fast. Defer long work (use
`time.AfterFunc` or goroutines owned by a supervisor, as
[`testkit`](../middlewares/testkit.md) and
[`loadkit`](../middlewares/loadkit.md) do).

## Module package shape

Every module package follows a 4-file layout — stick to it:

| File             | Responsibility                                                          |
| ---------------- | ----------------------------------------------------------------------- |
| `config.go`      | `Config`/`Scenario` structs, `ParseConfig([]byte)`, validation, defaults. |
| `module.go`      | `Module` struct, `New(client, cfg)`, `Name/Schedule/Run`.               |
| `register.go`    | `Build` function matching `module.Builder`.                             |
| `module_test.go` | Table-driven parse tests + `Run` tests using `kubernetes/fake.Clientset`. |

Shared conventions:

- Targeting goes through `pkg/matchers.CollectPods`. Don't reimplement
  pod listing. `Rollout` is the exception — it targets workload objects
  directly.
- **Validation failures are returned from `ParseConfig`**, never from
  `New`. `New` accepts a valid `Config` by value.
- **Duration fields** are stored as raw strings in the exported scenario
  (`RawInterval`, `RawWait`) and the parsed `time.Duration` is kept on
  an unexported field on `Config` with public accessors.
- **`dryRun: true`** produces the same logs as a real run minus the
  mutating call.
- **No string-templated JSON.** When building API payloads (e.g.
  strategic-merge patches), declare typed structs and `json.Marshal`
  them — see the `restartPatch` chain in `pkg/rollout/module.go`.

## Middleware package shape

Both `pkg/testkit` and `pkg/loadkit` follow the same shape:

- a typed `Spec` parsed by `internal/config`,
- an `ApplyDefaultsAndValidate(scenarioInterval time.Duration) error`
  method — nil receivers are valid,
- a `NewMiddleware(...)` constructor returning `module.Middleware` that
  returns a no-op wrapper when the spec is nil,
- a process-wide supervisor (`Supervisor` / `Runner`) built in `main`
  and `Stop()`-ed at shutdown.

## Logging & metrics

- Logging: `go.uber.org/zap` via the global logger (`zap.L()`). Include
  at minimum `kind`, `name`, `namespace` on every module-level log.
- Metrics: registered in `pkg/metrics` only. Don't import
  `client_golang` directly from modules.

## Testing

- Table-driven tests for `ParseConfig`.
- Scenario-oriented tests for `Run` using
  `k8s.io/client-go/kubernetes/fake.NewSimpleClientset`.
- `go test -race ./...` is the baseline; `make check` is the CI gate.
