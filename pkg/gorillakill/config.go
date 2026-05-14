package gorillakill

import (
	"fmt"
	"strings"
	"time"

	gronx "github.com/adhocore/gronx"
	"github.com/hhertout/chaos_zookoo/pkg/matchers"
	"github.com/hhertout/chaos_zookoo/pkg/module"
	"gopkg.in/yaml.v3"
)

// When controls whether the module runs once or on a recurring interval.
type When string

const (
	WhenOnce     When = "once"
	WhenPeriodic When = "periodic"
)

// Config holds the parsed configuration for the gorillakill module.
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
	When        When              `yaml:"when"`
	RawInterval string            `yaml:"interval"`
	RawCron     string            `yaml:"cron"`
	RawWait     string            `yaml:"wait"`
	DryRun      bool              `yaml:"dryRun"`
	Matchers    matchers.Matchers `yaml:"matchers"`
}

// Interval exposes the resolved scenario interval (0 when when=once).
func (c Config) Interval() time.Duration { return c.interval }

// Wait exposes the resolved initial delay before the first execution.
func (c Config) Wait() time.Duration { return c.wait }

// ParseConfig unmarshals raw YAML into a validated Config.
func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing gorillakill config: %w", err)
	}

	cfg.Metadata.Name = strings.TrimSpace(cfg.Metadata.Name)
	if cfg.Metadata.Name == "" {
		return Config{}, fmt.Errorf("gorillakill config requires a name")
	}
	if cfg.Metadata.Namespace == "" {
		return Config{}, fmt.Errorf("gorillakill config requires metadata.namespace")
	}
	if cfg.Scenario.Matchers.IsEmpty() {
		return Config{}, fmt.Errorf("gorillakill config requires at least one scenario.matchers entry")
	}

	switch cfg.Scenario.When {
	case WhenOnce:
		if cfg.Scenario.RawInterval != "" {
			return Config{}, fmt.Errorf("gorillakill: scenario.interval is not valid when scenario.when=once")
		}
		if cfg.Scenario.RawCron != "" {
			return Config{}, fmt.Errorf("gorillakill: scenario.cron is not valid when scenario.when=once")
		}
	case WhenPeriodic:
		if cfg.Scenario.RawInterval != "" && cfg.Scenario.RawCron != "" {
			return Config{}, fmt.Errorf("gorillakill config: scenario.interval and scenario.cron are mutually exclusive")
		}
		switch {
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
				return Config{}, fmt.Errorf("gorillakill scenario.interval must be > 0, got %s", cfg.Scenario.RawInterval)
			}
			cfg.interval = dur
		default:
			return Config{}, fmt.Errorf("gorillakill with scenario.when=periodic requires scenario.interval or scenario.cron")
		}
	case "":
		return Config{}, fmt.Errorf("gorillakill config requires scenario.when (once|periodic)")
	default:
		return Config{}, fmt.Errorf("invalid scenario.when %q: must be once or periodic", cfg.Scenario.When)
	}

	if cfg.Scenario.RawWait != "" {
		if cfg.Scenario.When == WhenPeriodic && cfg.cronExpr != "" {
			return Config{}, fmt.Errorf("gorillakill scenario.wait is not supported with cron scheduling")
		}
		w, err := time.ParseDuration(cfg.Scenario.RawWait)
		if err != nil {
			return Config{}, fmt.Errorf("invalid scenario.wait %q: %w", cfg.Scenario.RawWait, err)
		}
		if w < 0 {
			return Config{}, fmt.Errorf("gorillakill scenario.wait must be >= 0")
		}
		if cfg.Scenario.When == WhenPeriodic && w >= cfg.interval {
			return Config{}, fmt.Errorf("gorillakill scenario.wait (%s) must be < scenario.interval (%s)", w, cfg.interval)
		}
		cfg.wait = w
	}

	return cfg, nil
}
