package helm

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
)

// Release is a simplified view of a deployed Helm release.
type Release struct {
	Name       string
	Namespace  string
	Chart      string // chart name only, e.g. "cert-manager"
	Version    string // chart version, e.g. "1.13.0"
	AppVersion string // from chart metadata
	ImageTag   string // actual running image tag from pods
	Status     string
}

// List returns all Helm releases across namespaces (or a single namespace if specified).
func List(kubeconfig, kubeCtx, namespace string) ([]Release, error) {
	settings := cli.New()
	if kubeconfig != "" {
		settings.KubeConfig = kubeconfig
	}
	if kubeCtx != "" {
		settings.KubeContext = kubeCtx
	}

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(
		settings.RESTClientGetter(),
		namespace,
		"secrets",
		func(format string, v ...interface{}) {}, // suppress helm debug logs
	); err != nil {
		return nil, fmt.Errorf("init helm action config: %w", err)
	}

	lister := action.NewList(actionConfig)
	lister.AllNamespaces = namespace == ""
	lister.All = true

	helmReleases, err := lister.Run()
	if err != nil {
		return nil, fmt.Errorf("list helm releases: %w", err)
	}

	kube, err := newKubeClient(settings.KubeConfig, kubeCtx)
	if err != nil {
		return nil, fmt.Errorf("build kube client: %w", err)
	}

	var releases []Release
	for _, r := range helmReleases {
		rel := fromHelm(r)
		rel.ImageTag = runningImageTag(kube, r.Namespace, r.Name)
		releases = append(releases, rel)
	}
	return releases, nil
}

func fromHelm(r *release.Release) Release {
	rel := Release{
		Name:      r.Name,
		Namespace: r.Namespace,
		Status:    r.Info.Status.String(),
	}
	if r.Chart != nil && r.Chart.Metadata != nil {
		rel.Chart = r.Chart.Metadata.Name
		rel.Version = r.Chart.Metadata.Version
		rel.AppVersion = r.Chart.Metadata.AppVersion
	}
	return rel
}

// runningImageTag lists pods for the release and returns the image tag of the
// first container in the first running pod. Returns empty string on any error
// or if no pods are found.
func runningImageTag(kube *kubernetes.Clientset, namespace, releaseName string) string {
	pods, err := kube.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: "meta.helm.sh/release-name=" + releaseName,
	})
	if err != nil || len(pods.Items) == 0 {
		// Fall back to app.kubernetes.io/instance label (common Helm convention).
		pods, err = kube.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/instance=" + releaseName,
		})
		if err != nil || len(pods.Items) == 0 {
			return ""
		}
	}

	image := pods.Items[0].Spec.Containers[0].Image
	if idx := strings.LastIndex(image, ":"); idx != -1 {
		return image[idx+1:]
	}
	return image
}

func newKubeClient(kubeconfig, kubeCtx string) (*kubernetes.Clientset, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}
	configOverrides := &clientcmd.ConfigOverrides{}
	if kubeCtx != "" {
		configOverrides.CurrentContext = kubeCtx
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, configOverrides,
	).ClientConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}
