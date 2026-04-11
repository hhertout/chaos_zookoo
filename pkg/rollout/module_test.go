package rollout

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func mustParseConfig(t *testing.T, yaml string) Config {
	t.Helper()
	cfg, err := ParseConfig([]byte(yaml))
	require.NoError(t, err)
	return cfg
}

func assertPatchAction(t *testing.T, client *fake.Clientset, resource string) {
	t.Helper()
	for _, a := range client.Actions() {
		if a.GetVerb() == "patch" && a.GetResource().Resource == resource {
			return
		}
	}
	t.Errorf("expected a patch action on %s", resource)
}

// --- Config tests ---

func TestParseConfig_RequiresResourceMatcher(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
name: test
metadata:
  namespace: default
scenario:
  interval: 1h
`))
	assert.Error(t, err)
}

func TestParseConfig_RequiresNamespace(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
name: test
scenario:
  interval: 1h
  matchers:
    deploymentName: my-deploy
`))
	assert.Error(t, err)
}

func TestParseConfig_AcceptsAllMatcherTypes(t *testing.T) {
	const tmpl = `kind: Rollout
name: test
metadata:
  namespace: ns
scenario:
  interval: 1h
  matchers:
%s`
	tests := []struct {
		name    string
		matcher string
	}{
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

func workloadCfg(workload, name string) string {
	return fmt.Sprintf(`kind: Rollout
name: test
metadata:
  namespace: default
scenario:
  interval: 1h
  matchers:
    %s: %s
`, workload, name)
}

func TestRun_PatchesDeployment(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-deploy", Namespace: "default"},
	})

	cfg := mustParseConfig(t, workloadCfg("deploymentName", "my-deploy"))
	m := New(client, cfg)

	assert.NoError(t, m.Run(context.Background()))
	assertPatchAction(t, client, "deployments")
}

func TestRun_PatchesDaemonset(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-ds", Namespace: "default"},
	})

	cfg := mustParseConfig(t, workloadCfg("daemonsetName", "my-ds"))
	m := New(client, cfg)

	assert.NoError(t, m.Run(context.Background()))
	assertPatchAction(t, client, "daemonsets")
}

func TestRun_PatchesStatefulset(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-sts", Namespace: "default"},
	})

	cfg := mustParseConfig(t, workloadCfg("statefulsetName", "my-sts"))
	m := New(client, cfg)

	assert.NoError(t, m.Run(context.Background()))
	assertPatchAction(t, client, "statefulsets")
}

func TestRun_NonExistentDeployment(t *testing.T) {
	client := fake.NewSimpleClientset()

	cfg := mustParseConfig(t, workloadCfg("deploymentName", "does-not-exist"))
	m := New(client, cfg)

	assert.Error(t, m.Run(context.Background()))
}

func TestName(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := mustParseConfig(t, workloadCfg("deploymentName", "x"))
	m := New(client, cfg)
	assert.Equal(t, "test", m.Name())
}

// --- Config validation edge cases ---

func TestParseConfig_RequiresName(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
metadata:
  namespace: default
scenario:
  interval: 1h
  matchers:
    deploymentName: my-deploy
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsWhitespaceOnlyName(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
name: "   "
metadata:
  namespace: default
scenario:
  interval: 1h
  matchers:
    deploymentName: my-deploy
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsZeroInterval(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
name: test
metadata:
  namespace: default
scenario:
  interval: 0s
  matchers:
    deploymentName: my-deploy
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsNegativeInterval(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
name: test
metadata:
  namespace: default
scenario:
  interval: -1h
  matchers:
    deploymentName: my-deploy
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsInvalidIntervalFormat(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
name: test
metadata:
  namespace: default
scenario:
  interval: "bad"
  matchers:
    deploymentName: my-deploy
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsIntervalAndCronTogether(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
name: test
metadata:
  namespace: default
scenario:
  interval: 1h
  cron: "* * * * *"
  matchers:
    deploymentName: my-deploy
`))
	assert.Error(t, err)
}

func TestParseConfig_AcceptsCronSchedule(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
name: test
metadata:
  namespace: default
scenario:
  cron: "*/30 * * * *"
  matchers:
    deploymentName: my-deploy
`))
	assert.NoError(t, err)
}

func TestParseConfig_RejectsInvalidCronExpression(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
name: test
metadata:
  namespace: default
scenario:
  cron: "bad-cron"
  matchers:
    deploymentName: my-deploy
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsWaitWithCron(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
name: test
metadata:
  namespace: default
scenario:
  cron: "*/30 * * * *"
  wait: 5m
  matchers:
    deploymentName: my-deploy
`))
	assert.Error(t, err)
}

func TestParseConfig_RejectsWaitGreaterThanInterval(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
name: test
metadata:
  namespace: default
scenario:
  interval: 1h
  wait: 2h
  matchers:
    deploymentName: my-deploy
`))
	assert.Error(t, err)
}

func TestParseConfig_AcceptsValidWait(t *testing.T) {
	_, err := ParseConfig([]byte(`kind: Rollout
name: test
metadata:
  namespace: default
scenario:
  interval: 1h
  wait: 5m
  matchers:
    deploymentName: my-deploy
`))
	assert.NoError(t, err)
}
