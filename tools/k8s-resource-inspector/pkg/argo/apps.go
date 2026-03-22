package argo

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var applicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

// App is a minimal representation of an ArgoCD Application CR.
type App struct {
	Name            string
	Namespace       string // spec.destination.namespace
	DestinationName string // spec.destination.name — used to look up Prometheus endpoint
	RepoURL         string // spec.source.repoURL
	TargetRevision  string // spec.source.targetRevision
	Path            string // spec.source.path — chart directory within the repo
	ValueFiles      []string // spec.source.helm.valueFiles
}

// List reads all ArgoCD Application CRs from the given namespace.
func List(ctx context.Context, dynClient dynamic.Interface, argoNamespace string) ([]App, error) {
	list, err := dynClient.Resource(applicationGVR).Namespace(argoNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list Applications in namespace %q: %w", argoNamespace, err)
	}

	apps := make([]App, 0, len(list.Items))
	for _, item := range list.Items {
		spec, _ := item.Object["spec"].(map[string]interface{})
		if spec == nil {
			continue
		}

		dest, _ := spec["destination"].(map[string]interface{})
		destName, _ := dest["name"].(string)
		destNS, _ := dest["namespace"].(string)

		// ArgoCD supports both spec.source (single) and spec.sources (multi).
		// kri only reads the primary source; skip multi-source apps with a warning.
		source, _ := spec["source"].(map[string]interface{})
		if _, hasSources := spec["sources"]; hasSources && source == nil {
			fmt.Fprintf(os.Stderr, "warn: app %q uses spec.sources (multi-source); skipping\n", item.GetName())
			continue
		}
		repoURL, _ := source["repoURL"].(string)
		targetRevision, _ := source["targetRevision"].(string)
		path, _ := source["path"].(string)

		var valueFiles []string
		if helm, ok := source["helm"].(map[string]interface{}); ok {
			if vf, ok := helm["valueFiles"].([]interface{}); ok {
				for _, f := range vf {
					if s, ok := f.(string); ok {
						valueFiles = append(valueFiles, s)
					}
				}
			}
		}

		apps = append(apps, App{
			Name:            item.GetName(),
			Namespace:       destNS,
			DestinationName: destName,
			RepoURL:         repoURL,
			TargetRevision:  targetRevision,
			Path:            path,
			ValueFiles:      valueFiles,
		})
	}

	return apps, nil
}
