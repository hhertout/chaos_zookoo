package loadkit

import (
	"context"
	"sync"

	"github.com/hhertout/chaos_zookoo/pkg/module"
	"go.uber.org/zap"
)

// Supervisor tracks in-flight load bursts so they can be drained at shutdown.
type Supervisor struct {
	mu      sync.Mutex
	stopped bool
	wg      sync.WaitGroup
}

// NewSupervisor builds a ready-to-use supervisor.
func NewSupervisor() *Supervisor { return &Supervisor{} }

// Stop waits for every in-flight burst to finish.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Supervisor) trigger(ctx context.Context, runner *Runner) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()
		if err := runner.Run(ctx); err != nil {
			zap.L().Error("load burst error", zap.String("name", runner.name), zap.Error(err))
		}
	}()
}

// NewMiddleware returns a Middleware that fires a load burst in parallel with
// every module execution. Returns a no-op middleware if spec is nil.
func NewMiddleware(sup *Supervisor, spec *Spec) module.Middleware {
	if spec == nil || sup == nil {
		return func(m module.ChaosModule) module.ChaosModule { return m }
	}
	return func(inner module.ChaosModule) module.ChaosModule {
		runner := NewRunner(inner.Name(), inner.Namespace(), spec)
		return &wrapped{inner: inner, sup: sup, runner: runner}
	}
}

type wrapped struct {
	inner  module.ChaosModule
	sup    *Supervisor
	runner *Runner
}

func (w *wrapped) Name() string              { return w.inner.Name() }
func (w *wrapped) Kind() string              { return w.inner.Kind() }
func (w *wrapped) Namespace() string         { return w.inner.Namespace() }
func (w *wrapped) Schedule() module.Schedule { return w.inner.Schedule() }
func (w *wrapped) Run(ctx context.Context) error {
	w.sup.trigger(ctx, w.runner)
	return w.inner.Run(ctx)
}
