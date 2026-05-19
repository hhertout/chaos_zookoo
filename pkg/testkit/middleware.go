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
	queriers map[ClientKind]Querier

	mu      sync.Mutex
	timers  map[*time.Timer]struct{}
	stopped bool
	wg      sync.WaitGroup
}

// NewRunner wires a runner to its backing queriers.
// Pass nil or an empty map to create a runner with no backend configured.
// The map is copied so later mutations by the caller cannot affect the runner.
func NewRunner(queriers map[ClientKind]Querier) *Runner {
	snapshot := make(map[ClientKind]Querier, len(queriers))
	for k, v := range queriers {
		if v != nil {
			snapshot[k] = v
		}
	}
	return &Runner{
		queriers: snapshot,
		timers:   make(map[*time.Timer]struct{}),
	}
}

// BuildRunner creates a Runner from environment configuration.
// It populates a Grafana client when GRAFANA_URL is set and a Prometheus client
// when PROMETHEUS_URL is set. Both can coexist.
func BuildRunner(getenv func(string) string) *Runner {
	queriers := make(map[ClientKind]Querier)
	if u := getenv("GRAFANA_URL"); u != "" {
		queriers[ClientGrafana] = NewGrafanaClient(u, getenv("GRAFANA_TOKEN"), nil)
	}
	if u := getenv("PROMETHEUS_URL"); u != "" {
		queriers[ClientPrometheus] = NewPrometheusClient(
			u,
			getenv("PROMETHEUS_TOKEN"),
			getenv("PROMETHEUS_USERNAME"),
			getenv("PROMETHEUS_PASSWORD"),
			nil,
		)
	}
	return NewRunner(queriers)
}

// HasQuerier reports whether the runner has at least one non-nil backend wired in.
func (r *Runner) HasQuerier() bool {
	if r == nil {
		return false
	}
	for _, q := range r.queriers {
		if q != nil {
			return true
		}
	}
	return false
}

// HasQuerierFor reports whether the runner has a non-nil backend for the given client kind.
func (r *Runner) HasQuerierFor(client ClientKind) bool {
	if r == nil {
		return false
	}
	q, ok := r.queriers[client]
	return ok && q != nil
}

// Schedule defers a single evaluation after spec.MaxWait(). All tests in the
// spec run sequentially at that point, and the metric is emitted once.
func (r *Runner) Schedule(ctx context.Context, name, namespace string, spec *Spec) {
	if r == nil || spec == nil {
		return
	}

	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.wg.Add(1)

	wait := spec.MaxWait()
	// timer is written under r.mu (below) and read by the callback also under
	// r.mu, so the mutex provides the happens-before that prevents the data
	// race inherent in capturing a *Timer variable in an AfterFunc closure.
	var timer *time.Timer
	timer = time.AfterFunc(wait, func() {
		defer r.wg.Done()
		r.mu.Lock()
		t := timer
		delete(r.timers, t)
		r.mu.Unlock()
		r.runTest(ctx, name, namespace, spec)
	})
	r.timers[timer] = struct{}{}
	r.mu.Unlock()

	zap.L().Debug("chaos test scheduled",
		zap.String("name", name),
		zap.Duration("wait", wait),
		zap.Int("tests", len(spec.Details)),
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

func (r *Runner) runTest(ctx context.Context, name, namespace string, spec *Spec) {
	if err := ctx.Err(); err != nil {
		zap.L().Debug("chaos test skipped: context canceled", zap.String("name", name))
		return
	}

	querier, ok := r.queriers[spec.Client]
	if !ok || querier == nil {
		zap.L().Warn("chaos test skipped: no querier configured for client",
			zap.String("name", name),
			zap.String("client", string(spec.Client)),
		)
		metrics.ChaosTestSuccess.WithLabelValues(name, namespace).Set(0)
		return
	}

	allPassed := true
	for i, d := range spec.Details {
		queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		value, err := querier.Query(queryCtx, d)
		cancel()

		if err != nil {
			zap.L().Error("chaos test query failed",
				zap.String("name", name),
				zap.Int("index", i),
				zap.Error(err),
			)
			allPassed = false
			continue
		}

		pass := Evaluate(value, d.Operator, d.Threshold)
		if !pass {
			allPassed = false
		}
		zap.L().Info("chaos test evaluated",
			zap.String("name", name),
			zap.Int("index", i),
			zap.Float64("value", value),
			zap.String("operator", string(d.Operator)),
			zap.Float64("threshold", d.Threshold),
			zap.Bool("pass", pass),
		)
	}

	metricValue := 0.0
	if allPassed {
		metricValue = 1.0
	}
	metrics.ChaosTestSuccess.WithLabelValues(name, namespace).Set(metricValue)

	zap.L().Info("chaos test suite result",
		zap.String("name", name),
		zap.Int("tests", len(spec.Details)),
		zap.Bool("allPassed", allPassed),
	)
}

// NewMiddleware returns a Middleware that schedules a post-run test after
// every module execution. Returns a no-op middleware if spec or runner is nil.
func NewMiddleware(runner *Runner, spec *Spec) (module.Middleware, error) {
	if spec == nil || runner == nil {
		return func(m module.ChaosModule) module.ChaosModule { return m }, nil
	}
	if !runner.HasQuerierFor(spec.Client) {
		switch spec.Client {
		case ClientGrafana:
			return nil, fmt.Errorf("testing block requires GRAFANA_URL to be set")
		case ClientPrometheus:
			return nil, fmt.Errorf("testing block requires PROMETHEUS_URL to be set")
		default:
			return nil, fmt.Errorf("testing block: no querier configured for client %q", spec.Client)
		}
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
	w.runner.Schedule(ctx, w.inner.Name(), w.inner.Namespace(), w.spec)
	return err
}
