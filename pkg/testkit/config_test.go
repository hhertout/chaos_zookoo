package testkit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validDetails(overrides ...func(*Details)) Details {
	d := Details{
		DatasourceID: "ds-uid",
		Query:        "up",
		Threshold:    1,
	}
	for _, fn := range overrides {
		fn(&d)
	}
	return d
}

func validSpec(details ...Details) *Spec {
	if len(details) == 0 {
		details = []Details{validDetails()}
	}
	return &Spec{Client: ClientGrafana, Details: details}
}

// ── ApplyDefaultsAndValidate ─────────────────────────────────────────────────

func TestApplyDefaultsAndValidate_NilSpec(t *testing.T) {
	var s *Spec
	assert.NoError(t, s.ApplyDefaultsAndValidate(0))
}

func TestApplyDefaultsAndValidate_ClientValidation(t *testing.T) {
	tests := []struct {
		name    string
		client  ClientKind
		wantErr string
	}{
		{"missing client", "", "testing.client is required"},
		{"unsupported client", "datadog", `testing.client "datadog" unsupported`},
		{"valid client", ClientGrafana, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpec()
			s.Client = tt.client
			err := s.ApplyDefaultsAndValidate(0)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestApplyDefaultsAndValidate_EmptySpecs(t *testing.T) {
	s := &Spec{Client: ClientGrafana, Details: []Details{}}
	err := s.ApplyDefaultsAndValidate(0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one entry")
}

func TestApplyDefaultsAndValidate_SingleTest_Defaults(t *testing.T) {
	s := validSpec()
	require.NoError(t, s.ApplyDefaultsAndValidate(0))

	d := s.Details[0]
	assert.Equal(t, DefaultDatasourceKind, d.DatasourceKind)
	assert.Equal(t, DefaultOperator, d.Operator)
	assert.Equal(t, DefaultWait, d.Wait())
	assert.Equal(t, DefaultTimeWindow, d.TimeWindow())
}

func TestApplyDefaultsAndValidate_MultipleTests_AllValid(t *testing.T) {
	s := validSpec(
		validDetails(func(d *Details) { d.RawWait = "30s" }),
		validDetails(func(d *Details) { d.RawWait = "2m"; d.Operator = OperatorSup }),
		validDetails(func(d *Details) { d.RawWait = "1m"; d.Operator = OperatorInf; d.Threshold = 100 }),
	)
	require.NoError(t, s.ApplyDefaultsAndValidate(5*time.Minute))

	assert.Equal(t, 30*time.Second, s.Details[0].Wait())
	assert.Equal(t, 2*time.Minute, s.Details[1].Wait())
	assert.Equal(t, time.Minute, s.Details[2].Wait())
}

func TestApplyDefaultsAndValidate_ErrorIncludesIndex(t *testing.T) {
	s := validSpec(
		validDetails(),
		Details{DatasourceID: "", Query: ""}, // invalid: missing datasourceId
	)
	err := s.ApplyDefaultsAndValidate(0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "specs[1]")
}

func TestApplyDefaultsAndValidate_DatasourceKind(t *testing.T) {
	tests := []struct {
		name    string
		kind    DatasourceKind
		wantErr bool
	}{
		{"empty defaults to prometheus", "", false},
		{"explicit prometheus", DatasourcePrometheus, false},
		{"unsupported kind", "loki", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpec(validDetails(func(d *Details) { d.DatasourceKind = tt.kind }))
			err := s.ApplyDefaultsAndValidate(0)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestApplyDefaultsAndValidate_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Details)
		wantErr string
	}{
		{"missing datasourceId", func(d *Details) { d.DatasourceID = "" }, "datasourceId is required"},
		{"missing query", func(d *Details) { d.Query = "" }, "query is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpec(validDetails(tt.mutate))
			err := s.ApplyDefaultsAndValidate(0)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestApplyDefaultsAndValidate_Operator(t *testing.T) {
	tests := []struct {
		name    string
		op      Operator
		wantErr bool
	}{
		{"eq", OperatorEq, false},
		{"neq", OperatorNeq, false},
		{"inf", OperatorInf, false},
		{"sup", OperatorSup, false},
		{"invalid", "gte", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpec(validDetails(func(d *Details) { d.Operator = tt.op }))
			err := s.ApplyDefaultsAndValidate(0)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestApplyDefaultsAndValidate_WaitValidation(t *testing.T) {
	tests := []struct {
		name     string
		rawWait  string
		interval time.Duration
		wantErr  string
	}{
		{"valid wait", "30s", time.Minute, ""},
		{"wait equals interval", "1m", time.Minute, ""},
		{"wait exceeds interval", "2m", time.Minute, "must not exceed"},
		{"invalid duration", "not-a-duration", 0, "wait:"},
		{"zero duration", "0s", 0, "wait must be > 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpec(validDetails(func(d *Details) { d.RawWait = tt.rawWait }))
			err := s.ApplyDefaultsAndValidate(tt.interval)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestApplyDefaultsAndValidate_TimeWindowValidation(t *testing.T) {
	tests := []struct {
		name      string
		rawWindow string
		wantErr   string
	}{
		{"valid window", "5m", ""},
		{"invalid duration", "bad", "timeWindow:"},
		{"zero duration", "0s", "timeWindow must be > 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpec(validDetails(func(d *Details) { d.RawTimeWindow = tt.rawWindow }))
			err := s.ApplyDefaultsAndValidate(0)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ── MaxWait ───────────────────────────────────────────────────────────────────

func TestMaxWait(t *testing.T) {
	s := validSpec(
		validDetails(func(d *Details) { d.RawWait = "30s" }),
		validDetails(func(d *Details) { d.RawWait = "5m" }),
		validDetails(func(d *Details) { d.RawWait = "1m" }),
	)
	require.NoError(t, s.ApplyDefaultsAndValidate(10*time.Minute))
	assert.Equal(t, 5*time.Minute, s.MaxWait())
}

func TestMaxWait_SingleDetail(t *testing.T) {
	s := validSpec(validDetails(func(d *Details) { d.RawWait = "45s" }))
	require.NoError(t, s.ApplyDefaultsAndValidate(0))
	assert.Equal(t, 45*time.Second, s.MaxWait())
}

// ── Evaluate ──────────────────────────────────────────────────────────────────

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name      string
		value     float64
		op        Operator
		threshold float64
		want      bool
	}{
		{"eq pass", 1, OperatorEq, 1, true},
		{"eq fail", 2, OperatorEq, 1, false},
		{"neq pass", 2, OperatorNeq, 1, true},
		{"neq fail", 1, OperatorNeq, 1, false},
		{"inf pass", 0.5, OperatorInf, 1, true},
		{"inf fail equal", 1, OperatorInf, 1, false},
		{"inf fail greater", 2, OperatorInf, 1, false},
		{"sup pass", 2, OperatorSup, 1, true},
		{"sup fail equal", 1, OperatorSup, 1, false},
		{"sup fail less", 0.5, OperatorSup, 1, false},
		{"unknown op", 1, Operator("unknown"), 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Evaluate(tt.value, tt.op, tt.threshold))
		})
	}
}
