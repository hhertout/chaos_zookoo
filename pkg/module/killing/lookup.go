package killing

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (m *Module) collectPods(ctx context.Context) ([]corev1.Pod, error) {
	seen := make(map[string]struct{})
	var result []corev1.Pod

	addUnique := func(pods []corev1.Pod) {
		for _, p := range pods {
			if _, ok := seen[p.Name]; !ok {
				seen[p.Name] = struct{}{}
				result = append(result, p)
			}
		}
	}

	if len(m.matchers.Labels) > 0 {
		selector := buildLabelSelector(m.matchers.Labels)
		pods, err := m.client.CoreV1().Pods(m.namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil {
			return nil, fmt.Errorf("listing pods by labels: %w", err)
		}
		addUnique(pods.Items)
	}

	if m.matchers.PodName != "" {
		pod, err := m.client.CoreV1().Pods(m.namespace).Get(ctx, m.matchers.PodName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("getting pod %s: %w", m.matchers.PodName, err)
		}
		addUnique([]corev1.Pod{*pod})
	}

	if m.matchers.DeploymentName != "" {
		deploy, err := m.client.AppsV1().Deployments(m.namespace).Get(ctx, m.matchers.DeploymentName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("getting deployment %s: %w", m.matchers.DeploymentName, err)
		}
		selector := buildLabelSelector(deploy.Spec.Selector.MatchLabels)
		pods, err := m.client.CoreV1().Pods(m.namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil {
			return nil, fmt.Errorf("listing pods for deployment %s: %w", m.matchers.DeploymentName, err)
		}
		addUnique(pods.Items)
	}

	if m.matchers.DaemonsetName != "" {
		ds, err := m.client.AppsV1().DaemonSets(m.namespace).Get(ctx, m.matchers.DaemonsetName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("getting daemonset %s: %w", m.matchers.DaemonsetName, err)
		}
		selector := buildLabelSelector(ds.Spec.Selector.MatchLabels)
		pods, err := m.client.CoreV1().Pods(m.namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil {
			return nil, fmt.Errorf("listing pods for daemonset %s: %w", m.matchers.DaemonsetName, err)
		}
		addUnique(pods.Items)
	}

	if m.matchers.StatefulsetName != "" {
		sts, err := m.client.AppsV1().StatefulSets(m.namespace).Get(ctx, m.matchers.StatefulsetName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("getting statefulset %s: %w", m.matchers.StatefulsetName, err)
		}
		selector := buildLabelSelector(sts.Spec.Selector.MatchLabels)
		pods, err := m.client.CoreV1().Pods(m.namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil {
			return nil, fmt.Errorf("listing pods for statefulset %s: %w", m.matchers.StatefulsetName, err)
		}
		addUnique(pods.Items)
	}

	return result, nil
}

func buildLabelSelector(labels map[string]string) string {
	var s string
	for k, v := range labels {
		if s != "" {
			s += ","
		}
		s += k + "=" + v
	}
	return s
}
