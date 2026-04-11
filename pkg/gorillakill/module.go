package gorillakill

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/matchers"
	"github.com/hhertout/chaos_zookoo/pkg/metrics"
	"github.com/hhertout/chaos_zookoo/pkg/module"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var _ module.ChaosModule = (*Module)(nil)

// Module kills every pod matching the configured selectors in a single pass.
type Module struct {
	client    kubernetes.Interface
	name      string
	namespace string
	matchers  matchers.Matchers
	when      When
	dryRun    bool
	interval  time.Duration
	wait      time.Duration
	cronExpr  string
}

// New creates a gorillakill module from the given client and config.
func New(client kubernetes.Interface, cfg Config) *Module {
	return &Module{
		client:    client,
		name:      cfg.Name,
		namespace: cfg.Metadata.Namespace,
		matchers:  cfg.Scenario.Matchers,
		when:      cfg.Scenario.When,
		dryRun:    cfg.Scenario.DryRun,
		interval:  cfg.interval,
		wait:      cfg.wait,
		cronExpr:  cfg.cronExpr,
	}
}

func (m *Module) Name() string      { return m.name }
func (m *Module) Kind() string      { return "GorillaKill" }
func (m *Module) Namespace() string { return m.namespace }

func (m *Module) Schedule() module.Schedule {
	if m.when == WhenOnce {
		return module.Schedule{Mode: module.ScheduleOnce, InitialDelay: m.wait}
	}
	if m.cronExpr != "" {
		return module.Schedule{Mode: module.ScheduleCron, CronExpr: m.cronExpr}
	}
	return module.Schedule{Mode: module.SchedulePeriodic, Interval: m.interval, InitialDelay: m.wait}
}

// Run deletes every pod matching the configured selectors.
func (m *Module) Run(ctx context.Context) error {
	time.Sleep(m.wait)

	pods, err := matchers.CollectPods(ctx, m.client, m.namespace, m.matchers)
	if err != nil {
		return err
	}

	if len(pods) == 0 {
		zap.L().Warn("gorillakill skipped: no pods match the configured matchers",
			zap.String("kind", "gorillakill"),
			zap.String("name", m.name),
			zap.String("namespace", m.namespace),
			zap.String("reason", "no_match"),
		)
		return nil
	}

	zap.L().Info("gorilla-killing pods",
		zap.String("kind", "gorillakill"),
		zap.String("name", m.name),
		zap.String("namespace", m.namespace),
		zap.Int("count", len(pods)),
		zap.Bool("dryRun", m.dryRun),
	)

	var errs []error
	for _, p := range pods {
		zap.L().Info("pod killed",
			zap.String("kind", "gorillakill"),
			zap.String("name", m.name),
			zap.String("namespace", m.namespace),
			zap.String("pod", p.Name),
			zap.Bool("dryRun", m.dryRun),
		)
		if m.dryRun {
			continue
		}
		if err := m.client.CoreV1().Pods(m.namespace).Delete(ctx, p.Name, metav1.DeleteOptions{}); err != nil {
			errs = append(errs, fmt.Errorf("deleting pod %s: %w", p.Name, err))
			continue
		}
		metrics.ChaosPodsAffectedTotal.WithLabelValues(m.name, "GorillaKill", m.namespace).Inc()
	}
	return errors.Join(errs...)
}
