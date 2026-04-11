package rollout

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/matchers"
	"github.com/hhertout/chaos_zookoo/pkg/module"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

var _ module.ChaosModule = (*Module)(nil)

type workloadPatcher func(ctx context.Context, client kubernetes.Interface, namespace, name string, patch []byte) error

type workloadTarget struct {
	kind    string
	name    string
	patcher workloadPatcher
}

type restartPatch struct {
	Spec restartPatchSpec `json:"spec"`
}

type restartPatchSpec struct {
	Template restartPatchTemplate `json:"template"`
}

type restartPatchTemplate struct {
	Metadata restartPatchMetadata `json:"metadata"`
}

type restartPatchMetadata struct {
	Annotations map[string]string `json:"annotations"`
}

var patchDeployment workloadPatcher = func(ctx context.Context, client kubernetes.Interface, ns, name string, patch []byte) error {
	_, err := client.AppsV1().Deployments(ns).Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	return err
}

var patchDaemonSet workloadPatcher = func(ctx context.Context, client kubernetes.Interface, ns, name string, patch []byte) error {
	_, err := client.AppsV1().DaemonSets(ns).Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	return err
}

var patchStatefulSet workloadPatcher = func(ctx context.Context, client kubernetes.Interface, ns, name string, patch []byte) error {
	_, err := client.AppsV1().StatefulSets(ns).Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	return err
}

// Module triggers rollout restarts on configured workload resources.
type Module struct {
	client    kubernetes.Interface
	name      string
	namespace string
	matchers  matchers.Matchers
	dryRun    bool
	interval  time.Duration
	wait      time.Duration
	cronExpr  string
}

// New creates a rollout module from the given client and config.
func New(client kubernetes.Interface, cfg Config) *Module {
	return &Module{
		client:    client,
		name:      cfg.Name,
		namespace: cfg.Metadata.Namespace,
		matchers:  cfg.Scenario.Matchers,
		dryRun:    cfg.Scenario.DryRun,
		interval:  cfg.interval,
		wait:      cfg.wait,
		cronExpr:  cfg.cronExpr,
	}
}

func (m *Module) Name() string      { return m.name }
func (m *Module) Kind() string      { return "Rollout" }
func (m *Module) Namespace() string { return m.namespace }
func (m *Module) Schedule() module.Schedule {
	if m.cronExpr != "" {
		return module.Schedule{Mode: module.ScheduleCron, CronExpr: m.cronExpr}
	}
	return module.Schedule{Mode: module.SchedulePeriodic, Interval: m.interval, InitialDelay: m.wait}
}

// Run patches each targeted workload with a restart annotation.
func (m *Module) Run(ctx context.Context) error {
	time.Sleep(m.wait)

	patch := restartPatch{
		Spec: restartPatchSpec{Template: restartPatchTemplate{
			Metadata: restartPatchMetadata{
				Annotations: map[string]string{
					"chaos-zookoo/restartedAt": time.Now().Format(time.RFC3339),
				},
			},
		}},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling patch: %w", err)
	}

	targets := m.collectTargets()
	for _, t := range targets {
		zap.L().Info("rollout restart",
			zap.String("kind", "rollout"),
			zap.String("name", m.name),
			zap.String("resource", t.kind),
			zap.String("target", t.name),
			zap.String("namespace", m.namespace),
			zap.Bool("dryRun", m.dryRun),
		)
		if m.dryRun {
			continue
		}
		if err := t.patcher(ctx, m.client, m.namespace, t.name, patchBytes); err != nil {
			return fmt.Errorf("patching %s %s: %w", t.kind, t.name, err)
		}
	}
	return nil
}

func (m *Module) collectTargets() []workloadTarget {
	var targets []workloadTarget
	if n := m.matchers.DeploymentName; n != "" {
		targets = append(targets, workloadTarget{kind: "Deployment", name: n, patcher: patchDeployment})
	}
	if n := m.matchers.DaemonsetName; n != "" {
		targets = append(targets, workloadTarget{kind: "DaemonSet", name: n, patcher: patchDaemonSet})
	}
	if n := m.matchers.StatefulsetName; n != "" {
		targets = append(targets, workloadTarget{kind: "StatefulSet", name: n, patcher: patchStatefulSet})
	}
	return targets
}
