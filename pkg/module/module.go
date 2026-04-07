package module

import (
	"context"
	"time"
)

type ChaosModule interface {
	Name() string
	Run(ctx context.Context) error
	Interval() time.Duration
}
