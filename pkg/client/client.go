package client

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// New returns a Kubernetes clientset from the given kubeconfig path and context.
// If kubeconfigPath is empty, the default discovery order applies
// ($KUBECONFIG, then ~/.kube/config, then in-cluster).
func New(kubeconfigPath, kubeCtx string) (*kubernetes.Clientset, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	if kubeCtx != "" {
		configOverrides.CurrentContext = kubeCtx
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		configOverrides,
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubeconfig: %w", err)
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}

	return cs, nil
}
