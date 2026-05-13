package nodedrain

import (
	"fmt"

	"github.com/hhertout/chaos_zookoo/pkg/module"
	"k8s.io/client-go/kubernetes"
)

// Build satisfies module.Builder — it parses YAML and returns a ready module.
func Build(client kubernetes.Interface, data []byte) (module.ChaosModule, error) {
	cfg, err := ParseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("invalid nodedrain config: %w", err)
	}
	return New(client, cfg), nil
}
