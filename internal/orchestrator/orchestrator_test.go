package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// counter is a mutex-protected int32 counter used in place of sync/atomic.Int32
// to satisfy the depguard rule banning sync/atomic imports.
type counter struct {
	mu  sync.Mutex
	val int32
}

func (c *counter) Add(n int32) {
	c.mu.Lock()
	c.val += n
	c.mu.Unlock()
}

func (c *counter) Load() int32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.val
}

// ---- mock ----

type mockModule struct {
	name     string
	calls    counter
	runErr   error
	runDelay time.Duration // if > 0, Run blocks for this duration (ctx-aware)
	schedule module.Schedule
}

func (m *mockModule) Name() string              { return m.name }
func (m *mockModule) Kind() string              { return "Mock" }
func (m *mockModule) Namespace() string         { return "test" }
func (m *mockModule) Schedule() module.Schedule { return m.schedule }
func (m *mockModule) Run(ctx context.Context) error {
	m.calls.Add(1)
	if m.runDelay > 0 {
		select {
		case <-time.After(m.runDelay):
		case <-ctx.Done():
		}
	}
	return m.runErr
}

// ---- schedule helpers ----

func periodic(interval time.Duration) module.Schedule {
	return module.Schedule{Mode: module.SchedulePeriodic, Interval: interval}
}

func periodicWithDelay(interval, delay time.Duration) module.Schedule {
	return module.Schedule{Mode: module.SchedulePeriodic, Interval: interval, InitialDelay: delay}
}

func once() module.Schedule {
	return module.Schedule{Mode: module.ScheduleOnce}
}

func onceWithDelay(delay time.Duration) module.Schedule {
	return module.Schedule{Mode: module.ScheduleOnce, InitialDelay: delay}
}

func cronSchedule(expr string) module.Schedule {
	return module.Schedule{Mode: module.ScheduleCron, CronExpr: expr}
}

// ---- Register / Start / Stop ----

func TestRegisterAndStart(t *testing.T) {
	o := New()
	mock := &mockModule{name: "test", schedule: periodic(50 * time.Millisecond)}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(150 * time.Millisecond)
	cancel()
	o.Stop()

	assert.GreaterOrEqual(t, mock.calls.Load(), int32(1))
}

func TestStopWithoutStart(_ *testing.T) {
	o := New()
	o.Stop()
}

func TestStopIsIdempotent(t *testing.T) {
	o := New()
	mock := &mockModule{name: "test", schedule: periodic(50 * time.Millisecond)}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()

	o.Stop()
	o.Stop() // must not panic
}

func TestStopStopsModulesWithoutContextCancel(t *testing.T) {
	o := New()
	mock := &mockModule{name: "stop-test", schedule: periodic(30 * time.Millisecond)}
	o.Register(mock)

	o.Start(context.Background()) // context never cancelled
	time.Sleep(100 * time.Millisecond)
	o.Stop() // must drain and stop goroutines on its own

	callsAfterStop := mock.calls.Load()
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, callsAfterStop, mock.calls.Load(), "no more calls after Stop()")
}

func TestRegisterMultiple(t *testing.T) {
	o := New()
	o.Register(&mockModule{name: "exp1", schedule: periodic(time.Minute)})
	o.Register(&mockModule{name: "exp2", schedule: periodic(time.Minute)})

	require.Len(t, o.modules, 2)
}

// ---- ScheduleOnce ----

func TestOnceModuleRunsExactlyOnce(t *testing.T) {
	o := New()
	mock := &mockModule{name: "one-shot", schedule: once()}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(100 * time.Millisecond)
	cancel()
	o.Stop()

	assert.Equal(t, int32(1), mock.calls.Load())
}

func TestOnceModuleWithInitialDelay(t *testing.T) {
	o := New()
	mock := &mockModule{name: "delayed-once", schedule: onceWithDelay(40 * time.Millisecond)}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o.Start(ctx)

	time.Sleep(15 * time.Millisecond)
	assert.Equal(t, int32(0), mock.calls.Load(), "must not run before initial delay elapses")

	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, int32(1), mock.calls.Load(), "must run exactly once after initial delay")

	cancel()
	o.Stop()
	assert.Equal(t, int32(1), mock.calls.Load(), "must not run again after stop")
}

func TestOnceModuleCancelledDuringInitialDelay(t *testing.T) {
	o := New()
	mock := &mockModule{name: "cancelled-once", schedule: onceWithDelay(200 * time.Millisecond)}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(20 * time.Millisecond)
	cancel()
	o.Stop()

	assert.Equal(t, int32(0), mock.calls.Load(), "module must not run when context cancelled before delay")
}

// ---- SchedulePeriodic ----

func TestConcurrentExecution(t *testing.T) {
	o := New()
	m1 := &mockModule{name: "m1", schedule: periodic(50 * time.Millisecond)}
	m2 := &mockModule{name: "m2", schedule: periodic(50 * time.Millisecond)}
	o.Register(m1)
	o.Register(m2)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	cancel()
	o.Stop()

	assert.GreaterOrEqual(t, m1.calls.Load(), int32(1))
	assert.GreaterOrEqual(t, m2.calls.Load(), int32(1))
}

func TestPeriodicWithInitialDelay(t *testing.T) {
	o := New()
	mock := &mockModule{name: "delayed-periodic", schedule: periodicWithDelay(500*time.Millisecond, 40*time.Millisecond)}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	// Before initial delay: must not have fired.
	time.Sleep(15 * time.Millisecond)
	assert.Equal(t, int32(0), mock.calls.Load(), "must not run before initial delay")

	// After initial delay (well within interval): must have fired exactly once.
	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, int32(1), mock.calls.Load(), "must run once at initial delay; periodic interval not yet elapsed")

	cancel()
	o.Stop()
}

func TestPeriodicCancelledDuringInitialDelay(t *testing.T) {
	o := New()
	mock := &mockModule{name: "cancelled-periodic", schedule: periodicWithDelay(100*time.Millisecond, 200*time.Millisecond)}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(20 * time.Millisecond)
	cancel()
	o.Stop()

	assert.Equal(t, int32(0), mock.calls.Load(), "module must not run when cancelled during initial delay")
}

// TestPeriodicNoInitialDelayRunsImmediately documents that a periodic module
// with no InitialDelay fires once on start, then on each interval.
func TestPeriodicNoInitialDelayRunsImmediately(t *testing.T) {
	o := New()
	mock := &mockModule{name: "immediate", schedule: periodic(1 * time.Second)}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()
	o.Stop()

	assert.Equal(t, int32(1), mock.calls.Load(),
		"periodic module with no InitialDelay must run exactly once on start before the interval elapses")
}

func TestModuleErrorContinuesScheduling(t *testing.T) {
	o := New()
	mock := &mockModule{
		name:     "erroring",
		schedule: periodic(30 * time.Millisecond),
		runErr:   errors.New("simulated failure"),
	}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(120 * time.Millisecond)
	cancel()
	o.Stop()

	assert.GreaterOrEqual(t, mock.calls.Load(), int32(2),
		"orchestrator must keep scheduling even when module returns errors")
}

// ---- ScheduleCron ----

func TestCronModuleExitsOnContextCancel(t *testing.T) {
	o := New()
	// Standard 5-field cron fires at most once per minute; we cancel before that.
	mock := &mockModule{name: "cron-cancel", schedule: cronSchedule("* * * * *")}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(20 * time.Millisecond)
	cancel()
	o.Stop()

	assert.Equal(t, int32(0), mock.calls.Load(), "cron module cancelled before first tick must not run")
}

func TestCronModuleExitsOnStop(t *testing.T) {
	o := New()
	mock := &mockModule{name: "cron-stop", schedule: cronSchedule("* * * * *")}
	o.Register(mock)

	o.Start(context.Background())

	time.Sleep(20 * time.Millisecond)
	o.Stop()

	assert.Equal(t, int32(0), mock.calls.Load())
}

func TestCronModuleInvalidExprExitsGoroutine(t *testing.T) {
	o := New()
	mock := &mockModule{name: "cron-invalid", schedule: cronSchedule("not-valid-cron")}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o.Start(ctx)

	// Goroutine logs the error and exits — WaitGroup must decrement so Stop() returns.
	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// goroutine exited cleanly
	case <-time.After(500 * time.Millisecond):
		t.Fatal("goroutine did not exit after invalid cron expression")
	}

	assert.Equal(t, int32(0), mock.calls.Load(), "invalid cron expr must not trigger any execution")
}

// ---- waitInitialDelay unit tests ----

func TestWaitInitialDelay_Zero_NotCancelled(t *testing.T) {
	o := New()
	assert.True(t, o.waitInitialDelay(context.Background(), 0))
}

func TestWaitInitialDelay_Zero_AlreadyCancelled(t *testing.T) {
	o := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.False(t, o.waitInitialDelay(ctx, 0), "must deterministically return false when ctx is already cancelled")
}

func TestWaitInitialDelay_Positive_Completes(t *testing.T) {
	o := New()
	start := time.Now()
	result := o.waitInitialDelay(context.Background(), 30*time.Millisecond)
	assert.True(t, result)
	assert.GreaterOrEqual(t, time.Since(start), 30*time.Millisecond)
}

func TestWaitInitialDelay_Positive_CancelledDuringWait(t *testing.T) {
	o := New()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	result := o.waitInitialDelay(ctx, 500*time.Millisecond)
	assert.False(t, result)
}

func TestWaitInitialDelay_Positive_StopDuringWait(t *testing.T) {
	o := New()
	go func() {
		time.Sleep(20 * time.Millisecond)
		o.stopOnce.Do(func() { close(o.stopCh) })
	}()
	result := o.waitInitialDelay(context.Background(), 500*time.Millisecond)
	assert.False(t, result)
}

// ---- edge cases ----

// TestStartAndStopWithNoModules verifies the orchestrator doesn't deadlock or
// panic when started with an empty module list.
func TestStartAndStopWithNoModules(t *testing.T) {
	o := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o.Start(ctx)
	o.Stop()
}

// TestStopImmediatelyAfterStart checks that calling Stop with no delay after
// Start does not race, panic, or hang.
func TestStopImmediatelyAfterStart(t *testing.T) {
	o := New()
	mock := &mockModule{name: "test", schedule: periodic(50 * time.Millisecond)}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o.Start(ctx)
	o.Stop()
}

// TestContextAlreadyCancelledAtStart ensures that no module runs when the
// context is already cancelled before Start is called.
func TestContextAlreadyCancelledAtStart(t *testing.T) {
	o := New()
	m1 := &mockModule{name: "once", schedule: once()}
	m2 := &mockModule{name: "periodic", schedule: periodic(10 * time.Millisecond)}
	o.Register(m1)
	o.Register(m2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before Start

	o.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	o.Stop()

	assert.Equal(t, int32(0), m1.calls.Load(), "once module must not run with pre-cancelled ctx")
	assert.Equal(t, int32(0), m2.calls.Load(), "periodic module must not run with pre-cancelled ctx")
}

// TestRegisterAfterStartIsIgnored documents that modules registered after
// Start has been called are added to the slice but are not scheduled.
func TestRegisterAfterStartIsIgnored(t *testing.T) {
	o := New()
	m1 := &mockModule{name: "before", schedule: periodic(20 * time.Millisecond)}
	o.Register(m1)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	m2 := &mockModule{name: "after", schedule: periodic(10 * time.Millisecond)}
	o.Register(m2)

	time.Sleep(100 * time.Millisecond)
	cancel()
	o.Stop()

	assert.GreaterOrEqual(t, m1.calls.Load(), int32(1), "module registered before Start must run")
	assert.Equal(t, int32(0), m2.calls.Load(), "module registered after Start must not run")
}

// TestSlowModuleDoesNotCascadeMissedTicks verifies that when a module's Run
// takes longer than its interval, missed ticks are dropped rather than queued.
// time.Ticker has a buffer of 1: at most one pending tick can accumulate.
func TestSlowModuleDoesNotCascadeMissedTicks(t *testing.T) {
	o := New()
	mock := &mockModule{
		name:     "slow-periodic",
		schedule: periodic(20 * time.Millisecond),
		runDelay: 80 * time.Millisecond, // 4× the interval
	}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(350 * time.Millisecond)
	cancel()
	o.Stop()

	// 350ms / 80ms ≈ 4 natural runs; cascading would produce 350ms / 20ms = 17.
	assert.LessOrEqual(t, mock.calls.Load(), int32(6), "missed ticks must not cascade")
	assert.GreaterOrEqual(t, mock.calls.Load(), int32(2), "module must have run multiple times")
}

// TestModulesRunConcurrently verifies that the per-module mutex allows two
// distinct modules to execute simultaneously. A slow module must not delay a
// fast one (which would happen with the old global mutex).
func TestModulesRunConcurrently(t *testing.T) {
	o := New()
	slow := &mockModule{
		name:     "slow",
		schedule: once(),
		runDelay: 80 * time.Millisecond,
	}
	fast := &mockModule{
		name:     "fast",
		schedule: periodic(15 * time.Millisecond),
	}
	o.Register(slow)
	o.Register(fast)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(100 * time.Millisecond)
	cancel()
	o.Stop()

	// With a global mutex, fast would be blocked for ~80ms → only 1–2 runs.
	// With per-module mutex, fast runs freely every 15ms → ≥4 runs in 100ms.
	assert.GreaterOrEqual(t, fast.calls.Load(), int32(4),
		"fast module must not be blocked by slow module (per-module mutex)")
}

// TestStopWaitsForRunningModule verifies that Stop blocks until the currently
// running module's Run returns, preventing torn state.
func TestStopWaitsForRunningModule(t *testing.T) {
	o := New()
	mock := &mockModule{
		name:     "slow-once",
		schedule: once(),
		runDelay: 100 * time.Millisecond,
	}
	o.Register(mock)

	// Use Background so the module's sleep is not interrupted by ctx cancel.
	o.Start(context.Background())
	time.Sleep(20 * time.Millisecond) // let the module enter Run

	start := time.Now()
	o.Stop() // must block until Run returns
	elapsed := time.Since(start)

	// Module had ~80ms left when Stop was called; Stop must have waited.
	assert.GreaterOrEqual(t, elapsed, 60*time.Millisecond,
		"Stop must wait for in-flight Run to complete")
	assert.Equal(t, int32(1), mock.calls.Load())
}

// TestNegativeInitialDelayTreatedAsImmediate verifies that a negative
// InitialDelay is treated the same as zero: the module fires immediately.
func TestNegativeInitialDelayTreatedAsImmediate(t *testing.T) {
	o := New()
	mock := &mockModule{
		name: "neg-delay",
		schedule: module.Schedule{
			Mode:         module.SchedulePeriodic,
			Interval:     1 * time.Second,
			InitialDelay: -5 * time.Millisecond,
		},
	}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()
	o.Stop()

	assert.Equal(t, int32(1), mock.calls.Load(),
		"negative InitialDelay must be treated as 0 (fires immediately)")
}

// TestConcurrentRegisterIsSafe verifies that multiple goroutines can call
// Register simultaneously without data races.
func TestConcurrentRegisterIsSafe(t *testing.T) {
	o := New()
	var wg sync.WaitGroup
	const n = 20
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			o.Register(&mockModule{
				name:     fmt.Sprintf("m%d", i),
				schedule: periodic(time.Minute),
			})
		}(i)
	}
	wg.Wait()
	require.Len(t, o.modules, n)
}

// TestHighModuleCount stress-tests the orchestrator with many modules running
// concurrently to surface any scheduling or synchronisation issue.
func TestHighModuleCount(t *testing.T) {
	const n = 50
	o := New()
	mocks := make([]*mockModule, n)
	for i := range mocks {
		mocks[i] = &mockModule{
			name:     fmt.Sprintf("module-%d", i),
			schedule: once(),
		}
		o.Register(mocks[i])
	}

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	cancel()
	o.Stop()

	for i, m := range mocks {
		assert.Equal(t, int32(1), m.calls.Load(), "module %d must have run exactly once", i)
	}
}

// TestOnceModuleStoppedDuringDelay verifies the stopCh path in waitInitialDelay
// for ScheduleOnce: Stop() (not ctx cancel) must prevent execution.
func TestOnceModuleStoppedDuringDelay(t *testing.T) {
	o := New()
	mock := &mockModule{name: "stopped-once", schedule: onceWithDelay(200 * time.Millisecond)}
	o.Register(mock)

	o.Start(context.Background()) // never cancel ctx

	time.Sleep(20 * time.Millisecond)
	o.Stop() // close stopCh while module is waiting in initial delay

	assert.Equal(t, int32(0), mock.calls.Load(),
		"once module must not run when Stop closes stopCh before initial delay elapses")
}

// TestPeriodicIntervalCadence verifies that the scheduling cadence is
// approximately correct: not too fast (burst) and not too slow (stall).
func TestPeriodicIntervalCadence(t *testing.T) {
	const interval = 40 * time.Millisecond
	const testDuration = 220 * time.Millisecond

	o := New()
	mock := &mockModule{name: "cadence", schedule: periodic(interval)}
	o.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(testDuration)
	cancel()
	o.Stop()

	calls := mock.calls.Load()
	// First run is immediate; subsequent runs are on the ticker (every 40ms).
	// 220ms: 1 immediate + floor((220-0)/40) = 1 + 5 = 6 max, allowing slack → ≤7.
	// At minimum we expect the immediate run + at least 2 ticks → ≥3.
	assert.GreaterOrEqual(t, calls, int32(3), "interval too slow: module under-fired")
	assert.LessOrEqual(t, calls, int32(7), "interval too fast: module over-fired")
}

// TestDuplicateModuleNamesBothScheduled documents that the orchestrator does
// not deduplicate by name: two modules with the same name are both scheduled
// and both run independently.
func TestDuplicateModuleNamesBothScheduled(t *testing.T) {
	o := New()
	m1 := &mockModule{name: "dup", schedule: once()}
	m2 := &mockModule{name: "dup", schedule: once()}
	o.Register(m1)
	o.Register(m2)

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx)

	time.Sleep(100 * time.Millisecond)
	cancel()
	o.Stop()

	assert.Equal(t, int32(1), m1.calls.Load(), "first module must run once")
	assert.Equal(t, int32(1), m2.calls.Load(), "second module with same name must also run once")
}

// ---- scheduleLabel ----

func TestScheduleLabel(t *testing.T) {
	tests := []struct {
		name      string
		schedule  module.Schedule
		wantType  string
		wantValue string
	}{
		{
			name:      "once",
			schedule:  module.Schedule{Mode: module.ScheduleOnce},
			wantType:  scheduleOnce,
			wantValue: scheduleOnce,
		},
		{
			name:      "cron",
			schedule:  module.Schedule{Mode: module.ScheduleCron, CronExpr: "*/5 * * * *"},
			wantType:  scheduleCron,
			wantValue: "*/5 * * * *",
		},
		{
			name:      "periodic",
			schedule:  module.Schedule{Mode: module.SchedulePeriodic, Interval: 30 * time.Second},
			wantType:  schedulePeriodic,
			wantValue: fmt.Sprintf("%v", 30*time.Second),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotValue := scheduleLabel(tt.schedule)
			assert.Equal(t, tt.wantType, gotType)
			assert.Equal(t, tt.wantValue, gotValue)
		})
	}
}
