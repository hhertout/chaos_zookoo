# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project role

`chaos_zookoo` is a Kubernetes chaos-engineering agent in the spirit of Chaos Mesh, but deliberately scoped down to the **Kubernetes API server**: every disruption is expressed as a regular API call (`EvictV1`, `Pods.Delete`, `Deployments.Patch`, ...) through `client-go`. There is no privileged node-level component, no DaemonSet poking at `/proc`, no custom sidecars: the agent is a single long-running process authenticated as a `ServiceAccount`, which means the **cluster's RBAC is the security model**. Whatever the SA can do, the agent can do; whatever it can't, the agent can't. This is the whole point — it lets platform teams grant fine-grained, auditable chaos permissions instead of deploying a framework with cluster-admin.

Configuration is YAML-driven (like Chaos Mesh CRDs), but the YAML is consumed locally by the process, not applied to the cluster. Each YAML document declares a `kind:` that maps to a **module**. Modules run on their own schedule (periodic or once) and can be wrapped with orthogonal **middlewares** (synthetic HTTP load, post-run observability assertions) without knowing they exist.

## Common commands

The Makefile is the single source of truth — prefer it over raw `go` invocations.

| Target              | What it does                                                        |
| ------------------- | ------------------------------------------------------------------- |
| `make build`        | Produces `bin/chaos_zookoo` from `./cmd/chaos_zookoo`.              |
| `make run`          | Build, then run the binary (reads `.env` and `CHAOS_CONFIG_DIR`).   |
| `make test`         | `go test -v -race ./...`                                            |
| `make test-cover`   | Race-enabled coverage run + `go tool cover -func`.                  |
| `make lint`         | `golangci-lint run ./...` (config in `.golangci.yml`).              |
| `make fmt`          | `gofumpt -w .` — the only accepted formatter.                       |
| `make vet` / `vuln` | `go vet` / `govulncheck`.                                           |
| `make check`        | CI gate: `tidy + fmt + vet + lint + test`. Run before pushing.      |

Running a single test:

```bash
go test -run TestParseConfig ./pkg/killing/...
go test -v -race -run TestOrchestrator_Start ./internal/orchestrator
```

Running the binary locally:

```bash
# Config source: flag wins, else $CHAOS_CONFIG_DIR, else ./configs.
./bin/chaos_zookoo --config ./examples
./bin/chaos_zookoo -C ./examples/killing.yaml   # single file is fine
```

Required environment variables are documented in `.env.example` (K8s connection
is out-of-cluster via `K8S_HOST`/`K8S_TOKEN`/`K8S_CLUSTER_CERT`; Grafana is only
needed when a module declares a `testing:` block).

## Architecture

### Top-level layout

```
cmd/chaos_zookoo/        # main + out-of-cluster REST config builder
internal/config/         # YAML loader + cross-cutting concerns parser
internal/orchestrator/   # schedules module.Run() per-module goroutine
pkg/module/              # the ChaosModule contract + Builder/Middleware types
pkg/matchers/            # selector model + pod collection (labels, pod, workload)
pkg/killing/             # module: random single-pod kill (evict|delete)
pkg/gorillakill/         # module: mass kill every matching pod (once|periodic)
pkg/rollout/             # module: rollout restart via spec.template annotation
pkg/testkit/             # middleware: post-run Grafana/Prometheus assertion
pkg/loadkit/             # middleware: synthetic HTTP load burst
pkg/metrics/             # Prometheus registry + /metrics HTTP server
helm/                    # Helm chart (Deployment, ConfigMap, optional Alloy sidecar)
examples/                # annotated YAML for each module kind
```

### The `ChaosModule` contract (`pkg/module`)

Everything in `pkg/` orbits around one interface:

```go
type ChaosModule interface {
    Name() string
    Run(ctx context.Context) error
    Schedule() Schedule
}

type Builder    func(client kubernetes.Interface, data []byte) (ChaosModule, error)
type Middleware func(ChaosModule) ChaosModule
```

- **`Schedule`** is returned by the module itself — the orchestrator doesn't decide cadence, the module does. `ScheduleOnce` runs after `InitialDelay` and exits; `SchedulePeriodic` fires on `Interval` with optional `InitialDelay`.
- **`Builder`** is the registration point. `cmd/chaos_zookoo/main.go` holds a `map[kind]Builder` — add a new module by adding a line there and implementing `Build` in the module's `register.go`.
- **`Middleware`** is a decorator that returns a new `ChaosModule`. Middlewares preserve `Name()` and `Schedule()` and wrap `Run()`. **Middlewares must not change scheduling semantics** — they are strictly additive (before/after/around the wrapped Run).

### Config flow

```
YAML file(s)
  └── config.LoadEntries           → map[kind][][]byte   (splits on "\n---")
         └── builders[kind].Build   → ChaosModule        (module-specific parse)
         └── config.ParseCrossCutting → testing/load specs
                └── testkit.NewMiddleware(...)
                └── loadkit.NewMiddleware(...)
                        └── orch.Register(testMw(loadMw(m)))
```

Key invariant: **each YAML document is parsed twice** — once by the module for its own fields (via its `Builder`), once by `internal/config` for cross-cutting blocks (`testing:`, `load:`). This keeps module packages unaware of cross-cutting concerns. When adding a new cross-cutting block, extend `internal/config/crosscutting.go` and a new middleware package under `pkg/`; do **not** teach a module about it.

### Orchestrator (`internal/orchestrator`)

One goroutine per registered module. The orchestrator owns a `stopCh` and a `WaitGroup` and coordinates graceful shutdown from `SIGINT`/`SIGTERM` via the `context.Context` passed to `Start`. `Stop()` is idempotent. The orchestrator **does not** own the module lifecycle beyond scheduling — modules must be cancellation-aware via the `ctx` they receive in `Run`.

Note: `execute()` currently takes `o.mu` for the duration of `module.Run`, so two modules cannot run concurrently. Respect that when adding long-running module work; prefer short `Run` with deferred work (as `testkit` does via `time.AfterFunc`).

### Modules (`pkg/<kind>`)

Every module package follows the same 4-file layout — stick to it when adding a new kind:

| File             | Responsibility                                                          |
| ---------------- | ----------------------------------------------------------------------- |
| `config.go`      | `Config`/`Scenario` structs, `ParseConfig([]byte)`, validation, defaults. |
| `module.go`      | `Module` struct, `New(client, cfg)`, `Name/Schedule/Run`.               |
| `register.go`    | `Build` function matching `module.Builder`.                             |
| `module_test.go` | Table-driven parse tests + `Run` tests using `kubernetes/fake.Clientset`. |

Shared patterns to keep consistent:
- **Targeting** goes through `pkg/matchers`. Do not reimplement pod listing — call `matchers.CollectPods` (union of label selector / pod name / workload owner). `Rollout` is the exception: it targets workload objects directly, not pods.
- **Safety floors** (e.g. `minAvailable` in `killing`) are enforced inside `Run` *before* any API mutation.
- **`dryRun: true`** must produce the exact same logs as a real run, minus the mutating call.
- **Validation failures are returned from `ParseConfig`**, never from `New` — `New` accepts an already-valid `Config` by value.
- **Duration fields** are stored as raw strings in the exported `Scenario` (`RawInterval`, `RawWait`) and the parsed `time.Duration` is kept on an unexported field on `Config` with public accessors.

### Middlewares (`pkg/testkit`, `pkg/loadkit`)

Both follow the same shape: a typed `Spec` parsed by `internal/config`, an `ApplyDefaultsAndValidate(scenarioInterval)` method, and a `NewMiddleware(...)` constructor that returns `module.Middleware`. The middleware returns a no-op decorator when its spec is nil, so modules that don't declare `testing:` / `load:` incur zero overhead.

- **`loadkit`** fires an HTTP burst *in parallel* with `inner.Run`, tracked by a `Supervisor` so shutdown can drain in-flight bursts.
- **`testkit`** schedules evaluation *after* `inner.Run` with `time.AfterFunc(spec.Wait())`. The Grafana client queries Prometheus via the datasource proxy and the result is exposed as `chaos_test_success{name="<module>", namespace="<namespace>"}` (1/0).

The wrap order in `main.registerModules` is `orch.Register(testMw(loadMw(m)))` — load fires *during* the action, test fires *after* the whole thing. Don't reorder without thinking it through.

### Metrics (`pkg/metrics`)

Prometheus registry is package-level and populated in `init()`. Four metrics today: `chaos_test_success`, `chaos_loading_http_active`, `chaos_load_requests_total`, `chaos_load_request_duration_seconds`. HTTP server bound to `METRICS_ADDR` (default `:9090`). When adding a module-observable signal, expose it here rather than importing Prometheus from the module package.

## Adding a new module (checklist)

1. Create `pkg/<kind>/{config.go,module.go,register.go,module_test.go}` following the 4-file layout above.
2. Make `Config.ParseConfig` validate required fields, parse durations, and refuse cross-cutting keys (`testing:`/`load:` are handled centrally).
3. Implement `Module` against `module.ChaosModule`. Use `matchers.CollectPods` for selection. Honor `dryRun`.
4. Export a `Build` function matching `module.Builder` in `register.go`.
5. Register it in `cmd/chaos_zookoo/main.go`'s `builders` map under the `kind:` value you chose.
6. Add an annotated sample under `examples/<kind>.yaml` — the existing samples are the documentation surface; keep the same level of inline commentary.
7. Confirm it composes with existing middlewares (`testing:`/`load:` blocks should still apply without code changes on your side).

## Code conventions

- Follow the patterns already in `pkg/killing` / `pkg/rollout` — new code that diverges will be hard to merge.
- **No string-templated JSON.** When building API payloads (e.g. strategic-merge patches) declare typed structs and `json.Marshal` them — see the `restartPatch` chain in `pkg/rollout/module.go`.
- Logging is `go.uber.org/zap` via the global logger (`zap.L()`), initialized in `main`. Use structured fields, not formatted strings. Include at minimum `kind`, `name`, `namespace` on every module-level log.
- Errors wrap with `%w` and a short context fragment (`"evicting pod %s: %w"`), never bare `fmt.Errorf("failed: %v", err)`.
- Tests use `kubernetes/fake.NewSimpleClientset` for the K8s surface. Table-driven tests for config parsing, scenario-oriented tests for `Run`.
- `gofumpt` is mandatory (stricter than `gofmt`). `golangci-lint` config enables `gocritic` (diagnostic + style + performance), `errorlint`, `prealloc`, `unparam`, `nilerr` — fix them, don't `nolint` them.

## Additional docs

- [Contributing](CONTRIBUTING.md): how to set up a dev environment, run tests, and submit a PR.
- [Codeowners](CODEOWNERS): who owns which parts of the codebase for review purposes.