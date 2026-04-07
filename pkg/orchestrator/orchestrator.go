package orchestrator

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/module"
)

type scheduledModule struct {
	module   module.ChaosModule
	interval time.Duration
}

type Orchestrator struct {
	mu      sync.Mutex
	modules []scheduledModule
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

func New() *Orchestrator {
	return &Orchestrator{
		stopCh: make(chan struct{}),
	}
}

func (o *Orchestrator) Register(m module.ChaosModule) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.modules = append(o.modules, scheduledModule{module: m, interval: m.Interval()})
}

func (o *Orchestrator) Start(ctx context.Context) {
	o.mu.Lock()
	mods := make([]scheduledModule, len(o.modules))
	copy(mods, o.modules)
	o.mu.Unlock()

	for _, sm := range mods {
		o.wg.Add(1)
		go o.runLoop(ctx, sm)
	}

	slog.Info("orchestrator started", "modules", len(mods))
}

func (o *Orchestrator) Stop() {
	close(o.stopCh)
	o.wg.Wait()
	slog.Info("orchestrator stopped")
}

func (o *Orchestrator) runLoop(ctx context.Context, sm scheduledModule) {
	defer o.wg.Done()

	ticker := time.NewTicker(sm.interval)
	defer ticker.Stop()

	slog.Info("module scheduled", "module", sm.module.Name(), "interval", sm.interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.execute(ctx, sm)
		}
	}
}

func (o *Orchestrator) execute(ctx context.Context, sm scheduledModule) {
	o.mu.Lock()
	defer o.mu.Unlock()

	slog.Info("executing module", "module", sm.module.Name())
	if err := sm.module.Run(ctx); err != nil {
		slog.Error("module execution failed", "module", sm.module.Name(), "error", err)
	}
}
