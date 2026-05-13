# NodeDrain Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a `NodeDrain` chaos module that simulates a rolling cluster upgrade by killing all pods in a namespace per node, one node at a time, with configurable guard exclusions.

**Architecture:** Sequential drain loop in `Run()` — for each schedulable node (alphabetical order), collect all namespace pods scheduled on it, subtract guard-excluded pods, kill the rest via `evict|delete`, then poll for recovery before advancing. Guard exclusion and recovery checks happen per-node. No goroutines inside `Run`.

**Tech Stack:** Go 1.21+, `client-go` (kubernetes.Interface + fake.Clientset for tests), `go.uber.org/zap`, `gopkg.in/yaml.v3`, `github.com/adhocore/gronx`, `github.com/stretchr/testify`.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `pkg/nodedrain/config.go` | Create | Config/Scenario/Specs/Guard structs, ParseConfig, all validation |
| `pkg/nodedrain/module.go` | Create | Module struct, PodRemover interface, New, Schedule, Run, drainNode, waitForRecovery |
| `pkg/nodedrain/register.go` | Create | Build function matching module.Builder |
| `pkg/nodedrain/module_test.go` | Create | Table-driven ParseConfig tests + scenario Run tests |
| `cmd/chaos_zookoo/main.go` | Modify | Add `"NodeDrain": nodedrain.Build` to builders map |
| `examples/nodedrain.yaml` | Create | Fully annotated YAML example |

---

## Task 1: Config parsing

**Files:**
- Create: `pkg/nodedrain/config.go`
- Create: `pkg/nodedrain/module_test.go` (config section only — Run section added in Task 3)

---

- [ ] **Step 1: Write the failing config test**

Create `pkg/nodedrain/module_test.go`:

```go
package nodedrain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParseConfig(t *testing.T, yaml string) Config {
	t.Helper()
	cfg, err := ParseConfig([]byte(yaml))
	require.NoError(t, err)
	return cfg
}

// --- Config tests ---

func TestParseConfig(t *testing.T) {
	const minimalOnce = `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: once
`
	const minimalPeriodic = `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: periodic
  interval: 60s
`

	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(t *testing.T, cfg Config)
	}{
		{
			name: "requires name",
			yaml: `kind: NodeDrain
metadata:
  namespace: default
scenario:
  when: once
`,
			wantErr: true,
		},
		{
			name: "requires namespace",
			yaml: `kind: NodeDrain
name: test
scenario:
  when: once
`,
			wantErr: true,
		},
		{
			name: "requires when",
			yaml: `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario: {}
`,
			wantErr: true,
		},
		{
			name: "rejects invalid when",
			yaml: `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: always
`,
			wantErr: true,
		},
		{
			name: "once rejects interval",
			yaml: `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: once
  interval: 60s
`,
			wantErr: true,
		},
		{
			name: "once rejects cron",
			yaml: `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: once
  cron: "*/5 * * * *"
`,
			wantErr: true,
		},
		{
			name:    "once accepts unbounded wait",
			yaml:    minimalOnce + "  wait: 120s\n",
			wantErr: false,
		},
		{
			name: "periodic requires interval or cron",
			yaml: `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: periodic
`,
			wantErr: true,
		},
		{
			name: "periodic rejects interval and cron together",
			yaml: `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: periodic
  interval: 60s
  cron: "*/5 * * * *"
`,
			wantErr: true,
		},
		{
			name: "periodic rejects invalid interval format",
			yaml: `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: periodic
  interval: "bad"
`,
			wantErr: true,
		},
		{
			name: "periodic rejects non-positive interval",
			yaml: `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: periodic
  interval: -30s
`,
			wantErr: true,
		},
		{
			name:    "periodic accepts cron schedule",
			yaml:    minimalPeriodic[:len(minimalPeriodic)-len("  interval: 60s\n")] + "  cron: \"*/5 * * * *\"\n",
			wantErr: false,
		},
		{
			name: "periodic rejects invalid cron expression",
			yaml: `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: periodic
  cron: "not-a-cron"
`,
			wantErr: true,
		},
		{
			name: "periodic rejects wait with cron",
			yaml: `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: periodic
  cron: "*/5 * * * *"
  wait: 30s
`,
			wantErr: true,
		},
		{
			name: "periodic rejects wait >= interval",
			yaml: `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: periodic
  interval: 60s
  wait: 90s
`,
			wantErr: true,
		},
		{
			name: "rejects invalid strategy",
			yaml: minimalOnce + "  specs:\n    strategy: nuke\n",
			wantErr: true,
		},
		{
			name:    "strategy defaults to evict",
			yaml:    minimalOnce,
			wantErr: false,
			check: func(t *testing.T, cfg Config) {
				t.Helper()
				assert.Equal(t, StrategyEvict, cfg.Scenario.Specs.Strategy)
			},
		},
		{
			name:    "readinessTimeout defaults to 5m",
			yaml:    minimalOnce,
			wantErr: false,
			check: func(t *testing.T, cfg Config) {
				t.Helper()
				assert.Equal(t, 5*time.Minute, cfg.Scenario.Specs.ReadinessTimeout())
			},
		},
		{
			name:    "rejects invalid readinessTimeout",
			yaml:    minimalOnce + "  specs:\n    readinessTimeout: bad\n",
			wantErr: true,
		},
		{
			name:    "rejects non-positive readinessTimeout",
			yaml:    minimalOnce + "  specs:\n    readinessTimeout: -1s\n",
			wantErr: true,
		},
		{
			name:    "minReady defaults to 1",
			yaml:    minimalOnce,
			wantErr: false,
			check: func(t *testing.T, cfg Config) {
				t.Helper()
				assert.Equal(t, 1, cfg.Scenario.Specs.MinReady)
			},
		},
		{
			name:    "guard is optional",
			yaml:    minimalOnce,
			wantErr: false,
		},
		{
			name: "valid full config",
			yaml: `kind: NodeDrain
name: simulate-upgrade
metadata:
  namespace: production
scenario:
  when: periodic
  interval: 6h
  wait: 30s
  dryRun: false
  specs:
    strategy: delete
    readinessTimeout: 3m
    minReady: 2
    nodeSelector:
      kubernetes.io/role: worker
  guard:
    matchers:
      labels:
        app: critical
`,
			wantErr: false,
			check: func(t *testing.T, cfg Config) {
				t.Helper()
				assert.Equal(t, "simulate-upgrade", cfg.Name)
				assert.Equal(t, "production", cfg.Metadata.Namespace)
				assert.Equal(t, StrategyDelete, cfg.Scenario.Specs.Strategy)
				assert.Equal(t, 3*time.Minute, cfg.Scenario.Specs.ReadinessTimeout())
				assert.Equal(t, 2, cfg.Scenario.Specs.MinReady)
				assert.Equal(t, map[string]string{"kubernetes.io/role": "worker"}, cfg.Scenario.Specs.NodeSelector)
				assert.Equal(t, map[string]string{"app": "critical"}, cfg.Scenario.Guard.Matchers.Labels)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseConfig([]byte(tt.yaml))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails (package does not exist yet)**

```bash
go test ./pkg/nodedrain/...
```

Expected: compilation error — package not found.

- [ ] **Step 3: Implement `pkg/nodedrain/config.go`**

```go
package nodedrain

import (
	"fmt"
	"strings"
	"time"

	gronx "github.com/adhocore/gronx"
	"github.com/hhertout/chaos_zookoo/pkg/matchers"
	"github.com/hhertout/chaos_zookoo/pkg/module"
	"gopkg.in/yaml.v3"
)

type Strategy string

const (
	StrategyEvict  Strategy = "evict"
	StrategyDelete Strategy = "delete"
)

type When string

const (
	WhenOnce     When = "once"
	WhenPeriodic When = "periodic"
)

const (
	defaultReadinessTimeout = 5 * time.Minute
	defaultMinReady         = 1
)

type Config struct {
	Kind     string          `yaml:"kind"`
	Name     string          `yaml:"name"`
	Metadata module.Metadata `yaml:"metadata"`
	Scenario Scenario        `yaml:"scenario"`

	interval time.Duration
	wait     time.Duration
	cronExpr string
}

func (c Config) Interval() time.Duration { return c.interval }
func (c Config) Wait() time.Duration     { return c.wait }

type Scenario struct {
	When        When   `yaml:"when"`
	RawInterval string `yaml:"interval"`
	RawCron     string `yaml:"cron"`
	RawWait     string `yaml:"wait"`
	DryRun      bool   `yaml:"dryRun"`
	Specs       Specs  `yaml:"specs"`
	Guard       Guard  `yaml:"guard"`
}

type Specs struct {
	Strategy            Strategy          `yaml:"strategy"`
	NodeSelector        map[string]string `yaml:"nodeSelector"`
	RawReadinessTimeout string            `yaml:"readinessTimeout"`
	MinReady            int               `yaml:"minReady"`

	readinessTimeout time.Duration
}

func (s Specs) ReadinessTimeout() time.Duration { return s.readinessTimeout }

type Guard struct {
	Matchers matchers.Matchers `yaml:"matchers"`
}

func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing nodedrain config: %w", err)
	}

	cfg.Name = strings.TrimSpace(cfg.Name)
	if cfg.Name == "" {
		return Config{}, fmt.Errorf("nodedrain config requires a name")
	}
	if cfg.Metadata.Namespace == "" {
		return Config{}, fmt.Errorf("nodedrain config requires metadata.namespace")
	}

	switch cfg.Scenario.When {
	case WhenOnce:
		if cfg.Scenario.RawInterval != "" {
			return Config{}, fmt.Errorf("nodedrain: scenario.interval is not valid when scenario.when=once")
		}
		if cfg.Scenario.RawCron != "" {
			return Config{}, fmt.Errorf("nodedrain: scenario.cron is not valid when scenario.when=once")
		}
	case WhenPeriodic:
		if cfg.Scenario.RawInterval != "" && cfg.Scenario.RawCron != "" {
			return Config{}, fmt.Errorf("nodedrain config: scenario.interval and scenario.cron are mutually exclusive")
		}
		switch {
		case cfg.Scenario.RawCron != "":
			if !gronx.New().IsValid(cfg.Scenario.RawCron) {
				return Config{}, fmt.Errorf("invalid scenario.cron expression %q", cfg.Scenario.RawCron)
			}
			cfg.cronExpr = cfg.Scenario.RawCron
		case cfg.Scenario.RawInterval != "":
			dur, err := time.ParseDuration(cfg.Scenario.RawInterval)
			if err != nil {
				return Config{}, fmt.Errorf("invalid scenario.interval %q: %w", cfg.Scenario.RawInterval, err)
			}
			if dur <= 0 {
				return Config{}, fmt.Errorf("nodedrain scenario.interval must be > 0, got %s", cfg.Scenario.RawInterval)
			}
			cfg.interval = dur
		default:
			return Config{}, fmt.Errorf("nodedrain with scenario.when=periodic requires scenario.interval or scenario.cron")
		}
	case "":
		return Config{}, fmt.Errorf("nodedrain config requires scenario.when (once|periodic)")
	default:
		return Config{}, fmt.Errorf("invalid scenario.when %q: must be once or periodic", cfg.Scenario.When)
	}

	if cfg.Scenario.RawWait != "" {
		if cfg.Scenario.When == WhenPeriodic && cfg.cronExpr != "" {
			return Config{}, fmt.Errorf("nodedrain scenario.wait is not supported with cron scheduling")
		}
		w, err := time.ParseDuration(cfg.Scenario.RawWait)
		if err != nil {
			return Config{}, fmt.Errorf("invalid scenario.wait %q: %w", cfg.Scenario.RawWait, err)
		}
		if w < 0 {
			return Config{}, fmt.Errorf("nodedrain scenario.wait must be >= 0")
		}
		if cfg.Scenario.When == WhenPeriodic && w >= cfg.interval {
			return Config{}, fmt.Errorf("nodedrain scenario.wait (%s) must be < scenario.interval (%s)", w, cfg.interval)
		}
		cfg.wait = w
	}

	switch cfg.Scenario.Specs.Strategy {
	case "":
		cfg.Scenario.Specs.Strategy = StrategyEvict
	case StrategyEvict, StrategyDelete:
	default:
		return Config{}, fmt.Errorf("invalid specs.strategy %q: must be evict or delete", cfg.Scenario.Specs.Strategy)
	}

	if cfg.Scenario.Specs.RawReadinessTimeout != "" {
		d, err := time.ParseDuration(cfg.Scenario.Specs.RawReadinessTimeout)
		if err != nil {
			return Config{}, fmt.Errorf("invalid specs.readinessTimeout %q: %w", cfg.Scenario.Specs.RawReadinessTimeout, err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("specs.readinessTimeout must be > 0")
		}
		cfg.Scenario.Specs.readinessTimeout = d
	} else {
		cfg.Scenario.Specs.readinessTimeout = defaultReadinessTimeout
	}

	if cfg.Scenario.Specs.MinReady < 0 {
		return Config{}, fmt.Errorf("specs.minReady must be >= 0")
	}
	if cfg.Scenario.Specs.MinReady == 0 {
		cfg.Scenario.Specs.MinReady = defaultMinReady
	}

	return cfg, nil
}
```

- [ ] **Step 4: Run tests and verify they pass**

```bash
go test -v -race ./pkg/nodedrain/...
```

Expected: all `TestParseConfig` sub-tests PASS.

---

## Task 2: Module skeleton + register

**Files:**
- Create: `pkg/nodedrain/module.go`
- Create: `pkg/nodedrain/register.go`

---

- [ ] **Step 1: Create `pkg/nodedrain/module.go`**

```go
package nodedrain

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/matchers"
	"github.com/hhertout/chaos_zookoo/pkg/metrics"
	"github.com/hhertout/chaos_zookoo/pkg/module"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var _ module.ChaosModule = (*Module)(nil)

// PodRemover abstracts how a pod is removed from the cluster.
type PodRemover interface {
	Remove(ctx context.Context, namespace, name string) error
}

type evictRemover struct{ client kubernetes.Interface }

func (r evictRemover) Remove(ctx context.Context, ns, name string) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
	if err := r.client.CoreV1().Pods(ns).EvictV1(ctx, eviction); err != nil {
		return fmt.Errorf("evicting pod %s: %w", name, err)
	}
	return nil
}

type deleteRemover struct{ client kubernetes.Interface }

func (r deleteRemover) Remove(ctx context.Context, ns, name string) error {
	if err := r.client.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("deleting pod %s: %w", name, err)
	}
	return nil
}

func newPodRemover(client kubernetes.Interface, strategy Strategy) PodRemover {
	if strategy == StrategyDelete {
		return deleteRemover{client: client}
	}
	return evictRemover{client: client}
}

// Module simulates a rolling node restart by draining pods per node sequentially.
type Module struct {
	client       kubernetes.Interface
	remover      PodRemover
	name         string
	namespace    string
	specs        Specs
	guard        Guard
	dryRun       bool
	when         When
	interval     time.Duration
	wait         time.Duration
	cronExpr     string
	pollInterval time.Duration
}

// New creates a NodeDrain module from the given client and config.
func New(client kubernetes.Interface, cfg Config) *Module {
	return &Module{
		client:       client,
		remover:      newPodRemover(client, cfg.Scenario.Specs.Strategy),
		name:         cfg.Name,
		namespace:    cfg.Metadata.Namespace,
		specs:        cfg.Scenario.Specs,
		guard:        cfg.Scenario.Guard,
		dryRun:       cfg.Scenario.DryRun,
		when:         cfg.Scenario.When,
		interval:     cfg.interval,
		wait:         cfg.wait,
		cronExpr:     cfg.cronExpr,
		pollInterval: 5 * time.Second,
	}
}

func (m *Module) Name() string      { return m.name }
func (m *Module) Kind() string      { return "NodeDrain" }
func (m *Module) Namespace() string { return m.namespace }

func (m *Module) Schedule() module.Schedule {
	if m.when == WhenOnce {
		return module.Schedule{Mode: module.ScheduleOnce}
	}
	if m.cronExpr != "" {
		return module.Schedule{Mode: module.ScheduleCron, CronExpr: m.cronExpr}
	}
	return module.Schedule{Mode: module.SchedulePeriodic, Interval: m.interval}
}

func (m *Module) Run(ctx context.Context) error {
	time.Sleep(m.wait)

	nodes, err := m.listTargetNodes(ctx)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		zap.L().Warn("nodedrain skipped: no nodes matched",
			zap.String("kind", "NodeDrain"),
			zap.String("name", m.name),
			zap.String("namespace", m.namespace),
		)
		return nil
	}

	for _, node := range nodes {
		if err := m.drainNode(ctx, node.Name); err != nil {
			return err
		}
	}

	zap.L().Info("node drain simulation complete",
		zap.String("kind", "NodeDrain"),
		zap.String("name", m.name),
		zap.String("namespace", m.namespace),
		zap.Int("nodes", len(nodes)),
	)
	return nil
}

func (m *Module) listTargetNodes(ctx context.Context) ([]corev1.Node, error) {
	opts := metav1.ListOptions{}
	if len(m.specs.NodeSelector) > 0 {
		opts.LabelSelector = matchers.BuildLabelSelector(m.specs.NodeSelector)
	}
	nodeList, err := m.client.CoreV1().Nodes().List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	var result []corev1.Node
	for _, n := range nodeList.Items {
		if !n.Spec.Unschedulable {
			result = append(result, n)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (m *Module) drainNode(ctx context.Context, nodeName string) error {
	allPods, err := m.client.CoreV1().Pods(m.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing pods on node %s: %w", nodeName, err)
	}

	var nodePods []corev1.Pod
	for _, p := range allPods.Items {
		if p.Spec.NodeName == nodeName {
			nodePods = append(nodePods, p)
		}
	}

	excluded := make(map[string]struct{})
	if !m.guard.Matchers.IsEmpty() {
		guardPods, err := matchers.CollectPods(ctx, m.client, m.namespace, m.guard.Matchers)
		if err != nil {
			return fmt.Errorf("resolving guard on node %s: %w", nodeName, err)
		}
		for _, p := range guardPods {
			excluded[p.Name] = struct{}{}
		}
	}

	var targets []corev1.Pod
	for _, p := range nodePods {
		if _, ok := excluded[p.Name]; !ok {
			targets = append(targets, p)
		}
	}

	if len(targets) == 0 {
		zap.L().Warn("nodedrain: no targets on node, skipping",
			zap.String("kind", "NodeDrain"),
			zap.String("name", m.name),
			zap.String("namespace", m.namespace),
			zap.String("node", nodeName),
		)
		return nil
	}

	zap.L().Info("draining node",
		zap.String("kind", "NodeDrain"),
		zap.String("name", m.name),
		zap.String("namespace", m.namespace),
		zap.String("node", nodeName),
		zap.Int("pods", len(targets)),
		zap.Bool("dryRun", m.dryRun),
	)

	for _, p := range targets {
		zap.L().Info("pod killed",
			zap.String("kind", "NodeDrain"),
			zap.String("name", m.name),
			zap.String("namespace", m.namespace),
			zap.String("node", nodeName),
			zap.String("pod", p.Name),
			zap.String("strategy", string(m.specs.Strategy)),
			zap.Bool("dryRun", m.dryRun),
		)
		if m.dryRun {
			continue
		}
		if err := m.remover.Remove(ctx, m.namespace, p.Name); err != nil {
			return err
		}
		metrics.ChaosPodsAffectedTotal.WithLabelValues(m.name, "NodeDrain", m.namespace).Inc()
	}

	if m.dryRun {
		return nil
	}

	if err := m.waitForRecovery(ctx, nodeName); err != nil {
		zap.L().Warn("readiness timeout on node, continuing to next node",
			zap.String("kind", "NodeDrain"),
			zap.String("name", m.name),
			zap.String("namespace", m.namespace),
			zap.String("node", nodeName),
			zap.Error(err),
		)
	}

	return nil
}

func (m *Module) waitForRecovery(ctx context.Context, nodeName string) error {
	timeout := time.After(m.specs.ReadinessTimeout())
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("readiness timeout after %s waiting for node %s recovery", m.specs.ReadinessTimeout(), nodeName)
		case <-ticker.C:
			pods, err := m.client.CoreV1().Pods(m.namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				continue
			}
			running := 0
			for _, p := range pods.Items {
				if p.Status.Phase == corev1.PodRunning {
					running++
				}
			}
			if running >= m.specs.MinReady {
				zap.L().Info("node recovered",
					zap.String("kind", "NodeDrain"),
					zap.String("name", m.name),
					zap.String("namespace", m.namespace),
					zap.String("node", nodeName),
					zap.Int("runningPods", running),
				)
				return nil
			}
		}
	}
}
```

- [ ] **Step 2: Create `pkg/nodedrain/register.go`**

```go
package nodedrain

import (
	"fmt"

	"github.com/hhertout/chaos_zookoo/pkg/module"
	"k8s.io/client-go/kubernetes"
)

// Build satisfies module.Builder — it parses YAML and returns a ready module.
func Build(client kubernetes.Interface, data []byte) (module.ChaosModule, error) {
	cfg, err := ParseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("invalid nodedrain config: %w", err)
	}
	return New(client, cfg), nil
}
```

- [ ] **Step 3: Verify the package compiles**

```bash
go build ./pkg/nodedrain/...
```

Expected: no errors.

---

## Task 3: Run tests + full implementation verification

**Files:**
- Modify: `pkg/nodedrain/module_test.go` (append Run scenario tests)

> Note: Run tests use `strategy: delete` because `fake.Clientset` does not implement `EvictV1`. The evict path is covered by the PodRemover interface and is tested implicitly via config parsing.

---

- [ ] **Step 1: Append Run scenario tests to `pkg/nodedrain/module_test.go`**

Add these imports to the existing import block:

```go
import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)
```

Append after the existing `TestParseConfig` function:

```go
// --- Helpers ---

func makeNode(name string, labels map[string]string, unschedulable bool) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec:       corev1.NodeSpec{Unschedulable: unschedulable},
	}
}

func makePod(name, nodeName, namespace string, labels map[string]string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec:       corev1.PodSpec{NodeName: nodeName},
		Status:     corev1.PodStatus{Phase: phase},
	}
}

func listPods(t *testing.T, client *fake.Clientset, ns string) []string {
	t.Helper()
	pods, err := client.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	names := make([]string, 0, len(pods.Items))
	for _, p := range pods.Items {
		names = append(names, p.Name)
	}
	return names
}

// newTestModule creates a Module with delete strategy and fast poll/timeout for tests.
func newTestModule(client *fake.Clientset, extraYAML string) *Module {
	base := `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: once
  specs:
    strategy: delete
    readinessTimeout: 50ms
    minReady: 1
`
	cfg, err := ParseConfig([]byte(base + extraYAML))
	if err != nil {
		panic("newTestModule: invalid config: " + err.Error())
	}
	m := New(client, cfg)
	m.pollInterval = 10 * time.Millisecond
	return m
}

// --- Run tests ---

func TestRun_HappyPath_TwoNodes(t *testing.T) {
	node1 := makeNode("node-alpha", nil, false)
	node2 := makeNode("node-beta", nil, false)
	// Two pods on node-alpha, one on node-beta. All Running so recovery on node-alpha succeeds.
	pod1 := makePod("pod-a1", "node-alpha", "default", nil, corev1.PodRunning)
	pod2 := makePod("pod-a2", "node-alpha", "default", nil, corev1.PodRunning)
	pod3 := makePod("pod-b1", "node-beta", "default", nil, corev1.PodRunning)
	client := fake.NewSimpleClientset(node1, node2, pod1, pod2, pod3)

	m := newTestModule(client, "")

	err := m.Run(context.Background())
	require.NoError(t, err)

	// All three pods must be gone.
	remaining := listPods(t, client, "default")
	assert.Empty(t, remaining)
}

func TestRun_DryRun_NoPodDeleted(t *testing.T) {
	node1 := makeNode("node-1", nil, false)
	pod1 := makePod("pod-1", "node-1", "default", nil, corev1.PodRunning)
	client := fake.NewSimpleClientset(node1, pod1)

	cfg, err := ParseConfig([]byte(`kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: once
  dryRun: true
  specs:
    strategy: delete
    readinessTimeout: 50ms
`))
	require.NoError(t, err)
	m := New(client, cfg)
	m.pollInterval = 10 * time.Millisecond

	err = m.Run(context.Background())
	require.NoError(t, err)

	remaining := listPods(t, client, "default")
	assert.Equal(t, []string{"pod-1"}, remaining)
}

func TestRun_GuardProtectsPod(t *testing.T) {
	node1 := makeNode("node-1", nil, false)
	podNormal := makePod("pod-normal", "node-1", "default", map[string]string{"app": "normal"}, corev1.PodRunning)
	podGuarded := makePod("pod-guarded", "node-1", "default", map[string]string{"app": "critical"}, corev1.PodRunning)
	client := fake.NewSimpleClientset(node1, podNormal, podGuarded)

	// pod-guarded stays Running after pod-normal is deleted, so recovery succeeds (1 >= 1).
	m := newTestModule(client, `  guard:
    matchers:
      labels:
        app: critical
`)

	err := m.Run(context.Background())
	require.NoError(t, err)

	remaining := listPods(t, client, "default")
	assert.Equal(t, []string{"pod-guarded"}, remaining, "guarded pod must survive")
}

func TestRun_NoNodes_Skipped(t *testing.T) {
	client := fake.NewSimpleClientset() // no nodes
	m := newTestModule(client, "")

	err := m.Run(context.Background())
	require.NoError(t, err) // graceful no-op
}

func TestRun_NodeWithNoPods_Skipped(t *testing.T) {
	node1 := makeNode("node-1", nil, false)
	client := fake.NewSimpleClientset(node1) // node exists but no pods in namespace

	m := newTestModule(client, "")

	err := m.Run(context.Background())
	require.NoError(t, err)
}

func TestRun_UnschedulableNodeSkipped(t *testing.T) {
	schedulable := makeNode("node-schedulable", nil, false)
	cordoned := makeNode("node-cordoned", nil, true) // Spec.Unschedulable = true
	podOnCordoned := makePod("pod-cordoned", "node-cordoned", "default", nil, corev1.PodRunning)
	client := fake.NewSimpleClientset(schedulable, cordoned, podOnCordoned)

	m := newTestModule(client, "")

	err := m.Run(context.Background())
	require.NoError(t, err)

	// Pod on the cordoned node must NOT be deleted — that node was excluded.
	remaining := listPods(t, client, "default")
	assert.Equal(t, []string{"pod-cordoned"}, remaining)
}

func TestRun_NodeSelector_FiltersNodes(t *testing.T) {
	workerNode := makeNode("node-worker", map[string]string{"role": "worker"}, false)
	controlNode := makeNode("node-control", map[string]string{"role": "control-plane"}, false)
	podWorker := makePod("pod-worker", "node-worker", "default", nil, corev1.PodRunning)
	podControl := makePod("pod-control", "node-control", "default", nil, corev1.PodRunning)
	client := fake.NewSimpleClientset(workerNode, controlNode, podWorker, podControl)

	m := newTestModule(client, `  specs:
    strategy: delete
    readinessTimeout: 50ms
    nodeSelector:
      role: worker
`)

	err := m.Run(context.Background())
	require.NoError(t, err)

	// Only pod on node-worker should be deleted; pod on node-control must survive.
	remaining := listPods(t, client, "default")
	assert.Equal(t, []string{"pod-control"}, remaining)
}

func TestWaitForRecovery_ReturnsWhenMinReadyMet(t *testing.T) {
	pod1 := makePod("pod-1", "node-1", "default", nil, corev1.PodRunning)
	client := fake.NewSimpleClientset(pod1)

	cfg := mustParseConfig(t, `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: once
  specs:
    strategy: delete
    readinessTimeout: 500ms
    minReady: 1
`)
	m := New(client, cfg)
	m.pollInterval = 10 * time.Millisecond

	err := m.waitForRecovery(context.Background(), "node-1")
	require.NoError(t, err)
}

func TestWaitForRecovery_TimesOut(t *testing.T) {
	client := fake.NewSimpleClientset() // no pods — Running count = 0

	cfg := mustParseConfig(t, `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: once
  specs:
    strategy: delete
    readinessTimeout: 50ms
    minReady: 1
`)
	m := New(client, cfg)
	m.pollInterval = 10 * time.Millisecond

	err := m.waitForRecovery(context.Background(), "node-1")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests and verify they pass**

```bash
go test -v -race ./pkg/nodedrain/...
```

Expected: all tests PASS. If any fail, fix the implementation before proceeding.

- [ ] **Step 3: Run full lint gate**

```bash
make check
```

Expected: all checks pass (tidy, fmt, vet, lint, test). Fix any `golangci-lint` findings — do not add `//nolint` tags.

---

## Task 4: Registration in main.go

**Files:**
- Modify: `cmd/chaos_zookoo/main.go`

---

- [ ] **Step 1: Add the import and builder entry**

In `cmd/chaos_zookoo/main.go`, add to the import block:

```go
"github.com/hhertout/chaos_zookoo/pkg/nodedrain"
```

And add to the `builders` map:

```go
var builders = map[string]module.Builder{
	"Killing":     killing.Build,
	"Rollout":     rollout.Build,
	"GorillaKill": gorillakill.Build,
	"NodeDrain":   nodedrain.Build,
}
```

- [ ] **Step 2: Verify the binary builds**

```bash
make build
```

Expected: `bin/chaos_zookoo` produced with no errors.

---

## Task 5: Example YAML

**Files:**
- Create: `examples/nodedrain.yaml`

---

- [ ] **Step 1: Create `examples/nodedrain.yaml`**

```yaml
# NodeDrain module — simulates a rolling cluster upgrade.
#
# Iterates over all targeted nodes in alphabetical order and kills every pod
# in the namespace that is scheduled on each node. After each node drain it
# waits for the namespace to recover (>= minReady Running pods) before moving
# to the next node. Pods protected by the `guard` block are never killed.
#
# This module models the disruption pattern of a managed Kubernetes upgrade
# (e.g. GKE, EKS, AKS) where worker nodes are recycled one at a time.

kind: NodeDrain

# Human-readable identifier for this module instance. Appears in every log
# line so you can distinguish multiple instances. Required.
name: simulate-upgrade

metadata:
  namespace: production

scenario:
  # Execution mode:
  #   - once:     run a single drain pass at startup.
  #   - periodic: repeat the full drain sequence on the given interval.
  when: once

  # Only required when `when: periodic`. `interval` and `cron` are mutually
  # exclusive — pick one.
  # interval: 6h
  # cron: "0 3 * * 1"   # every Monday at 03:00

  # Delay before the first (or only) execution.
  # For `when: periodic`, must be strictly less than `interval`.
  # Ignored with `cron`. Default: 0.
  # wait: 30s

  # If true, log what WOULD be killed without touching the API.
  # Produces identical output to a real run, minus the mutating call.
  # Default: false
  dryRun: false

  specs:
    # How pods are removed on each drained node:
    #   - evict  (default): uses the Eviction API — respects PodDisruptionBudgets.
    #   - delete: hard delete, bypasses PDBs. Use when the SA lacks eviction RBAC.
    strategy: evict

    # Label selector to restrict which nodes are included in the drain sequence.
    # If absent, all schedulable nodes in the cluster are targeted.
    # Cordoned nodes (spec.unschedulable: true) are always excluded.
    nodeSelector:
      kubernetes.io/role: worker

    # How long to wait for the namespace to reach `minReady` Running pods after
    # draining each node before continuing to the next one.
    # A timeout is a warning — the simulation continues regardless.
    # Default: 5m
    readinessTimeout: 5m

    # Minimum number of Running pods in the namespace required to consider a
    # node "absorbed" and safe to proceed to the next one.
    # Default: 1
    minReady: 2

  # Pods and workloads that must never be killed, regardless of which node they
  # are scheduled on. Optional — omit entirely if no exclusions are needed.
  # Uses the same matchers shape as other modules.
  guard:
    matchers:
      # 1) Label selector — all pods carrying these labels are excluded.
      labels:
        app: critical-infra

      # 2) Workload owners — resolves selector labels, then excludes matching pods.
      # deploymentName: monitoring-agent
      # daemonsetName:  node-exporter
      # statefulsetName: zookeeper

      # 3) Exact pod name — excludes a single pod.
      # podName: my-fixed-pod

# Optional post-run verification — same cross-cutting block available on all
# modules. After the full drain pass completes, the module defers a Prometheus
# query and records the result on the
# `chaos_test_success{name="simulate-upgrade", namespace="production"}` gauge.
#
# testing:
#   client: grafana
#   specs:
#     - datasourceKind: prometheus
#       datasourceId: my-ds-uid
#       query: sum(kube_pod_status_ready{namespace="production",condition="true"})
#       wait: 2m
#       timeWindow: 10m
#       operator: sup
#       threshold: 3
```

- [ ] **Step 2: Verify the binary still builds and tests still pass**

```bash
make check
```

Expected: all checks pass.

---

## Self-review notes

- **Spec coverage:** YAML API ✓, validation rules ✓, Run() behaviour ✓, struct layout ✓, registration ✓, example ✓, RBAC table (doc only, no code change needed) ✓, middleware compatibility ✓ (no code change needed).
- **`pollInterval` field:** unexported on `Module`, defaults to `5 * time.Second`, overridden to `10ms` in tests — keeps production behaviour while making tests fast.
- **`strategy: delete` in Run tests:** the fake client does not implement `EvictV1`; evict path is exercised via the `PodRemover` interface contract.
- **Node filtering in memory:** pod-by-nodeName and running-pod count both done via in-memory filtering after a namespace-wide `Pods.List` — avoids field selector limitations of the fake client.
- **Alphabetical node ordering:** `sort.Slice` in `listTargetNodes` guarantees deterministic drain order across runs.
