package rollout

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/module"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

type Module struct {
	client    kubernetes.Interface
	namespace string
	matchers  module.Matchers
	interval  time.Duration
}

type rolloutPatch struct {
	Spec rolloutPatchSpec `json:"spec"`
}

type rolloutPatchSpec struct {
	Template rolloutPatchTemplate `json:"template"`
}

type rolloutPatchTemplate struct {
	Metadata rolloutPatchMetadata `json:"metadata"`
}

type rolloutPatchMetadata struct {
	Annotations map[string]string `json:"annotations"`
}

func New(client kubernetes.Interface, cfg Config) *Module {
	return &Module{
		client:    client,
		namespace: cfg.Namespace,
		matchers:  cfg.Matchers,
		interval:  cfg.interval,
	}
}

func (m *Module) Name() string {
	return "rollout"
}

func (m *Module) Interval() time.Duration {
	return m.interval
}

func (m *Module) Run(ctx context.Context) error {
	patchBytes, err := json.Marshal(buildPatch())
	if err != nil {
		return fmt.Errorf("marshaling patch: %w", err)
	}

	if m.matchers.DeploymentName != "" {
		if err := m.restartDeployment(ctx, patchBytes); err != nil {
			return err
		}
	}
	if m.matchers.DaemonsetName != "" {
		if err := m.restartDaemonset(ctx, patchBytes); err != nil {
			return err
		}
	}
	if m.matchers.StatefulsetName != "" {
		if err := m.restartStatefulset(ctx, patchBytes); err != nil {
			return err
		}
	}

	return nil
}

func buildPatch() rolloutPatch {
	return rolloutPatch{
		Spec: rolloutPatchSpec{
			Template: rolloutPatchTemplate{
				Metadata: rolloutPatchMetadata{
					Annotations: map[string]string{
						"chaos_zookoo/restartedAt": time.Now().Format(time.RFC3339),
					},
				},
			},
		},
	}
}

func (m *Module) restartDeployment(ctx context.Context, patchBytes []byte) error {
	slog.Info("triggering rollout restart", "deployment", m.matchers.DeploymentName, "namespace", m.namespace)
	_, err := m.client.AppsV1().Deployments(m.namespace).Patch(
		ctx,
		m.matchers.DeploymentName,
		types.StrategicMergePatchType,
		patchBytes,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patching deployment %s: %w", m.matchers.DeploymentName, err)
	}
	return nil
}

func (m *Module) restartDaemonset(ctx context.Context, patchBytes []byte) error {
	slog.Info("triggering rollout restart", "daemonset", m.matchers.DaemonsetName, "namespace", m.namespace)
	_, err := m.client.AppsV1().DaemonSets(m.namespace).Patch(
		ctx,
		m.matchers.DaemonsetName,
		types.StrategicMergePatchType,
		patchBytes,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patching daemonset %s: %w", m.matchers.DaemonsetName, err)
	}
	return nil
}

func (m *Module) restartStatefulset(ctx context.Context, patchBytes []byte) error {
	slog.Info("triggering rollout restart", "statefulset", m.matchers.StatefulsetName, "namespace", m.namespace)
	_, err := m.client.AppsV1().StatefulSets(m.namespace).Patch(
		ctx,
		m.matchers.StatefulsetName,
		types.StrategicMergePatchType,
		patchBytes,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patching statefulset %s: %w", m.matchers.StatefulsetName, err)
	}
	return nil
}
