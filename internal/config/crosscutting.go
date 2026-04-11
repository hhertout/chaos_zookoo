package config

import (
	"fmt"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/loadkit"
	"github.com/hhertout/chaos_zookoo/pkg/testkit"
	"gopkg.in/yaml.v3"
)

// CrossCutting holds the cross-cutting concern specs extracted from a module's
// raw YAML. These are orthogonal to the module logic itself.
type CrossCutting struct {
	Testing *testkit.Spec `yaml:"testing"`
	Load    *loadkit.Spec `yaml:"load"`
}

// ParseCrossCutting extracts and validates testing/load blocks from raw YAML.
// scenarioInterval is the module's cadence; pass 0 for once-only modules.
func ParseCrossCutting(data []byte, scenarioInterval time.Duration) (CrossCutting, error) {
	var cc CrossCutting
	if err := yaml.Unmarshal(data, &cc); err != nil {
		return CrossCutting{}, fmt.Errorf("parsing cross-cutting config: %w", err)
	}

	if err := cc.Testing.ApplyDefaultsAndValidate(scenarioInterval); err != nil {
		return CrossCutting{}, fmt.Errorf("testing: %w", err)
	}
	if err := cc.Load.ApplyDefaultsAndValidate(scenarioInterval); err != nil {
		return CrossCutting{}, fmt.Errorf("load: %w", err)
	}

	return cc, nil
}
