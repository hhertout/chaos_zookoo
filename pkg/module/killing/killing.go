package killing

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/module"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Module struct {
	client       kubernetes.Interface
	namespace    string
	matchers     module.Matchers
	minAvailable int
	interval     time.Duration
}

func New(client kubernetes.Interface, cfg Config) *Module {
	return &Module{
		client:       client,
		namespace:    cfg.Namespace,
		matchers:     cfg.Matchers,
		minAvailable: cfg.MinAvailable,
		interval:     cfg.interval,
	}
}

func (m *Module) Name() string {
	return "killing"
}

func (m *Module) Interval() time.Duration {
	return m.interval
}

func (m *Module) Run(ctx context.Context) error {
	pods, err := m.collectPods(ctx)
	if err != nil {
		return err
	}

	if len(pods) == 0 {
		slog.Info("no pods found matching matchers", "namespace", m.namespace)
		return nil
	}

	total := len(pods)
	killable := total - m.minAvailable
	if killable <= 0 {
		slog.Info("all pods needed to satisfy minAvailable", "total", total, "minAvailable", m.minAvailable)
		return nil
	}

	idx := rand.IntN(len(pods))
	target := pods[idx]

	slog.Info("killing pod", "pod", target.Name, "namespace", m.namespace)
	err = m.client.CoreV1().Pods(m.namespace).Delete(ctx, target.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("deleting pod %s: %w", target.Name, err)
	}

	return nil
}
