package killing

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
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

func daemonset(name, ns string, matchLabels map[string]string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: matchLabels}},
	}
}

func statefulset(name, ns string, matchLabels map[string]string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       appsv1.StatefulSetSpec{Selector: &metav1.LabelSelector{MatchLabels: matchLabels}},
	}
}

func listPods(t *testing.T, client *fake.Clientset) []corev1.Pod {
	t.Helper()
	pods, err := client.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	return pods.Items
}

// --- Config tests ---

const baseLabelsYAML = `kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  strategy: delete
  matchers:
    labels:
      app: test
`

func TestParseConfig_RequiresMatcher(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
`))
	assert.Error(t, err)
}

func TestParseConfig_RequiresNamespace(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
scenario:
  interval: 60s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_DefaultMinAvailableIsZero(t *testing.T) {
	cfg := mustParseConfig(t, baseLabelsYAML)
	assert.Equal(t, 0, cfg.Scenario.MinAvailable)
}

func TestParseConfig_RejectsNegativeMinAvailable(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  minAvailable: -1
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_AcceptsAllMatcherTypes(t *testing.T) {
	const tmpl = `kind: Killing
metadata:
  name: test
  namespace: ns
scenario:
  interval: 60s
  strategy: delete
  matchers:
%s`
	tests := []struct {
		name    string
		matcher string
	}{
		{"labels", "    labels:\n      app: test\n"},
		{"podName", "    podName: my-pod\n"},
		{"deploymentName", "    deploymentName: my-deploy\n"},
		{"daemonsetName", "    daemonsetName: my-ds\n"},
		{"statefulsetName", "    statefulsetName: my-sts\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(fmt.Sprintf(tmpl, tt.matcher)))
			assert.NoError(t, err)
		})
	}
}

// --- Run tests ---

func TestRun_NoPods(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := mustParseConfig(t, baseLabelsYAML)
	m := New(client, cfg)

	assert.NoError(t, m.Run(context.Background()))
}

const labelsMinAvailable1YAML = `kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  strategy: delete
  minAvailable: 1
  matchers:
    labels:
      app: test
`

func TestRun_KillsPodByLabels(t *testing.T) {
	labels := map[string]string{"app": "test"}
	client := fake.NewSimpleClientset(
		pod("pod-1", labels),
		pod("pod-2", labels),
		pod("pod-3", labels),
	)

	cfg := mustParseConfig(t, labelsMinAvailable1YAML)
	m := New(client, cfg)

	require.NoError(t, m.Run(context.Background()))
	assert.Len(t, listPods(t, client), 2, "one pod should have been killed")
}

func TestRun_KillsPodByPodName(t *testing.T) {
	client := fake.NewSimpleClientset(
		pod("target-pod", map[string]string{"app": "test"}),
		pod("extra-pod", nil),
	)

	cfg := mustParseConfig(t, `kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  strategy: delete
  minAvailable: 1
  matchers:
    podName: extra-pod
    labels:
      app: test
`)
	m := New(client, cfg)

	require.NoError(t, m.Run(context.Background()))
	assert.Len(t, listPods(t, client), 1, "one pod should have been killed from the combined pool")
}

func TestRun_KillsPodByDeployment(t *testing.T) {
	labels := map[string]string{"app": "deploy-app"}
	client := fake.NewSimpleClientset(
		deployment("my-deploy", "default", labels),
		pod("deploy-pod-1", labels),
		pod("deploy-pod-2", labels),
	)

	cfg := mustParseConfig(t, `kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  strategy: delete
  minAvailable: 1
  matchers:
    deploymentName: my-deploy
`)
	m := New(client, cfg)

	require.NoError(t, m.Run(context.Background()))
	assert.Len(t, listPods(t, client), 1, "one pod should have been killed")
}

func TestRun_KillsPodByDaemonset(t *testing.T) {
	labels := map[string]string{"app": "ds-app"}
	client := fake.NewSimpleClientset(
		daemonset("my-ds", "default", labels),
		pod("ds-pod-1", labels),
		pod("ds-pod-2", labels),
	)

	cfg := mustParseConfig(t, `kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  strategy: delete
  minAvailable: 1
  matchers:
    daemonsetName: my-ds
`)
	m := New(client, cfg)

	require.NoError(t, m.Run(context.Background()))
	assert.Len(t, listPods(t, client), 1, "one pod should have been killed")
}

func TestRun_KillsPodByStatefulset(t *testing.T) {
	labels := map[string]string{"app": "sts-app"}
	client := fake.NewSimpleClientset(
		statefulset("my-sts", "default", labels),
		pod("sts-pod-1", labels),
		pod("sts-pod-2", labels),
	)

	cfg := mustParseConfig(t, `kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  strategy: delete
  minAvailable: 1
  matchers:
    statefulsetName: my-sts
`)
	m := New(client, cfg)

	require.NoError(t, m.Run(context.Background()))
	assert.Len(t, listPods(t, client), 1, "one pod should have been killed")
}

func TestRun_RespectsMinAvailable(t *testing.T) {
	client := fake.NewSimpleClientset(
		pod("pod-1", map[string]string{"app": "test"}),
	)

	cfg := mustParseConfig(t, labelsMinAvailable1YAML)
	m := New(client, cfg)

	require.NoError(t, m.Run(context.Background()))
	assert.Len(t, listPods(t, client), 1, "pod should not have been killed")
}

func TestParseConfig_DefaultStrategyIsEvict(t *testing.T) {
	cfg := mustParseConfig(t, `kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  matchers:
    labels:
      app: test
`)
	assert.Equal(t, StrategyEvict, cfg.Scenario.Strategy)
}

func TestParseConfig_RejectsInvalidStrategy(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  strategy: nuke
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestRun_DryRunSkipsDeletion(t *testing.T) {
	labels := map[string]string{"app": "test"}
	client := fake.NewSimpleClientset(
		pod("pod-1", labels),
		pod("pod-2", labels),
	)

	cfg := mustParseConfig(t, `kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  minAvailable: 1
  dryRun: true
  strategy: delete
  matchers:
    labels:
      app: test
`)
	m := New(client, cfg)

	require.NoError(t, m.Run(context.Background()))
	assert.Len(t, listPods(t, client), 2, "dryRun must not delete")

	for _, a := range client.Actions() {
		assert.NotEqual(t, "delete", a.GetVerb(), "no delete action expected in dryRun")
	}
}

func TestRun_EvictStrategyIssuesEviction(t *testing.T) {
	labels := map[string]string{"app": "test"}
	client := fake.NewSimpleClientset(
		pod("pod-1", labels),
		pod("pod-2", labels),
	)

	cfg := mustParseConfig(t, `kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  minAvailable: 1
  matchers:
    labels:
      app: test
`)
	m := New(client, cfg)

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

func TestRun_ForceDeleteStrategyIssuesForceDelete(t *testing.T) {
	labels := map[string]string{"app": "test"}
	client := fake.NewSimpleClientset(
		pod("pod-1", labels),
		pod("pod-2", labels),
	)

	cfg := mustParseConfig(t, `kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  minAvailable: 1
  strategy: force-delete
  matchers:
    labels:
      app: test
`)
	m := New(client, cfg)

	require.NoError(t, m.Run(context.Background()))

	var sawForceDelete bool
	for _, a := range client.Actions() {
		da, ok := a.(ktesting.DeleteAction)
		if ok && da.GetVerb() == "delete" && da.GetResource().Resource == "pods" {
			if da.GetDeleteOptions().GracePeriodSeconds != nil && *da.GetDeleteOptions().GracePeriodSeconds == 0 {
				sawForceDelete = true
				break
			}
		}
	}
	assert.True(t, sawForceDelete, "a force-delete (GracePeriodSeconds=0) should have been issued")
}

func TestName(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := mustParseConfig(t, `kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  strategy: delete
  matchers:
    labels:
      a: b
`)
	m := New(client, cfg)
	assert.Equal(t, "test", m.Name())
}

// --- Config validation edge cases ---

func TestParseConfig_RequiresName(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  namespace: default
scenario:
  interval: 60s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsWhitespaceOnlyName(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: "   "
  namespace: default
scenario:
  interval: 60s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsZeroInterval(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 0s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsNegativeInterval(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: -10s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsInvalidIntervalFormat(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: "not-a-duration"
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsIntervalAndCronTogether(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  cron: "* * * * *"
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_AcceptsCronSchedule(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  cron: "*/5 * * * *"
  matchers:
    labels:
      app: test
`))
	assert.NoError(t, err)
}

func TestParseConfig_RejectsInvalidCronExpression(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  cron: "not-a-cron"
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsWaitWithCron(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  cron: "*/5 * * * *"
  wait: 30s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsWaitEqualToInterval(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  wait: 60s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsWaitGreaterThanInterval(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  wait: 90s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsNegativeWait(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  wait: -5s
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsInvalidWaitFormat(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  wait: "bad"
  matchers:
    labels:
      app: test
`))
	assert.Error(t, err)
}

func TestParseConfig_AcceptsValidWait(t *testing.T) {
	cfg, err := ParseConfig([]byte(`kind: Killing
metadata:
  name: test
  namespace: default
scenario:
  interval: 60s
  wait: 30s
  matchers:
    labels:
      app: test
`))
	assert.NoError(t, err)
	assert.Equal(t, cfg.Wait().Seconds(), float64(30))
}
