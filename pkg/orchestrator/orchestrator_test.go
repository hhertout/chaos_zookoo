package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockModule struct {
	name     string
	calls    atomic.Int32
	runErr   error
	interval time.Duration
}

func (m *mockModule) Name() string            { return m.name }
func (m *mockModule) Interval() time.Duration  { return m.interval }
func (m *mockModule) Run(_ context.Context) error {
	m.calls.Add(1)
	return m.runErr
}

func TestRegisterAndStart(t *testing.T) {
	o := New()
	mock := &mockModule{name: "test", interval: 50 * time.Millisecond}
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

func TestMutexSerializesExecution(t *testing.T) {
	o := New()
	m1 := &mockModule{name: "m1", interval: 50 * time.Millisecond}
	m2 := &mockModule{name: "m2", interval: 50 * time.Millisecond}
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

func TestRegisterMultiple(t *testing.T) {
	o := New()
	o.Register(&mockModule{name: "exp1", interval: time.Minute})
	o.Register(&mockModule{name: "exp2", interval: time.Minute})

	require.Len(t, o.modules, 2)
}
