package rollout

import (
	"fmt"
	"strings"
	"time"

	gronx "github.com/adhocore/gronx"
	"github.com/hhertout/chaos_zookoo/pkg/matchers"
	"github.com/hhertout/chaos_zookoo/pkg/module"
	"gopkg.in/yaml.v3"
)

// Config holds the parsed configuration for the rollout module.
type Config struct {
	Kind     string          `yaml:"kind"`
	Metadata module.Metadata `yaml:"metadata"`
	Scenario Scenario        `yaml:"scenario"`

	interval time.Duration
	wait     time.Duration
	cronExpr string
}

// Scenario groups every field that describes the chaos action itself.
type Scenario struct {
	RawInterval string            `yaml:"interval"`
	RawCron     string            `yaml:"cron"`
	RawWait     string            `yaml:"wait"`
	DryRun      bool              `yaml:"dryRun"`
	Matchers    matchers.Matchers `yaml:"matchers"`
}

// Interval exposes the resolved scenario interval.
func (c Config) Interval() time.Duration { return c.interval }

// Wait exposes the resolved initial delay before the first execution.
func (c Config) Wait() time.Duration { return c.wait }

// ParseConfig unmarshals raw YAML into a validated Config.
func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing rollout config: %w", err)
	}

	cfg.Metadata.Name = strings.TrimSpace(cfg.Metadata.Name)
	if cfg.Metadata.Name == "" {
		return Config{}, fmt.Errorf("rollout config requires metadata.name")
	}
	if cfg.Metadata.Namespace == "" {
		return Config{}, fmt.Errorf("rollout config requires metadata.namespace")
	}
	if !cfg.Scenario.Matchers.HasWorkloadTarget() {
		return Config{}, fmt.Errorf("rollout config requires at least one of scenario.matchers.deploymentName, daemonsetName, or statefulsetName")
	}

	switch {
	case cfg.Scenario.RawInterval != "" && cfg.Scenario.RawCron != "":
		return Config{}, fmt.Errorf("rollout config: scenario.interval and scenario.cron are mutually exclusive")
	case cfg.Scenario.RawCron != "":
		if !gronx.New().IsValid(cfg.Scenario.RawCron) {
			return Config{}, fmt.Errorf("invalid scenario.cron expression %q", cfg.Scenario.RawCron)
		}
		cfg.cronExpr = cfg.Scenario.RawCron
	case cfg.Scenario.RawInterval != "":
		dur, err := time.ParseDuration(cfg.Scenario.RawInterval)
		if err != nil {
			return Config{}, fmt.Errorf("invalid scenario.interval %q: %w", cfg.Scenario.RawInterval, err)
		}
		if dur <= 0 {
			return Config{}, fmt.Errorf("rollout scenario.interval must be > 0, got %s", cfg.Scenario.RawInterval)
		}
		cfg.interval = dur
	default:
		return Config{}, fmt.Errorf("rollout config requires scenario.interval or scenario.cron")
	}

	if cfg.Scenario.RawWait != "" {
		if cfg.cronExpr != "" {
			return Config{}, fmt.Errorf("rollout scenario.wait is not supported with cron scheduling")
		}
		w, err := time.ParseDuration(cfg.Scenario.RawWait)
		if err != nil {
			return Config{}, fmt.Errorf("invalid scenario.wait %q: %w", cfg.Scenario.RawWait, err)
		}
		if w < 0 {
			return Config{}, fmt.Errorf("rollout scenario.wait must be >= 0")
		}
		if w >= cfg.interval {
			return Config{}, fmt.Errorf("rollout scenario.wait (%s) must be < scenario.interval (%s)", w, cfg.interval)
		}
		cfg.wait = w
	}

	return cfg, nil
}
