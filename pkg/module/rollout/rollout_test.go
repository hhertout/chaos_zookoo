package rollout

import (
	"context"
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

func TestParseConfig_RequiresResourceMatcher(t *testing.T) {
	_, err := ParseConfig([]byte("namespace: default\ninterval: 1h\n"))
	assert.Error(t, err)
}

func TestParseConfig_RequiresNamespace(t *testing.T) {
	_, err := ParseConfig([]byte("interval: 1h\nmatchers:\n  deploymentName: my-deploy\n"))
	assert.Error(t, err)
}

func TestParseConfig_AcceptsAllMatcherTypes(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"deploymentName", "namespace: ns\ninterval: 1h\nmatchers:\n  deploymentName: my-deploy\n"},
		{"daemonsetName", "namespace: ns\ninterval: 1h\nmatchers:\n  daemonsetName: my-ds\n"},
		{"statefulsetName", "namespace: ns\ninterval: 1h\nmatchers:\n  statefulsetName: my-sts\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tt.yaml))
			assert.NoError(t, err)
		})
	}
}

func TestRun_PatchesDeployment(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy",
			Namespace: "default",
		},
	}
	client := fake.NewSimpleClientset(deploy)

	cfg := mustParseConfig(t, "namespace: default\ninterval: 1h\nmatchers:\n  deploymentName: my-deploy\n")
	m := New(client, cfg)

	err := m.Run(context.Background())
	assert.NoError(t, err)

	assertPatchAction(t, client, "deployments")
}

func TestRun_PatchesDaemonset(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ds",
			Namespace: "default",
		},
	}
	client := fake.NewSimpleClientset(ds)

	cfg := mustParseConfig(t, "namespace: default\ninterval: 1h\nmatchers:\n  daemonsetName: my-ds\n")
	m := New(client, cfg)

	err := m.Run(context.Background())
	assert.NoError(t, err)

	assertPatchAction(t, client, "daemonsets")
}

func TestRun_PatchesStatefulset(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-sts",
			Namespace: "default",
		},
	}
	client := fake.NewSimpleClientset(sts)

	cfg := mustParseConfig(t, "namespace: default\ninterval: 1h\nmatchers:\n  statefulsetName: my-sts\n")
	m := New(client, cfg)

	err := m.Run(context.Background())
	assert.NoError(t, err)

	assertPatchAction(t, client, "statefulsets")
}

func TestRun_NonExistentDeployment(t *testing.T) {
	client := fake.NewSimpleClientset()

	cfg := mustParseConfig(t, "namespace: default\ninterval: 1h\nmatchers:\n  deploymentName: does-not-exist\n")
	m := New(client, cfg)

	err := m.Run(context.Background())
	assert.Error(t, err)
}

func TestName(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := mustParseConfig(t, "namespace: default\ninterval: 1h\nmatchers:\n  deploymentName: x\n")
	m := New(client, cfg)
	assert.Equal(t, "rollout", m.Name())
}

func assertPatchAction(t *testing.T, client *fake.Clientset, resource string) {
	t.Helper()
	found := false
	for _, a := range client.Actions() {
		if a.GetVerb() == "patch" && a.GetResource().Resource == resource {
			found = true
		}
	}
	assert.True(t, found, "expected a patch action on %s", resource)
}
