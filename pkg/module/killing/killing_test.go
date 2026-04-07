package killing

import (
	"context"
	"testing"

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

func TestParseConfig_RequiresMatcher(t *testing.T) {
	_, err := ParseConfig([]byte("namespace: default\ninterval: 60s\n"))
	assert.Error(t, err)
}

func TestParseConfig_RequiresNamespace(t *testing.T) {
	_, err := ParseConfig([]byte("interval: 60s\nmatchers:\n  labels:\n    app: test\n"))
	assert.Error(t, err)
}

func TestParseConfig_DefaultMinAvailable(t *testing.T) {
	cfg := mustParseConfig(t, "namespace: default\ninterval: 60s\nmatchers:\n  labels:\n    app: test\n")
	assert.Equal(t, 1, cfg.MinAvailable)
}

func TestParseConfig_AcceptsAllMatcherTypes(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"labels", "namespace: ns\ninterval: 60s\nmatchers:\n  labels:\n    app: test\n"},
		{"podName", "namespace: ns\ninterval: 60s\nmatchers:\n  podName: my-pod\n"},
		{"deploymentName", "namespace: ns\ninterval: 60s\nmatchers:\n  deploymentName: my-deploy\n"},
		{"daemonsetName", "namespace: ns\ninterval: 60s\nmatchers:\n  daemonsetName: my-ds\n"},
		{"statefulsetName", "namespace: ns\ninterval: 60s\nmatchers:\n  statefulsetName: my-sts\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tt.yaml))
			assert.NoError(t, err)
		})
	}
}

func TestRun_NoPods(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := mustParseConfig(t, "namespace: default\ninterval: 60s\nmatchers:\n  labels:\n    app: test\n")
	m := New(client, cfg)

	err := m.Run(context.Background())
	assert.NoError(t, err)
}

func TestRun_KillsPodByLabels(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-1",
				Namespace: "default",
				Labels:    map[string]string{"app": "test"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-2",
				Namespace: "default",
				Labels:    map[string]string{"app": "test"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-3",
				Namespace: "default",
				Labels:    map[string]string{"app": "test"},
			},
		},
	)

	cfg := mustParseConfig(t, "namespace: default\nminAvailable: 1\ninterval: 60s\nmatchers:\n  labels:\n    app: test\n")
	m := New(client, cfg)

	err := m.Run(context.Background())
	require.NoError(t, err)

	pods, err := client.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, len(pods.Items), "one pod should have been killed")
}

func TestRun_KillsPodByPodName(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "target-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "test"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "extra-pod",
				Namespace: "default",
			},
		},
	)

	cfg := mustParseConfig(t, "namespace: default\nminAvailable: 1\ninterval: 60s\nmatchers:\n  podName: extra-pod\n  labels:\n    app: test\n")
	m := New(client, cfg)

	err := m.Run(context.Background())
	require.NoError(t, err)

	pods, err := client.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, len(pods.Items), "one pod should have been killed from the combined pool")
}

func TestRun_KillsPodByDeployment(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-deploy",
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "deploy-app"},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "deploy-pod-1",
				Namespace: "default",
				Labels:    map[string]string{"app": "deploy-app"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "deploy-pod-2",
				Namespace: "default",
				Labels:    map[string]string{"app": "deploy-app"},
			},
		},
	)

	cfg := mustParseConfig(t, "namespace: default\nminAvailable: 1\ninterval: 60s\nmatchers:\n  deploymentName: my-deploy\n")
	m := New(client, cfg)

	err := m.Run(context.Background())
	require.NoError(t, err)

	pods, err := client.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, len(pods.Items), "one pod should have been killed")
}

func TestRun_KillsPodByDaemonset(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-ds",
				Namespace: "default",
			},
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "ds-app"},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ds-pod-1",
				Namespace: "default",
				Labels:    map[string]string{"app": "ds-app"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ds-pod-2",
				Namespace: "default",
				Labels:    map[string]string{"app": "ds-app"},
			},
		},
	)

	cfg := mustParseConfig(t, "namespace: default\nminAvailable: 1\ninterval: 60s\nmatchers:\n  daemonsetName: my-ds\n")
	m := New(client, cfg)

	err := m.Run(context.Background())
	require.NoError(t, err)

	pods, err := client.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, len(pods.Items), "one pod should have been killed")
}

func TestRun_KillsPodByStatefulset(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-sts",
				Namespace: "default",
			},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "sts-app"},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sts-pod-1",
				Namespace: "default",
				Labels:    map[string]string{"app": "sts-app"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sts-pod-2",
				Namespace: "default",
				Labels:    map[string]string{"app": "sts-app"},
			},
		},
	)

	cfg := mustParseConfig(t, "namespace: default\nminAvailable: 1\ninterval: 60s\nmatchers:\n  statefulsetName: my-sts\n")
	m := New(client, cfg)

	err := m.Run(context.Background())
	require.NoError(t, err)

	pods, err := client.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, len(pods.Items), "one pod should have been killed")
}

func TestRun_RespectsMinAvailable(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-1",
				Namespace: "default",
				Labels:    map[string]string{"app": "test"},
			},
		},
	)

	cfg := mustParseConfig(t, "namespace: default\nminAvailable: 1\ninterval: 60s\nmatchers:\n  labels:\n    app: test\n")
	m := New(client, cfg)

	err := m.Run(context.Background())
	require.NoError(t, err)

	pods, err := client.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, len(pods.Items), "pod should not have been killed")
}

func TestBuildLabelSelector(t *testing.T) {
	s := buildLabelSelector(map[string]string{"app": "test"})
	assert.Equal(t, "app=test", s)
}

func TestName(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := mustParseConfig(t, "namespace: default\ninterval: 60s\nmatchers:\n  labels:\n    a: b\n")
	m := New(client, cfg)
	assert.Equal(t, "killing", m.Name())
}
