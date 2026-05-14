package nodedrain

import (
	"fmt"
	"strings"
	"time"

	gronx "github.com/adhocore/gronx"
	"github.com/hhertout/chaos_zookoo/pkg/matchers"
	"github.com/hhertout/chaos_zookoo/pkg/module"
	"gopkg.in/yaml.v3"
)

type Strategy string

const (
	StrategyEvict  Strategy = "evict"
	StrategyDelete Strategy = "delete"
)

type When string

const (
	WhenOnce     When = "once"
	WhenPeriodic When = "periodic"
)

const defaultReadinessTimeout = 5 * time.Minute

type Config struct {
	Kind     string          `yaml:"kind"`
	Name     string          `yaml:"name"`
	Metadata module.Metadata `yaml:"metadata"`
	Scenario Scenario        `yaml:"scenario"`

	interval time.Duration
	wait     time.Duration
	cronExpr string
}

func (c Config) Interval() time.Duration { return c.interval }
func (c Config) Wait() time.Duration     { return c.wait }

type Scenario struct {
	When        When   `yaml:"when"`
	RawInterval string `yaml:"interval"`
	RawCron     string `yaml:"cron"`
	RawWait     string `yaml:"wait"`
	DryRun      bool   `yaml:"dryRun"`
	Specs       Specs  `yaml:"specs"`
	Guard       Guard  `yaml:"guard"`
}

type Specs struct {
	Strategy            Strategy          `yaml:"strategy"`
	NodeSelector        map[string]string `yaml:"nodeSelector"`
	RawReadinessTimeout string            `yaml:"readinessTimeout"`
	MinReady            int               `yaml:"minReady"`

	readinessTimeout time.Duration
}

func (s Specs) ReadinessTimeout() time.Duration { return s.readinessTimeout }

type Guard struct {
	Matchers matchers.Matchers `yaml:"matchers"`
}

func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing nodedrain config: %w", err)
	}

	cfg.Name = strings.TrimSpace(cfg.Name)
	if cfg.Name == "" {
		return Config{}, fmt.Errorf("nodedrain config requires a name")
	}
	if cfg.Metadata.Namespace == "" {
		return Config{}, fmt.Errorf("nodedrain config requires metadata.namespace")
	}

	switch cfg.Scenario.When {
	case WhenOnce:
		if cfg.Scenario.RawInterval != "" {
			return Config{}, fmt.Errorf("nodedrain: scenario.interval is not valid when scenario.when=once")
		}
		if cfg.Scenario.RawCron != "" {
			return Config{}, fmt.Errorf("nodedrain: scenario.cron is not valid when scenario.when=once")
		}
	case WhenPeriodic:
		if cfg.Scenario.RawInterval != "" && cfg.Scenario.RawCron != "" {
			return Config{}, fmt.Errorf("nodedrain config: scenario.interval and scenario.cron are mutually exclusive")
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
				return Config{}, fmt.Errorf("nodedrain scenario.interval must be > 0, got %s", cfg.Scenario.RawInterval)
			}
			cfg.interval = dur
		default:
			return Config{}, fmt.Errorf("nodedrain with scenario.when=periodic requires scenario.interval or scenario.cron")
		}
	case "":
		return Config{}, fmt.Errorf("nodedrain config requires scenario.when (once|periodic)")
	default:
		return Config{}, fmt.Errorf("invalid scenario.when %q: must be once or periodic", cfg.Scenario.When)
	}

	if cfg.Scenario.RawWait != "" {
		if cfg.Scenario.When == WhenPeriodic && cfg.cronExpr != "" {
			return Config{}, fmt.Errorf("nodedrain config: scenario.wait is not supported with cron scheduling")
		}
		w, err := time.ParseDuration(cfg.Scenario.RawWait)
		if err != nil {
			return Config{}, fmt.Errorf("invalid scenario.wait %q: %w", cfg.Scenario.RawWait, err)
		}
		if w < 0 {
			return Config{}, fmt.Errorf("nodedrain config: scenario.wait must be >= 0")
		}
		if cfg.Scenario.When == WhenPeriodic && w >= cfg.interval {
			return Config{}, fmt.Errorf("nodedrain config: scenario.wait (%s) must be < scenario.interval (%s)", w, cfg.interval)
		}
		cfg.wait = w
	}

	switch cfg.Scenario.Specs.Strategy {
	case "":
		cfg.Scenario.Specs.Strategy = StrategyEvict
	case StrategyEvict, StrategyDelete:
	default:
		return Config{}, fmt.Errorf("nodedrain config: invalid specs.strategy %q: must be evict or delete", cfg.Scenario.Specs.Strategy)
	}

	if cfg.Scenario.Specs.RawReadinessTimeout != "" {
		d, err := time.ParseDuration(cfg.Scenario.Specs.RawReadinessTimeout)
		if err != nil {
			return Config{}, fmt.Errorf("nodedrain config: invalid specs.readinessTimeout %q: %w", cfg.Scenario.Specs.RawReadinessTimeout, err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("nodedrain config: specs.readinessTimeout must be > 0")
		}
		cfg.Scenario.Specs.readinessTimeout = d
	} else {
		cfg.Scenario.Specs.readinessTimeout = defaultReadinessTimeout
	}

	if cfg.Scenario.Specs.MinReady < 0 {
		return Config{}, fmt.Errorf("nodedrain config: specs.minReady must be >= 0")
	}

	return cfg, nil
}
