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
	allPods, err := m.client.CoreV1().Pods(m.namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})
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
			return fmt.Errorf("removing pod %s: %w", p.Name, err)
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
