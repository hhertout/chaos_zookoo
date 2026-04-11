package matchers

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type workloadResolver func(ctx context.Context, client kubernetes.Interface, namespace, name string) (map[string]string, error)

var workloadResolvers = map[string]struct {
	name     func(m Matchers) string
	resolver workloadResolver
}{
	"deployment": {
		name: func(m Matchers) string { return m.DeploymentName },
		resolver: func(ctx context.Context, client kubernetes.Interface, ns, name string) (map[string]string, error) {
			d, err := client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("getting deployment %s: %w", name, err)
			}
			return d.Spec.Selector.MatchLabels, nil
		},
	},
	"daemonset": {
		name: func(m Matchers) string { return m.DaemonsetName },
		resolver: func(ctx context.Context, client kubernetes.Interface, ns, name string) (map[string]string, error) {
			d, err := client.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("getting daemonset %s: %w", name, err)
			}
			return d.Spec.Selector.MatchLabels, nil
		},
	},
	"statefulset": {
		name: func(m Matchers) string { return m.StatefulsetName },
		resolver: func(ctx context.Context, client kubernetes.Interface, ns, name string) (map[string]string, error) {
			d, err := client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("getting statefulset %s: %w", name, err)
			}
			return d.Spec.Selector.MatchLabels, nil
		},
	},
}

// CollectPods returns the union of pods in the namespace matching any of the configured selectors.
func CollectPods(ctx context.Context, client kubernetes.Interface, namespace string, m Matchers) ([]corev1.Pod, error) {
	seen := make(map[string]struct{})
	var result []corev1.Pod

	addUnique := func(pods []corev1.Pod) {
		for _, p := range pods {
			if _, ok := seen[p.Name]; ok {
				continue
			}
			seen[p.Name] = struct{}{}
			result = append(result, p)
		}
	}

	if len(m.Labels) > 0 {
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: BuildLabelSelector(m.Labels),
		})
		if err != nil {
			return nil, fmt.Errorf("listing pods by labels: %w", err)
		}
		addUnique(pods.Items)
	}

	if m.PodName != "" {
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, m.PodName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("getting pod %s: %w", m.PodName, err)
		}
		addUnique([]corev1.Pod{*pod})
	}

	for _, wk := range workloadResolvers {
		resourceName := wk.name(m)
		if resourceName == "" {
			continue
		}
		labels, err := wk.resolver(ctx, client, namespace, resourceName)
		if err != nil {
			return nil, err
		}
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: BuildLabelSelector(labels),
		})
		if err != nil {
			return nil, fmt.Errorf("listing pods for %s: %w", resourceName, err)
		}
		addUnique(pods.Items)
	}

	return result, nil
}

// BuildLabelSelector serializes a label map into a comma-separated equality selector.
func BuildLabelSelector(labels map[string]string) string {
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, k+"="+v)
	}
	return strings.Join(pairs, ",")
}
