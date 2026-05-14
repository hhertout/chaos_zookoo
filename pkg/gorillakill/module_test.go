package gorillakill

import (
	"context"
	"testing"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
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

func pod(name string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: labels}}
}

func deployment(name, ns string, matchLabels map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: matchLabels}},
	}
}

func listPods(t *testing.T, client *fake.Clientset, ns string) []corev1.Pod {
	t.Helper()
	pods, err := client.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	return pods.Items
}

const onceLabelsYAML = `kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: once
  matchers:
    labels:
      app: test
`

// --- Config tests ---

func TestParseConfig_RequiresMatcher(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: once
`))
	assert.Error(t, err)
}

func TestParseConfig_RequiresNamespace(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
scenario:
  when: once
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RequiresWhen(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsInvalidWhen(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: sometimes
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_PeriodicRequiresInterval(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: periodic
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_OnceWorks(t *testing.T) {
	cfg := mustParseConfig(t, onceLabelsYAML)
	assert.Equal(t, WhenOnce, cfg.Scenario.When)
}

func TestParseConfig_OnceRejectsInterval(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: once
  interval: 30s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_OnceRejectsCron(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: once
  cron: "* * * * *"
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_PeriodicParsesInterval(t *testing.T) {
	cfg := mustParseConfig(t, `kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: periodic
  interval: 30s
  matchers:
    labels:
      app: test
`)
	assert.Equal(t, WhenPeriodic, cfg.Scenario.When)
	assert.Equal(t, 30*time.Second, cfg.interval)
}

// --- Schedule tests ---

func TestSchedule_Once(t *testing.T) {
	cfg := mustParseConfig(t, onceLabelsYAML)
	m := New(fake.NewSimpleClientset(), cfg)
	assert.Equal(t, module.ScheduleOnce, m.Schedule().Mode)
}

func TestSchedule_Periodic(t *testing.T) {
	cfg := mustParseConfig(t, `kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: periodic
  interval: 1m
  matchers:
    labels:
      app: test
`)
	m := New(fake.NewSimpleClientset(), cfg)
	sched := m.Schedule()
	assert.Equal(t, module.SchedulePeriodic, sched.Mode)
	assert.Equal(t, time.Minute, sched.Interval)
}

// --- Run tests ---

func TestRun_NoPods(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := mustParseConfig(t, onceLabelsYAML)
	m := New(client, cfg)

	assert.NoError(t, m.Run(context.Background()))
}

func TestRun_KillsAllMatchingPods(t *testing.T) {
	labels := map[string]string{"app": "test"}
	client := fake.NewSimpleClientset(
		pod("pod-1", labels),
		pod("pod-2", labels),
		pod("pod-3", labels),
		pod("other", map[string]string{"app": "other"}),
	)

	cfg := mustParseConfig(t, onceLabelsYAML)
	m := New(client, cfg)

	require.NoError(t, m.Run(context.Background()))
	remaining := listPods(t, client, "default")
	assert.Len(t, remaining, 1, "only the non-matching pod should survive")
	assert.Equal(t, "other", remaining[0].Name)
}

func TestRun_KillsAllByDeployment(t *testing.T) {
	labels := map[string]string{"app": "deploy-app"}
	client := fake.NewSimpleClientset(
		deployment("my-deploy", "default", labels),
		pod("deploy-pod-1", labels),
		pod("deploy-pod-2", labels),
		pod("deploy-pod-3", labels),
	)

	cfg := mustParseConfig(t, `kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: once
  matchers:
    deploymentName: my-deploy
`)
	m := New(client, cfg)

	require.NoError(t, m.Run(context.Background()))
	assert.Empty(t, listPods(t, client, "default"), "all deployment pods should have been killed")
}

func TestName(t *testing.T) {
	cfg := mustParseConfig(t, `kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: once
  matchers:
    labels:
      a: b
`)
	m := New(fake.NewSimpleClientset(), cfg)
	assert.Equal(t, "test", m.Name())
}

// --- Config validation edge cases ---

func TestParseConfig_RequiresName(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  namespace: default
scenario:
  when: once
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsWhitespaceOnlyName(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: "   "
  namespace: default
scenario:
  when: once
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_PeriodicRejectsZeroInterval(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: periodic
  interval: 0s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_PeriodicRejectsNegativeInterval(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: periodic
  interval: -30s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_PeriodicRejectsInvalidIntervalFormat(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: periodic
  interval: "bad"
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_PeriodicRejectsIntervalAndCronTogether(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: periodic
  interval: 60s
  cron: "* * * * *"
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_PeriodicAcceptsCronSchedule(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: periodic
  cron: "*/5 * * * *"
  matchers:
    labels:
      app: test
`))
	assert.NoError(t, err)
}

func TestParseConfig_PeriodicRejectsInvalidCronExpression(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: periodic
  cron: "not-a-cron"
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_PeriodicRejectsWaitWithCron(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: periodic
  cron: "*/5 * * * *"
  wait: 30s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_PeriodicRejectsWaitGreaterThanInterval(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: periodic
  interval: 60s
  wait: 90s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_OnceAcceptsWait(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: GorillaKill
metadata:
  name: test
  namespace: default
scenario:
  when: once
  wait: 30s
  matchers:
    labels:
      app: test
`))
	assert.NoError(t, err)
}
