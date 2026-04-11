package testkit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/metrics"
	"github.com/hhertout/chaos_zookoo/pkg/module"
	"go.uber.org/zap"
)

// Runner schedules and executes post-run tests. Tests are deferred with
// time.AfterFunc so the wait window does not hold a goroutine.
type Runner struct {
	querier Querier

	mu      sync.Mutex
	timers  map[*time.Timer]struct{}
	stopped bool
	wg      sync.WaitGroup
}

// NewRunner wires a runner to its backing querier. The querier may be nil,
// in which case the runner will reject scheduling until one is configured.
func NewRunner(querier Querier) *Runner {
	return &Runner{
		querier: querier,
		timers:  make(map[*time.Timer]struct{}),
	}
}

// BuildRunner creates a Runner from environment configuration.
// The lookup function is used to resolve GRAFANA_URL and GRAFANA_TOKEN.
func BuildRunner(getenv func(string) string) *Runner {
	url := getenv("GRAFANA_URL")
	if url == "" {
		return NewRunner(nil)
	}
	token := getenv("GRAFANA_TOKEN")
	return NewRunner(NewGrafanaClient(url, token, nil))
}

// HasQuerier reports whether the runner has a backend wired in.
func (r *Runner) HasQuerier() bool {
	if r == nil {
		return false
	}
	return r.querier != nil
}

// Schedule defers a test evaluation by spec.Details.Wait().
func (r *Runner) Schedule(ctx context.Context, name string, spec *Spec) {
	if r == nil || spec == nil {
		return
	}

	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.wg.Add(1)

	var timer *time.Timer
	timer = time.AfterFunc(spec.Details.Wait(), func() {
		defer r.wg.Done()
		r.forgetTimer(timer)
		r.runTest(ctx, name, spec)
	})
	r.timers[timer] = struct{}{}
	r.mu.Unlock()

	zap.L().Debug("chaos test scheduled",
		zap.String("name", name),
		zap.Duration("wait", spec.Details.Wait()),
	)
}

// Stop cancels every pending test and waits for in-flight evaluations.
func (r *Runner) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.stopped = true
	pending := make([]*time.Timer, 0, len(r.timers))
	for t := range r.timers {
		pending = append(pending, t)
	}
	r.mu.Unlock()

	for _, t := range pending {
		if t.Stop() {
			r.forgetTimer(t)
			r.wg.Done()
		}
	}
	r.wg.Wait()
}

func (r *Runner) forgetTimer(t *time.Timer) {
	r.mu.Lock()
	delete(r.timers, t)
	r.mu.Unlock()
}

func (r *Runner) runTest(ctx context.Context, name string, spec *Spec) {
	if err := ctx.Err(); err != nil {
		zap.L().Debug("chaos test skipped: context canceled", zap.String("name", name))
		return
	}

	if r.querier == nil {
		zap.L().Warn("chaos test skipped: no querier configured", zap.String("name", name))
		metrics.ChaosTestSuccess.WithLabelValues(name).Set(0)
		return
	}

	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	value, err := r.querier.Query(queryCtx, spec.Details)
	if err != nil {
		zap.L().Error("chaos test query failed",
			zap.String("name", name),
			zap.Error(err),
		)
		metrics.ChaosTestSuccess.WithLabelValues(name).Set(0)
		return
	}

	pass := Evaluate(value, spec.Details.Operator, spec.Details.Threshold)
	metricValue := 0.0
	if pass {
		metricValue = 1.0
	}
	metrics.ChaosTestSuccess.WithLabelValues(name).Set(metricValue)

	zap.L().Info("chaos test evaluated",
		zap.String("name", name),
		zap.Float64("value", value),
		zap.String("operator", string(spec.Details.Operator)),
		zap.Float64("threshold", spec.Details.Threshold),
		zap.Bool("pass", pass),
	)
}

// NewMiddleware returns a Middleware that schedules a post-run test after
// every module execution. Returns a no-op middleware if spec or runner is nil.
func NewMiddleware(runner *Runner, spec *Spec) (module.Middleware, error) {
	if spec == nil || runner == nil {
		return func(m module.ChaosModule) module.ChaosModule { return m }, nil
	}
	if !runner.HasQuerier() {
		return nil, fmt.Errorf("testing block requires GRAFANA_URL to be set")
	}
	return func(inner module.ChaosModule) module.ChaosModule {
		return &wrapped{inner: inner, runner: runner, spec: spec}
	}, nil
}

type wrapped struct {
	inner  module.ChaosModule
	runner *Runner
	spec   *Spec
}

func (w *wrapped) Name() string              { return w.inner.Name() }
func (w *wrapped) Kind() string              { return w.inner.Kind() }
func (w *wrapped) Namespace() string         { return w.inner.Namespace() }
func (w *wrapped) Schedule() module.Schedule { return w.inner.Schedule() }
func (w *wrapped) Run(ctx context.Context) error {
	err := w.inner.Run(ctx)
	w.runner.Schedule(ctx, w.inner.Name(), w.spec)
	return err
}
