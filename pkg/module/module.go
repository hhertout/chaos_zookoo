package module

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"
)

// ScheduleMode controls how the orchestrator runs a module.
type ScheduleMode int

const (
	SchedulePeriodic ScheduleMode = iota
	ScheduleOnce
	ScheduleCron
)

// Schedule describes when a module should be executed by the orchestrator.
type Schedule struct {
	Mode         ScheduleMode
	Interval     time.Duration // SchedulePeriodic only
	InitialDelay time.Duration // SchedulePeriodic and ScheduleOnce
	CronExpr     string        // ScheduleCron only
}

// Metadata carries namespace-level targeting shared by all modules.
type Metadata struct {
	Namespace string `yaml:"namespace"`
}

// ChaosModule defines the contract every chaos engineering module must satisfy.
type ChaosModule interface {
	Name() string
	Kind() string
	Namespace() string
	Run(ctx context.Context) error
	Schedule() Schedule
}

// Builder constructs a ChaosModule from raw YAML and a Kubernetes client.
// Every module package exposes a Build function matching this signature.
type Builder func(client kubernetes.Interface, data []byte) (ChaosModule, error)

// Middleware wraps a ChaosModule, returning a decorated version.
// Middlewares are chained to add cross-cutting concerns (testing, load, etc.)
// without the module knowing about them.
type Middleware func(ChaosModule) ChaosModule
