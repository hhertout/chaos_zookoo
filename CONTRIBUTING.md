# Contributing to Chaos Zookoo

Thank you for your interest in contributing! This document covers environment setup, code conventions, and the process for submitting changes.

## Table of contents

- [Setting up the development environment](#setting-up-the-development-environment)
- [Running locally](#running-locally)
- [Testing](#testing)
- [Code conventions](#code-conventions)
- [Adding a new module](#adding-a-new-module)
- [Submitting a pull request](#submitting-a-pull-request)

## Setting up the development environment

**Requirements:**

- Go 1.21+
- [`gofumpt`](https://github.com/mvdan/gofumpt) — mandatory formatter (stricter than `gofmt`)
- [`golangci-lint`](https://golangci-lint.run/) — linter suite

```bash
go install mvdan.cc/gofumpt@latest
go install github.com/golangci-lint/golangci-lint/cmd/golangci-lint@latest
```

**Clone and configure:**

```bash
git clone https://github.com/hhertout/chaos_zookoo.git
cd chaos_zookoo
cp .env.example .env   # fill in K8S_HOST, K8S_TOKEN, K8S_CLUSTER_CERT
```

The required environment variables are:

| Variable | Description |
| --- | --- |
| `K8S_HOST` | Kubernetes API server URL |
| `K8S_CLUSTER_CERT` | Base64-encoded cluster CA certificate |
| `K8S_TOKEN` | ServiceAccount bearer token |
| `GRAFANA_URL` | Grafana base URL (only needed when a module declares `testing:`) |
| `GRAFANA_TOKEN` | Grafana bearer token (same condition) |
| `METRICS_ADDR` | Address for the Prometheus `/metrics` endpoint (default: `:9090`) |
| `CHAOS_CONFIG_DIR` | Default config directory when `--config` flag is not set |

## Running locally

```bash
make build
./bin/chaos_zookoo --config ./examples

# Or point to a single file:
./bin/chaos_zookoo -C ./examples/killing.yaml

# make run builds and runs in one step, reading .env automatically:
make run
```

The Makefile is the single source of truth — prefer it over raw `go` invocations.

## Testing

Run the full test suite (race detector enabled):

```bash
make test
```

Run a single test or package:

```bash
go test -run TestParseConfig ./pkg/killing/...
go test -v -race -run TestOrchestrator_Start ./internal/orchestrator
```

Generate a coverage report:

```bash
make test-cover
```

**Testing conventions:**

- Use `kubernetes/fake.NewSimpleClientset` for all Kubernetes surface — no real cluster required.
- Config parsing → table-driven tests in `module_test.go`.
- `Run` behavior → scenario-oriented tests covering the happy path, dry-run, and safety floors.
- Do not mock the Kubernetes client at the interface level; the fake client is sufficient and exercises the real call paths.

## Code conventions

### Structure

Every module package follows a strict 4-file layout:

| File | Responsibility |
| --- | --- |
| `config.go` | `Config`/`Scenario` structs, `ParseConfig([]byte)`, validation, defaults |
| `module.go` | `Module` struct, `New(client, cfg)`, `Name` / `Schedule` / `Run` |
| `register.go` | `Build` function matching `module.Builder` |
| `module_test.go` | Table-driven parse tests + `Run` tests |

Do not merge these into fewer files. The layout exists to make navigation predictable across all module packages.

### Patterns

- **Pod selection** goes through `pkg/matchers.CollectPods` — do not reimplement pod listing. `Rollout` is the only exception: it targets workload objects directly.
- **Safety floors** (e.g. `minAvailable`) are enforced inside `Run` *before* any API mutation.
- **`dryRun: true`** must produce the exact same log output as a real run, minus the mutating API call.
- **Validation happens in `ParseConfig`**, not in `New`. `New` accepts an already-valid `Config` by value and must not fail.
- **Duration fields** are stored as raw strings in the exported `Scenario` (`RawInterval`, `RawWait`) and parsed into `time.Duration` on an unexported field with public accessors.

### Code style

- **No string-templated JSON.** Declare typed structs and `json.Marshal` them. See `restartPatch` in `pkg/rollout/module.go` for the pattern.
- **Logging** uses `go.uber.org/zap` via `zap.L()`. Use structured fields, not formatted strings. Every module-level log line must include at minimum `kind`, `name`, and `namespace`.
- **Error wrapping** uses `%w` with a short context fragment: `"evicting pod %s: %w"`. Never use bare `fmt.Errorf("failed: %v", err)`.
- **Linting**: `golangci-lint` enables `gocritic`, `errorlint`, `prealloc`, `unparam`, and `nilerr`. Fix all findings — do not suppress with `//nolint` unless there is a documented reason.
- **Formatting**: `gofumpt` is mandatory. Run `make fmt` before committing.

### Cross-cutting concerns

The YAML for each module document is parsed twice: once by the module's own `Builder`, and once by `internal/config` for cross-cutting blocks (`testing:`, `load:`). This keeps module packages unaware of observability and load concerns.

When adding a new cross-cutting block: extend `internal/config/crosscutting.go` and create a new middleware package under `pkg/`. Do **not** add cross-cutting logic inside a module package.

## Adding a new module

1. Create `pkg/<kind>/config.go`, `module.go`, `register.go`, `module_test.go` following the 4-file layout above.
2. In `config.go`: validate required fields, parse durations, and reject cross-cutting keys (`testing:`, `load:` — these are handled centrally).
3. In `module.go`: implement `module.ChaosModule`. Use `matchers.CollectPods` for selection. Honor `dryRun`.
4. In `register.go`: export a `Build` function matching the `module.Builder` signature.
5. In `cmd/chaos_zookoo/main.go`: add your kind to the `builders` map.
6. Add an annotated example under `examples/<kind>.yaml`. The existing examples are the primary documentation surface — match their level of inline commentary.
7. Verify that `testing:` and `load:` blocks work without any code changes on your side. If they don't, something is wrong with the cross-cutting wiring, not the module.

## Submitting a pull request

1. Fork the repository and create a branch from `main`.
2. Run the full CI gate before pushing:

```bash
make check   # tidy + fmt + vet + lint + test
```

3. Keep commits focused. One logical change per commit.
4. Open a pull request against `main`. Describe **what** changed and **why**. Reference any related issue.
5. A maintainer will review your PR. Expect feedback on adherence to the patterns above — consistency across modules matters more than cleverness.
