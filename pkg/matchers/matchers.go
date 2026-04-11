package matchers

// Matchers defines the set of Kubernetes resource selectors a module can target.
type Matchers struct {
	Labels          map[string]string `yaml:"labels"`
	DeploymentName  string            `yaml:"deploymentName"`
	PodName         string            `yaml:"podName"`
	DaemonsetName   string            `yaml:"daemonsetName"`
	StatefulsetName string            `yaml:"statefulsetName"`
}

// IsEmpty reports whether no matchers are configured.
func (m Matchers) IsEmpty() bool {
	return len(m.Labels) == 0 &&
		m.DeploymentName == "" &&
		m.PodName == "" &&
		m.DaemonsetName == "" &&
		m.StatefulsetName == ""
}

// HasWorkloadTarget reports whether at least one workload-level matcher is set.
func (m Matchers) HasWorkloadTarget() bool {
	return m.DeploymentName != "" || m.DaemonsetName != "" || m.StatefulsetName != ""
}
