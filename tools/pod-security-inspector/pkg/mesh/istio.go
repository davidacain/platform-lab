package mesh

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// MeshMode describes how (or whether) a pod is mesh-protected.
type MeshMode string

const (
	ModeNone    MeshMode = "NONE"
	ModeSidecar MeshMode = "SIDECAR"
	ModeAmbient MeshMode = "AMBIENT"
	ModeBoth    MeshMode = "BOTH"
)

// Finding represents the mesh status of a single pod.
type Finding struct {
	Namespace        string
	Pod              string
	Mode             MeshMode
	NSInject         bool   // namespace label istio-injection: enabled
	PodOptedOut      bool   // pod annotation sidecar.istio.io/inject: "false"
	SidecarReady     bool   // istio-proxy container present and ready
	ProxyVersion     string // istio-proxy image tag, or "N/A"
	AmbientNS        bool   // namespace label istio.io/dataplane-mode: ambient
	ZtunnelNode      bool   // ztunnel pod running and ready on same node as this pod
	WaypointAttached bool   // waypoint Gateway present AND namespace has use-waypoint label
}

// ambientContext holds cluster-wide ambient state fetched once per Inspect call.
type ambientContext struct {
	ztunnelNodes map[string]bool // node names with a ready ztunnel pod
	waypointNS   map[string]bool // namespaces with an attached waypoint
}

// Inspect returns one Finding per pod with full sidecar + ambient mesh status.
func Inspect(ctx context.Context, cs kubernetes.Interface, namespace string) ([]Finding, error) {
	namespaces, err := namespacesToCheck(ctx, cs, namespace)
	if err != nil {
		return nil, err
	}

	ac, err := buildAmbientContext(ctx, cs)
	if err != nil {
		return nil, err
	}

	var findings []Finding
	for _, ns := range namespaces {
		nsInject := ns.Labels["istio-injection"] == "enabled"
		ambientNS := ns.Labels["istio.io/dataplane-mode"] == "ambient"
		waypointAttached := ac.waypointNS[ns.Name]

		pods, err := cs.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list pods in %s: %w", ns.Name, err)
		}

		for _, pod := range pods.Items {
			f := podFinding(pod, nsInject, ambientNS, waypointAttached, ac)
			findings = append(findings, f)
		}
	}
	return findings, nil
}

// buildAmbientContext fetches ztunnel and waypoint state once for the whole cluster.
func buildAmbientContext(ctx context.Context, cs kubernetes.Interface) (ambientContext, error) {
	ac := ambientContext{
		ztunnelNodes: make(map[string]bool),
		waypointNS:   make(map[string]bool),
	}

	// Find nodes with a ready ztunnel pod.
	ztunnelPods, err := cs.CoreV1().Pods("istio-system").List(ctx, metav1.ListOptions{
		LabelSelector: "app=ztunnel",
	})
	if err == nil {
		for _, p := range ztunnelPods.Items {
			if isPodReady(p) {
				ac.ztunnelNodes[p.Spec.NodeName] = true
			}
		}
	}

	// Find waypoint Gateways via raw REST (avoids importing sigs.k8s.io/gateway-api).
	// If the Gateway CRD isn't installed, the call returns a non-2xx and we skip.
	waypointNSWithGateway := listWaypointNamespaces(ctx, cs)

	// A namespace is waypoint-attached only if it also has the use-waypoint label.
	if len(waypointNSWithGateway) > 0 {
		nsList, nsErr := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if nsErr == nil {
			for _, ns := range nsList.Items {
				if waypointNSWithGateway[ns.Name] {
					if _, ok := ns.Labels["istio.io/use-waypoint"]; ok {
						ac.waypointNS[ns.Name] = true
					}
				}
			}
		}
	}

	return ac, nil
}

// listWaypointNamespaces returns namespaces that contain a Gateway with the
// istio.io/waypoint label. Uses a raw REST call to avoid a gateway-api dependency.
// Returns an empty map (not an error) if the CRD is not installed.
// restClientGetter is satisfied by *kubernetes.Clientset but not by fake clients.
type restClientGetter interface {
	RESTClient() rest.Interface
}

func listWaypointNamespaces(ctx context.Context, cs kubernetes.Interface) map[string]bool {
	result := make(map[string]bool)

	rc, ok := cs.(restClientGetter)
	if !ok {
		// Fake clients (e.g. in tests) don't expose RESTClient — skip gracefully.
		return result
	}

	data, err := rc.RESTClient().Get().
		AbsPath("/apis/gateway.networking.k8s.io/v1/gateways").
		Param("labelSelector", "istio.io/waypoint").
		DoRaw(ctx)
	if err != nil {
		return result
	}

	type gatewayItem struct {
		Metadata struct {
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}
	type gatewayList struct {
		Items []gatewayItem `json:"items"`
	}

	var gl gatewayList
	if err := json.Unmarshal(data, &gl); err != nil {
		return result
	}
	for _, item := range gl.Items {
		result[item.Metadata.Namespace] = true
	}
	return result
}

func namespacesToCheck(ctx context.Context, cs kubernetes.Interface, namespace string) ([]corev1.Namespace, error) {
	if namespace != "" {
		ns, err := cs.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get namespace %s: %w", namespace, err)
		}
		return []corev1.Namespace{*ns}, nil
	}

	nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	return nsList.Items, nil
}

func podFinding(pod corev1.Pod, nsInject, ambientNS, waypointAttached bool, ac ambientContext) Finding {
	f := Finding{
		Namespace:        pod.Namespace,
		Pod:              pod.Name,
		NSInject:         nsInject,
		ProxyVersion:     "N/A",
		AmbientNS:        ambientNS,
		WaypointAttached: waypointAttached,
		ZtunnelNode:      ambientNS && ac.ztunnelNodes[pod.Spec.NodeName],
	}

	if v, ok := pod.Annotations["sidecar.istio.io/inject"]; ok && v == "false" {
		f.PodOptedOut = true
	}

	sidecarPresent := false
	for _, c := range pod.Spec.Containers {
		if c.Name == "istio-proxy" {
			sidecarPresent = true
			f.ProxyVersion = imageTag(c.Image)
			break
		}
	}

	if sidecarPresent {
		f.SidecarReady = isSidecarReady(pod)
	}

	ambientProtected := ambientNS && f.ZtunnelNode

	switch {
	case sidecarPresent && ambientProtected:
		f.Mode = ModeBoth
	case sidecarPresent:
		f.Mode = ModeSidecar
	case ambientProtected:
		f.Mode = ModeAmbient
	default:
		f.Mode = ModeNone
	}

	return f
}

func isSidecarReady(pod corev1.Pod) bool {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == "istio-proxy" {
			return cs.Ready
		}
	}
	return false
}

func isPodReady(pod corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func imageTag(image string) string {
	for i := len(image) - 1; i >= 0; i-- {
		if image[i] == ':' {
			return image[i+1:]
		}
		if image[i] == '/' {
			break
		}
	}
	return "unknown"
}
