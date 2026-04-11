package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	gronx "github.com/adhocore/gronx"
	"github.com/hhertout/chaos_zookoo/pkg/metrics"
	"github.com/hhertout/chaos_zookoo/pkg/module"
	"go.uber.org/zap"
)

type scheduledModule struct {
	mu       sync.Mutex // serializes concurrent executions of the same module
	module   module.ChaosModule
	schedule module.Schedule
}

// Orchestrator schedules and runs chaos modules according to their configured schedule.
type Orchestrator struct {
	mu       sync.Mutex // protects the modules slice only
	modules  []*scheduledModule
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// New creates a ready-to-use Orchestrator.
func New() *Orchestrator {
	return &Orchestrator{
		stopCh: make(chan struct{}),
	}
}

// Register adds a chaos module to the orchestrator and publishes its info metric.
func (o *Orchestrator) Register(m module.ChaosModule) {
	o.mu.Lock()
	defer o.mu.Unlock()
	sched := m.Schedule()
	o.modules = append(o.modules, &scheduledModule{module: m, schedule: sched})

	schedType, schedValue := scheduleLabel(sched)
	metrics.ChaosModuleInfo.WithLabelValues(
		m.Name(), m.Kind(), m.Namespace(), schedType, schedValue,
	).Set(1)
}

// Start launches a goroutine per registered module.
func (o *Orchestrator) Start(ctx context.Context) {
	o.mu.Lock()
	mods := make([]*scheduledModule, len(o.modules))
	copy(mods, o.modules)
	o.mu.Unlock()

	for _, sm := range mods {
		o.wg.Add(1)
		go o.runLoop(ctx, sm)
	}
	zap.L().Info("orchestrator started", zap.Int("modules", len(mods)))
}

// Stop signals all loops to exit and waits for them to finish.
// It is safe to call multiple times.
func (o *Orchestrator) Stop() {
	o.stopOnce.Do(func() { close(o.stopCh) })
	o.wg.Wait()
	zap.L().Info("orchestrator stopped")
}

func (o *Orchestrator) runLoop(ctx context.Context, sm *scheduledModule) {
	defer o.wg.Done()

	switch sm.schedule.Mode {
	case module.ScheduleOnce:
		zap.L().Info("module scheduled",
			zap.String("module", sm.module.Name()),
			zap.String("mode", "once"),
			zap.Duration("wait", sm.schedule.InitialDelay),
		)
		if !o.waitInitialDelay(ctx, sm.schedule.InitialDelay) {
			return
		}
		o.execute(ctx, sm)

	case module.ScheduleCron:
		zap.L().Info("module scheduled",
			zap.String("module", sm.module.Name()),
			zap.String("mode", "cron"),
			zap.String("expr", sm.schedule.CronExpr),
		)
		o.runCronLoop(ctx, sm)

	default: // SchedulePeriodic
		zap.L().Info("module scheduled",
			zap.String("module", sm.module.Name()),
			zap.String("mode", "periodic"),
			zap.Duration("interval", sm.schedule.Interval),
			zap.Duration("wait", sm.schedule.InitialDelay),
		)
		// InitialDelay == 0 fires immediately; > 0 waits then fires.
		if !o.waitInitialDelay(ctx, sm.schedule.InitialDelay) {
			return
		}
		o.execute(ctx, sm)
		ticker := time.NewTicker(sm.schedule.Interval)
		defer ticker.Stop()
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
}

func (o *Orchestrator) runCronLoop(ctx context.Context, sm *scheduledModule) {
	for {
		next, err := gronx.NextTick(sm.schedule.CronExpr, false)
		if err != nil {
			zap.L().Error("invalid cron expression",
				zap.String("module", sm.module.Name()),
				zap.String("expr", sm.schedule.CronExpr),
				zap.Error(err),
			)
			return
		}
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-o.stopCh:
			timer.Stop()
			return
		case <-timer.C:
			o.execute(ctx, sm)
		}
	}
}

// waitInitialDelay sleeps for d, returning false if the orchestrator was
// canceled before the delay elapsed.
// d <= 0 returns true immediately unless already cancelled.
func (o *Orchestrator) waitInitialDelay(ctx context.Context, d time.Duration) bool {
	// Explicit pre-check so a cancelled context always wins, even when d == 0.
	if ctx.Err() != nil {
		return false
	}
	select {
	case <-o.stopCh:
		return false
	default:
	}
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-o.stopCh:
		return false
	case <-timer.C:
		return true
	}
}

func (o *Orchestrator) execute(ctx context.Context, sm *scheduledModule) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	name, kind, ns := sm.module.Name(), sm.module.Kind(), sm.module.Namespace()
	zap.L().Debug("executing module", zap.String("module", name))

	start := time.Now()
	err := sm.module.Run(ctx)
	dur := time.Since(start)

	status := "success"
	if err != nil {
		status = "error"
		zap.L().Error("module execution failed", zap.String("module", name), zap.Error(err))
	}

	metrics.ChaosModuleRunsTotal.WithLabelValues(name, kind, ns, status).Inc()
	metrics.ChaosModuleLastRunTimestamp.WithLabelValues(name, kind, ns).SetToCurrentTime()
	metrics.ChaosModuleRunDuration.WithLabelValues(name, kind, ns).Observe(dur.Seconds())
}

func scheduleLabel(s module.Schedule) (schedType, schedValue string) {
	switch s.Mode {
	case module.ScheduleOnce:
		return "once", "once"
	case module.ScheduleCron:
		return "cron", s.CronExpr
	default:
		return "periodic", fmt.Sprintf("%v", s.Interval)
	}
}
