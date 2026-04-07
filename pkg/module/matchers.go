package module

type Matchers struct {
	Labels          map[string]string `yaml:"labels"`
	DeploymentName  string            `yaml:"deploymentName"`
	PodName         string            `yaml:"podName"`
	DaemonsetName   string            `yaml:"daemonsetName"`
	StatefulsetName string            `yaml:"statefulsetName"`
}

func (m Matchers) IsEmpty() bool {
	return len(m.Labels) == 0 &&
		m.DeploymentName == "" &&
		m.PodName == "" &&
		m.DaemonsetName == "" &&
		m.StatefulsetName == ""
}
