package loadkit

import (
	"testing"
	"time"
)

func TestApplyDefaultsAndValidate_AllowsSkipTLSVerify(t *testing.T) {
	t.Parallel()

	spec := &Spec{
		Vus:           1,
		Duration:      "1s",
		SkipTLSVerify: true,
		Requests: Requests{
			Method: "GET",
			URL:    "https://example.com/healthz",
		},
	}

	err := spec.ApplyDefaultsAndValidate(5 * time.Second)
	if err != nil {
		t.Fatalf("expected skipTlsVerify to be allowed, got %v", err)
	}
}
