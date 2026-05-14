package nodedrain

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
			name: "periodic accepts cron schedule",
			yaml: `kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: periodic
  cron: "*/5 * * * *"
`,
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
			name:    "rejects invalid strategy",
			yaml:    minimalOnce + "  specs:\n    strategy: nuke\n",
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
			name:    "rejects negative minReady",
			yaml:    minimalOnce + "  specs:\n    minReady: -2\n",
			wantErr: true,
		},
		{
			name:    "minReady defaults to 0",
			yaml:    minimalOnce,
			wantErr: false,
			check: func(t *testing.T, cfg Config) {
				t.Helper()
				assert.Equal(t, 0, cfg.Scenario.Specs.MinReady)
			},
		},
		{
			name:    "minReady 0 is valid",
			yaml:    minimalOnce + "  specs:\n    minReady: 0\n",
			wantErr: false,
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
			if tt.wantErr {
				_, err := ParseConfig([]byte(tt.yaml))
				require.Error(t, err)
				return
			}
			cfg := mustParseConfig(t, tt.yaml)
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

// --- Helpers ---

func makeNode(name string, labels map[string]string, unschedulable bool) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec:       corev1.NodeSpec{Unschedulable: unschedulable},
	}
}

func makePod(name, nodeName string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: labels},
		Spec:       corev1.PodSpec{NodeName: nodeName},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func listPodNames(t *testing.T, client *fake.Clientset) []string {
	t.Helper()
	pods, err := client.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
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
	// Two pods on node-alpha, one on node-beta. All Running so recovery on node-alpha succeeds
	// (node-beta pod still present), then node-beta drains and hits the 50ms timeout gracefully.
	pod1 := makePod("pod-a1", "node-alpha", nil)
	pod2 := makePod("pod-a2", "node-alpha", nil)
	pod3 := makePod("pod-b1", "node-beta", nil)
	client := fake.NewSimpleClientset(node1, node2, pod1, pod2, pod3)

	m := newTestModule(client, "")

	err := m.Run(context.Background())
	require.NoError(t, err)

	remaining := listPodNames(t, client)
	assert.Empty(t, remaining)
}

func TestRun_DryRun_NoPodDeleted(t *testing.T) {
	node1 := makeNode("node-1", nil, false)
	pod1 := makePod("pod-1", "node-1", nil)
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

	remaining := listPodNames(t, client)
	assert.Equal(t, []string{"pod-1"}, remaining)
}

func TestRun_GuardProtectsPod(t *testing.T) {
	node1 := makeNode("node-1", nil, false)
	podNormal := makePod("pod-normal", "node-1", map[string]string{"app": "normal"})
	podGuarded := makePod("pod-guarded", "node-1", map[string]string{"app": "critical"})
	client := fake.NewSimpleClientset(node1, podNormal, podGuarded)

	// pod-guarded stays Running after pod-normal is deleted, so recovery succeeds (1 >= 1).
	m := newTestModule(client, `  guard:
    matchers:
      labels:
        app: critical
`)

	err := m.Run(context.Background())
	require.NoError(t, err)

	remaining := listPodNames(t, client)
	assert.Equal(t, []string{"pod-guarded"}, remaining, "guarded pod must survive")
}

func TestRun_NoNodes_Skipped(t *testing.T) {
	client := fake.NewSimpleClientset()
	m := newTestModule(client, "")

	err := m.Run(context.Background())
	require.NoError(t, err)
}

func TestRun_NodeWithNoPods_Skipped(t *testing.T) {
	node1 := makeNode("node-1", nil, false)
	client := fake.NewSimpleClientset(node1)

	m := newTestModule(client, "")

	err := m.Run(context.Background())
	require.NoError(t, err)
}

func TestRun_UnschedulableNodeSkipped(t *testing.T) {
	schedulable := makeNode("node-schedulable", nil, false)
	cordoned := makeNode("node-cordoned", nil, true)
	podOnCordoned := makePod("pod-cordoned", "node-cordoned", nil)
	client := fake.NewSimpleClientset(schedulable, cordoned, podOnCordoned)

	m := newTestModule(client, "")

	err := m.Run(context.Background())
	require.NoError(t, err)

	remaining := listPodNames(t, client)
	assert.Equal(t, []string{"pod-cordoned"}, remaining)
}

func TestRun_NodeSelector_FiltersNodes(t *testing.T) {
	workerNode := makeNode("node-worker", map[string]string{"role": "worker"}, false)
	controlNode := makeNode("node-control", map[string]string{"role": "control-plane"}, false)
	podWorker := makePod("pod-worker", "node-worker", nil)
	podControl := makePod("pod-control", "node-control", nil)
	client := fake.NewSimpleClientset(workerNode, controlNode, podWorker, podControl)

	cfg, err := ParseConfig([]byte(`kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: once
  specs:
    strategy: delete
    readinessTimeout: 50ms
    minReady: 1
    nodeSelector:
      role: worker
`))
	require.NoError(t, err)
	m := New(client, cfg)
	m.pollInterval = 10 * time.Millisecond

	err = m.Run(context.Background())
	require.NoError(t, err)

	remaining := listPodNames(t, client)
	assert.Equal(t, []string{"pod-control"}, remaining)
}

func TestNewPodRemover_ReturnsCorrectType(t *testing.T) {
	client := fake.NewSimpleClientset()
	_, evict := newPodRemover(client, StrategyEvict).(evictRemover)
	assert.True(t, evict, "StrategyEvict should produce an evictRemover")
	_, del := newPodRemover(client, StrategyDelete).(deleteRemover)
	assert.True(t, del, "StrategyDelete should produce a deleteRemover")
}

func TestRun_EvictStrategyIssuesEviction(t *testing.T) {
	node1 := makeNode("node-1", nil, false)
	pod1 := makePod("pod-1", "node-1", nil)
	client := fake.NewSimpleClientset(node1, pod1)

	cfg, err := ParseConfig([]byte(`kind: NodeDrain
name: test
metadata:
  namespace: default
scenario:
  when: once
  specs:
    strategy: evict
    readinessTimeout: 50ms
`))
	require.NoError(t, err)
	m := New(client, cfg)
	m.pollInterval = 10 * time.Millisecond

	require.NoError(t, m.Run(context.Background()))

	var sawEviction bool
	for _, a := range client.Actions() {
		if a.GetVerb() == "create" && a.GetSubresource() == "eviction" {
			sawEviction = true
			break
		}
	}
	assert.True(t, sawEviction, "an eviction should have been issued")
}

func TestListTargetNodes_AlphabeticalOrder(t *testing.T) {
	nodeZ := makeNode("node-z", nil, false)
	nodeA := makeNode("node-a", nil, false)
	nodeM := makeNode("node-m", nil, false)
	client := fake.NewSimpleClientset(nodeZ, nodeA, nodeM)

	m := newTestModule(client, "")
	nodes, err := m.listTargetNodes(context.Background())
	require.NoError(t, err)

	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Name
	}
	assert.Equal(t, []string{"node-a", "node-m", "node-z"}, names)
}

func TestWaitForRecovery_ReturnsWhenMinReadyMet(t *testing.T) {
	pod1 := makePod("pod-1", "node-1", nil)
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
	client := fake.NewSimpleClientset()

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
