---
title: Adding a module
sidebar_position: 2
---

# Adding a new module

Checklist for implementing a new chaos `kind`. The existing
`pkg/killing` package is the best reference to copy from.

## 1. Create the package

```
pkg/<kind>/
├── config.go        # Config, Scenario, ParseConfig([]byte)
├── module.go        # Module struct, New(client, cfg), Name/Schedule/Run
├── register.go      # Build function matching module.Builder
└── module_test.go   # ParseConfig tests + Run tests with fake client
```

Stick to these four files. Anything else will be hard to review because
it will diverge from the shape readers already expect.

## 2. Implement `ParseConfig`

```go
type Config struct {
    Kind     string          `yaml:"kind"`
    Name     string          `yaml:"name"`
    Metadata module.Metadata `yaml:"metadata"`
    Scenario Scenario        `yaml:"scenario"`

    interval time.Duration   // unexported, parsed from Scenario.RawInterval
    wait     time.Duration   // unexported, parsed from Scenario.RawWait
}

type Scenario struct {
    RawInterval string `yaml:"interval"`
    RawWait     string `yaml:"wait"`
    DryRun      bool   `yaml:"dryRun"`
    Matchers    matchers.Matchers `yaml:"matchers"`
    // ... your module-specific fields
}

func ParseConfig(data []byte) (Config, error) { /* ... */ }
```

Rules:

- All validation happens here, **not** in `New`.
- Return concrete error messages: `"<kind> config requires a name"`,
  `"invalid scenario.interval %q: %w"`.
- Never touch `testing:` / `load:` — those are centrally handled.

## 3. Implement `Module`

```go
var _ module.ChaosModule = (*Module)(nil)

type Module struct { /* fields */ }

func New(client kubernetes.Interface, cfg Config) *Module { /* ... */ }

func (m *Module) Name() string            { return m.name }
func (m *Module) Schedule() module.Schedule { /* ... */ }
func (m *Module) Run(ctx context.Context) error { /* ... */ }
```

Rules:

- `Name()` returns the user-provided `name:` — it's used as the
  Prometheus label value and in every log line.
- `Run` must honor `ctx` on every API call.
- `Run` must honor `dryRun`: log the intended action, skip the mutation.
- Always include `kind`, `name`, `namespace` in log fields.
- Target selection goes through `matchers.CollectPods` — unless your
  module acts on workloads directly, like `Rollout`.

## 4. Expose `Build`

```go
// register.go
func Build(client kubernetes.Interface, data []byte) (module.ChaosModule, error) {
    cfg, err := ParseConfig(data)
    if err != nil {
        return nil, fmt.Errorf("invalid <kind> config: %w", err)
    }
    return New(client, cfg), nil
}
```

## 5. Register in `main`

Open `cmd/chaos_zookoo/main.go` and extend:

```go
var builders = map[string]module.Builder{
    "Killing":     killing.Build,
    "Rollout":     rollout.Build,
    "GorillaKill": gorillakill.Build,
    "<Kind>":      <pkg>.Build,   // ← add this
}
```

The `kind:` string in YAML is matched verbatim against this map.

## 6. Add an example

Drop an annotated `examples/<kind>.yaml`. The existing samples are the
documentation surface — match their level of inline commentary. Every
field should have a comment explaining what it does, its default, and
its constraints.

## 7. Write tests

- `ParseConfig` — table-driven over valid and invalid inputs. Every
  validation branch gets a case.
- `Run` — with `fake.NewSimpleClientset`, assert on the API calls made
  (or not made, in `dryRun`).

## 8. Add user-facing documentation

Create `documentation/docs/modules/<kind>.md` following the shape of the
existing module pages: *When to use it*, *Minimal config*, *Full
reference*, *Behavior*, *RBAC*.

Update `documentation/sidebars.ts` so the new page appears in the
`Modules` category.

## Adding a middleware

The middleware path is separate — it does **not** touch existing
modules.

1. Create `pkg/<middleware>/` with:
   - `config.go` exposing a `Spec` with
     `ApplyDefaultsAndValidate(scenarioInterval time.Duration) error`,
   - `middleware.go` exposing `NewMiddleware(sup *Supervisor, spec *Spec) module.Middleware`,
   - a process-wide supervisor tracking in-flight work and a `Stop()`
     that drains it.
2. Extend `internal/config/crosscutting.go` with a new top-level YAML
   key and call `spec.ApplyDefaultsAndValidate(interval)` from
   `ParseCrossCutting`.
3. Instantiate the supervisor in `cmd/chaos_zookoo/main.go` and wrap
   `m` with the new middleware in `registerModules` — pay attention to
   the wrap order (see [Middlewares overview](../middlewares/overview.md#composition-order)).
4. Add tests and a doc page.

The test that you got it right: you can **disable the new middleware
for all existing scenarios by removing the YAML block** and nothing
about the modules changes. That's the invariant.
