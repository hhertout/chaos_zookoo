package rollout

import (
	"fmt"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/module"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Namespace   string          `yaml:"namespace"`
	RawInterval string          `yaml:"interval"`
	Matchers    module.Matchers `yaml:"matchers"`
	interval    time.Duration
}

func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing rollout config: %w", err)
	}

	dur, err := time.ParseDuration(cfg.RawInterval)
	if err != nil {
		return Config{}, fmt.Errorf("invalid interval %q: %w", cfg.RawInterval, err)
	}
	cfg.interval = dur

	if cfg.Namespace == "" {
		return Config{}, fmt.Errorf("rollout config requires a namespace")
	}
	if cfg.Matchers.DeploymentName == "" && cfg.Matchers.DaemonsetName == "" && cfg.Matchers.StatefulsetName == "" {
		return Config{}, fmt.Errorf("rollout config requires at least one of deploymentName, daemonsetName, or statefulsetName matcher")
	}

	return cfg, nil
}

func (c Config) GetInterval() time.Duration {
	return c.interval
}
