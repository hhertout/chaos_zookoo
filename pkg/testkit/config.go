// Package testkit provides post-run verification for chaos modules by
// querying an observability backend (Grafana or Prometheus) and comparing the
// result to an expected threshold.
package testkit

import (
	"fmt"
	"time"
)

// ClientKind identifies the observability client used to evaluate a test.
type ClientKind string

const (
	ClientGrafana     ClientKind = "grafana"
	ClientPrometheus  ClientKind = "prometheus"
)

// DatasourceKind identifies the kind of datasource being queried through the client.
type DatasourceKind string

const (
	DatasourcePrometheus DatasourceKind = "prometheus"
)

// Operator compares a query result to the configured threshold.
type Operator string

const (
	OperatorEq  Operator = "eq"
	OperatorNeq Operator = "neq"
	OperatorInf Operator = "inf"
	OperatorSup Operator = "sup"
)

// Defaults applied when fields are omitted from the YAML.
const (
	DefaultWait           = time.Minute
	DefaultTimeWindow     = 10 * time.Minute
	DefaultOperator       = OperatorEq
	DefaultDatasourceKind = DatasourcePrometheus
)

// Spec is the top-level `testing:` block of a module config.
type Spec struct {
	Client  ClientKind `yaml:"client"`
	Details []Details  `yaml:"specs"`
}

// MaxWait returns the largest Wait duration across all test details.
// It is used to schedule the single evaluation timer that runs all tests,
// ensuring every test has waited at least its declared minimum before being checked.
func (s *Spec) MaxWait() time.Duration {
	var max time.Duration
	for _, d := range s.Details {
		if d.Wait() > max {
			max = d.Wait()
		}
	}
	return max
}

// Details carries the fields of a single test specification.
type Details struct {
	DatasourceKind DatasourceKind `yaml:"datasourceKind"`
	DatasourceID   string         `yaml:"datasourceId"`
	Query          string         `yaml:"query"`
	RawWait        string         `yaml:"wait"`
	RawTimeWindow  string         `yaml:"timeWindow"`
	Operator       Operator       `yaml:"operator"`
	Threshold      float64        `yaml:"threshold"`

	wait       time.Duration
	timeWindow time.Duration
}

// Wait is the delay between the module run and the test evaluation.
func (d Details) Wait() time.Duration { return d.wait }

// TimeWindow is the lookback window used when querying the datasource.
func (d Details) TimeWindow() time.Duration { return d.timeWindow }

// ApplyDefaultsAndValidate fills in missing fields and checks invariants.
// interval is the cadence at which the owning module runs; pass 0 for modules
// that run a single time so the wait-vs-interval check is skipped.
func (s *Spec) ApplyDefaultsAndValidate(interval time.Duration) error {
	if s == nil {
		return nil
	}

	if s.Client == "" {
		return fmt.Errorf("testing.client is required")
	}
	if s.Client != ClientGrafana && s.Client != ClientPrometheus {
		return fmt.Errorf("testing.client %q unsupported: must be %q or %q", s.Client, ClientGrafana, ClientPrometheus)
	}

	if len(s.Details) == 0 {
		return fmt.Errorf("testing.specs must contain at least one entry")
	}

	for i := range s.Details {
		if err := s.Details[i].applyDefaultsAndValidate(interval, s.Client); err != nil {
			return fmt.Errorf("testing.specs[%d]: %w", i, err)
		}
	}

	return nil
}

func (d *Details) applyDefaultsAndValidate(interval time.Duration, client ClientKind) error {
	if d.DatasourceKind == "" {
		d.DatasourceKind = DefaultDatasourceKind
	}
	if d.DatasourceKind != DatasourcePrometheus {
		return fmt.Errorf("datasourceKind %q unsupported: only %q is available", d.DatasourceKind, DatasourcePrometheus)
	}
	if client == ClientGrafana && d.DatasourceID == "" {
		return fmt.Errorf("datasourceId is required")
	}
	if d.Query == "" {
		return fmt.Errorf("query is required")
	}

	if d.Operator == "" {
		d.Operator = DefaultOperator
	}
	switch d.Operator {
	case OperatorEq, OperatorNeq, OperatorInf, OperatorSup:
	default:
		return fmt.Errorf("operator %q invalid: must be eq, neq, inf or sup", d.Operator)
	}

	wait, err := parseDurationOrDefault(d.RawWait, DefaultWait)
	if err != nil {
		return fmt.Errorf("wait: %w", err)
	}
	if wait <= 0 {
		return fmt.Errorf("wait must be > 0")
	}
	if interval > 0 && wait > interval {
		return fmt.Errorf("wait (%s) must not exceed the scenario interval (%s)", wait, interval)
	}
	d.wait = wait

	window, err := parseDurationOrDefault(d.RawTimeWindow, DefaultTimeWindow)
	if err != nil {
		return fmt.Errorf("timeWindow: %w", err)
	}
	if window <= 0 {
		return fmt.Errorf("timeWindow must be > 0")
	}
	d.timeWindow = window

	return nil
}

func parseDurationOrDefault(raw string, fallback time.Duration) (time.Duration, error) {
	if raw == "" {
		return fallback, nil
	}
	return time.ParseDuration(raw)
}

// Evaluate returns true if value satisfies operator/threshold.
func Evaluate(value float64, op Operator, threshold float64) bool {
	switch op {
	case OperatorEq:
		return value == threshold
	case OperatorNeq:
		return value != threshold
	case OperatorInf:
		return value < threshold
	case OperatorSup:
		return value > threshold
	}
	return false
}
