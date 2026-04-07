package main

import (
	"encoding/base64"
	"fmt"
	"os"

	"k8s.io/client-go/rest"
)

func buildK8sConfig() (*rest.Config, error) {
	host := os.Getenv("K8S_HOST")
	token := os.Getenv("K8S_TOKEN")
	certB64 := os.Getenv("K8S_CLUSTER_CERT")

	if host == "" || token == "" || certB64 == "" {
		return nil, fmt.Errorf("K8S_HOST, K8S_TOKEN and K8S_CLUSTER_CERT env vars are required")
	}

	certData, err := base64.StdEncoding.DecodeString(certB64)
	if err != nil {
		return nil, fmt.Errorf("decoding K8S_CLUSTER_CERT: %w", err)
	}

	return &rest.Config{
		Host:        host,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: certData,
		},
	}, nil
}
