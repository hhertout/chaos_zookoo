package testkit

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hhertout/chaos_zookoo/pkg/metrics"
)

// mockQuerier replays a fixed sequence of (value, error) pairs, one per Query call.
type mockQuerier struct {
	results []queryResult
	callIdx int
}

type queryResult struct {
	value float64
	err   error
}

func (m *mockQuerier) Query(_ context.Context, _ Details) (float64, error) {
	if m.callIdx >= len(m.results) {
		return 0, fmt.Errorf("mockQuerier: unexpected call #%d", m.callIdx)
	}
	r := m.results[m.callIdx]
	m.callIdx++
	return r.value, r.err
}

// metricValue reads the current value of ChaosTestSuccess for a given module name and namespace.
func metricValue(name, namespace string) float64 {
	return testutil.ToFloat64(metrics.ChaosTestSuccess.WithLabelValues(name, namespace))
}

// builtSpec builds and validates a Spec from a list of Details, fataling on error.
func builtSpec(t *testing.T, details ...Details) *Spec {
	t.Helper()
	s := &Spec{Client: ClientGrafana, Details: details}
	require.NoError(t, s.ApplyDefaultsAndValidate(0))
	return s
}

// ── runTest ───────────────────────────────────────────────────────────────────

func TestRunTest_AllPass_MetricIsOne(t *testing.T) {
	name := "all-pass"
	namespace := "default"
	q := &mockQuerier{results: []queryResult{{value: 1}, {value: 42}}}
	r := NewRunner(q)

	spec := builtSpec(t,
		validDetails(func(d *Details) { d.Operator = OperatorEq; d.Threshold = 1 }),
		validDetails(func(d *Details) { d.Operator = OperatorSup; d.Threshold = 10 }),
	)

	r.runTest(context.Background(), name, namespace, spec)

	assert.Equal(t, 1.0, metricValue(name, namespace))
	assert.Equal(t, 2, q.callIdx, "both queries must be called")
}

func TestRunTest_OneFailsFirst_MetricIsZero(t *testing.T) {
	name := "first-fails"
	namespace := "default"
	q := &mockQuerier{results: []queryResult{{value: 0}, {value: 1}}}
	r := NewRunner(q)

	spec := builtSpec(t,
		validDetails(func(d *Details) { d.Operator = OperatorEq; d.Threshold = 1 }), // 0 != 1 → fail
		validDetails(func(d *Details) { d.Operator = OperatorEq; d.Threshold = 1 }), // 1 == 1 → pass
	)

	r.runTest(context.Background(), name, namespace, spec)

	assert.Equal(t, 0.0, metricValue(name, namespace))
	assert.Equal(t, 2, q.callIdx, "all queries run even if one fails")
}

func TestRunTest_OneFailsLast_MetricIsZero(t *testing.T) {
	name := "last-fails"
	namespace := "default"
	q := &mockQuerier{results: []queryResult{{value: 1}, {value: 0}}}
	r := NewRunner(q)

	spec := builtSpec(t,
		validDetails(func(d *Details) { d.Operator = OperatorEq; d.Threshold = 1 }), // pass
		validDetails(func(d *Details) { d.Operator = OperatorEq; d.Threshold = 1 }), // 0 != 1 → fail
	)

	r.runTest(context.Background(), name, namespace, spec)

	assert.Equal(t, 0.0, metricValue(name, namespace))
}

func TestRunTest_QueryError_MetricIsZero_ContinuesOtherTests(t *testing.T) {
	name := "query-error"
	namespace := "default"
	q := &mockQuerier{results: []queryResult{
		{err: errors.New("timeout")},
		{value: 1},
		{value: 1},
	}}
	r := NewRunner(q)

	spec := builtSpec(t,
		validDetails(),
		validDetails(func(d *Details) { d.Operator = OperatorEq; d.Threshold = 1 }),
		validDetails(func(d *Details) { d.Operator = OperatorEq; d.Threshold = 1 }),
	)

	r.runTest(context.Background(), name, namespace, spec)

	assert.Equal(t, 0.0, metricValue(name, namespace))
	assert.Equal(t, 3, q.callIdx, "remaining queries still run after an error")
}

func TestRunTest_AllQueriesError_MetricIsZero(t *testing.T) {
	name := "all-errors"
	namespace := "default"
	q := &mockQuerier{results: []queryResult{
		{err: errors.New("err1")},
		{err: errors.New("err2")},
	}}
	r := NewRunner(q)

	spec := builtSpec(t, validDetails(), validDetails())

	r.runTest(context.Background(), name, namespace, spec)

	assert.Equal(t, 0.0, metricValue(name, namespace))
}

func TestRunTest_NilQuerier_MetricIsZero(t *testing.T) {
	name := "nil-querier"
	namespace := "default"
	r := NewRunner(nil)
	spec := builtSpec(t, validDetails())

	r.runTest(context.Background(), name, namespace, spec)

	assert.Equal(t, 0.0, metricValue(name, namespace))
}

func TestRunTest_ContextAlreadyCanceled_NoMetric(t *testing.T) {
	name := "ctx-canceled"
	namespace := "default"
	// Reset any prior value by setting a known sentinel first.
	metrics.ChaosTestSuccess.WithLabelValues(name, namespace).Set(99)

	q := &mockQuerier{results: []queryResult{{value: 1}}}
	r := NewRunner(q)
	spec := builtSpec(t, validDetails())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r.runTest(ctx, name, namespace, spec)

	// Metric must not have been touched (stays at sentinel).
	assert.Equal(t, 99.0, metricValue(name, namespace))
	assert.Equal(t, 0, q.callIdx, "querier must not be called when context is already canceled")
}

// ── Schedule + Stop ───────────────────────────────────────────────────────────

func TestSchedule_FiresAndSetsMetric(t *testing.T) {
	name := "schedule-fires"
	namespace := "default"
	q := &mockQuerier{results: []queryResult{{value: 1}}}
	r := NewRunner(q)

	spec := builtSpec(t, validDetails(func(d *Details) {
		d.RawWait = "10ms"
		d.Operator = OperatorEq
		d.Threshold = 1
	}))

	r.Schedule(context.Background(), name, namespace, spec)
	r.wg.Wait()

	assert.Equal(t, 1.0, metricValue(name, namespace))
}

func TestSchedule_StoppedRunner_DoesNotFire(t *testing.T) {
	name := "stopped-runner"
	namespace := "default"
	metrics.ChaosTestSuccess.WithLabelValues(name, namespace).Set(99)

	q := &mockQuerier{results: []queryResult{{value: 1}}}
	r := NewRunner(q)
	r.Stop()

	spec := builtSpec(t, validDetails(func(d *Details) { d.RawWait = "10ms" }))
	r.Schedule(context.Background(), name, namespace, spec)

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 99.0, metricValue(name, namespace), "stopped runner must not update metric")
}

func TestStop_CancelsPendingTimers(t *testing.T) {
	name := "stop-cancels"
	namespace := "default"
	metrics.ChaosTestSuccess.WithLabelValues(name, namespace).Set(99)

	q := &mockQuerier{results: []queryResult{{value: 1}}}
	r := NewRunner(q)

	spec := builtSpec(t, validDetails(func(d *Details) { d.RawWait = "10s" }))
	r.Schedule(context.Background(), name, namespace, spec)
	r.Stop() // must return before the 10s timer fires

	assert.Equal(t, 99.0, metricValue(name, namespace), "metric must not change after Stop")
	assert.Equal(t, 0, q.callIdx, "querier must not be called after Stop")
}

// ── NewRunner / HasQuerier ────────────────────────────────────────────────────

func TestNewRunner_HasQuerier(t *testing.T) {
	assert.True(t, NewRunner(&mockQuerier{}).HasQuerier())
	assert.False(t, NewRunner(nil).HasQuerier())
}

func TestHasQuerier_NilRunner(t *testing.T) {
	var r *Runner
	assert.False(t, r.HasQuerier())
}

// ── NewMiddleware ─────────────────────────────────────────────────────────────

func TestNewMiddleware_NilSpec_ReturnsNoop(t *testing.T) {
	mw, err := NewMiddleware(NewRunner(&mockQuerier{}), nil)
	require.NoError(t, err)
	require.NotNil(t, mw)
}

func TestNewMiddleware_NilRunner_ReturnsNoop(t *testing.T) {
	spec := builtSpec(t, validDetails())
	mw, err := NewMiddleware(nil, spec)
	require.NoError(t, err)
	require.NotNil(t, mw)
}

func TestNewMiddleware_NoQuerier_ReturnsError(t *testing.T) {
	spec := builtSpec(t, validDetails())
	_, err := NewMiddleware(NewRunner(nil), spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GRAFANA_URL")
}

func TestNewMiddleware_Valid_ReturnsMiddleware(t *testing.T) {
	spec := builtSpec(t, validDetails())
	mw, err := NewMiddleware(NewRunner(&mockQuerier{}), spec)
	require.NoError(t, err)
	require.NotNil(t, mw)
}
