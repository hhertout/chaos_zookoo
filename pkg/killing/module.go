package killing

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/matchers"
	"github.com/hhertout/chaos_zookoo/pkg/metrics"
	"github.com/hhertout/chaos_zookoo/pkg/module"
	"go.uber.org/zap"
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
	switch strategy {
	case StrategyDelete:
		return deleteRemover{client: client}
	default:
		return evictRemover{client: client}
	}
}

// Module randomly kills pods that match the configured selectors.
type Module struct {
	client       kubernetes.Interface
	remover      PodRemover
	name         string
	namespace    string
	matchers     matchers.Matchers
	minAvailable int
	dryRun       bool
	strategy     Strategy
	interval     time.Duration
	wait         time.Duration
	cronExpr     string
}

// New creates a killing module from the given client and config.
func New(client kubernetes.Interface, cfg Config) *Module {
	return &Module{
		client:       client,
		remover:      newPodRemover(client, cfg.Scenario.Strategy),
		name:         cfg.Name,
		namespace:    cfg.Metadata.Namespace,
		matchers:     cfg.Scenario.Matchers,
		minAvailable: cfg.Scenario.MinAvailable,
		dryRun:       cfg.Scenario.DryRun,
		strategy:     cfg.Scenario.Strategy,
		interval:     cfg.interval,
		wait:         cfg.wait,
		cronExpr:     cfg.cronExpr,
	}
}

func (m *Module) Name() string      { return m.name }
func (m *Module) Kind() string      { return "Killing" }
func (m *Module) Namespace() string { return m.namespace }
func (m *Module) Schedule() module.Schedule {
	if m.cronExpr != "" {
		return module.Schedule{Mode: module.ScheduleCron, CronExpr: m.cronExpr}
	}
	return module.Schedule{Mode: module.SchedulePeriodic, Interval: m.interval}
}

// Run collects matching pods and randomly kills one, respecting minAvailable.
func (m *Module) Run(ctx context.Context) error {
	time.Sleep(m.wait)

	pods, err := matchers.CollectPods(ctx, m.client, m.namespace, m.matchers)
	if err != nil {
		return err
	}

	if len(pods) == 0 {
		zap.L().Warn("killing skipped: no pods match the configured matchers",
			zap.String("kind", "killing"),
			zap.String("name", m.name),
			zap.String("namespace", m.namespace),
			zap.String("reason", "no_match"),
		)
		return nil
	}

	if killable := len(pods) - m.minAvailable; killable <= 0 {
		zap.L().Warn("killing skipped: would breach minAvailable floor",
			zap.String("kind", "killing"),
			zap.String("name", m.name),
			zap.String("namespace", m.namespace),
			zap.Int("total", len(pods)),
			zap.Int("minAvailable", m.minAvailable),
			zap.String("reason", "min_available_floor"),
		)
		return nil
	}

	targetIndex, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(pods))))
	if err != nil {
		return fmt.Errorf("selecting pod target: %w", err)
	}
	target := pods[targetIndex.Int64()]
	zap.L().Info("pod killed",
		zap.String("kind", "killing"),
		zap.String("name", m.name),
		zap.String("namespace", m.namespace),
		zap.String("pod", target.Name),
		zap.String("strategy", string(m.strategy)),
		zap.Bool("dryRun", m.dryRun),
	)

	if m.dryRun {
		return nil
	}
	if err := m.remover.Remove(ctx, m.namespace, target.Name); err != nil {
		return err
	}
	metrics.ChaosPodsAffectedTotal.WithLabelValues(m.name, "Killing", m.namespace).Inc()
	return nil
}
