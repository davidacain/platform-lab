package mesh

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestImageTag(t *testing.T) {
	cases := []struct {
		image string
		want  string
	}{
		{"docker.io/istio/proxyv2:1.17.2", "1.17.2"},
		{"istio/proxyv2:1.20.0-distroless", "1.20.0-distroless"},
		{"proxyv2:latest", "latest"},
		{"proxyv2", "unknown"},
		{"registry.example.com:5000/istio/proxyv2:1.18.0", "1.18.0"},
	}
	for _, c := range cases {
		t.Run(c.image, func(t *testing.T) {
			if got := imageTag(c.image); got != c.want {
				t.Errorf("imageTag(%q) = %q, want %q", c.image, got, c.want)
			}
		})
	}
}

func TestIsSidecarReady_Ready(t *testing.T) {
	pod := corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "istio-proxy", Ready: true},
			},
		},
	}
	if !isSidecarReady(pod) {
		t.Error("expected sidecar to be ready")
	}
}

func TestIsSidecarReady_NotReady(t *testing.T) {
	pod := corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "istio-proxy", Ready: false},
			},
		},
	}
	if isSidecarReady(pod) {
		t.Error("expected sidecar to not be ready")
	}
}

func TestIsSidecarReady_NoSidecar(t *testing.T) {
	pod := corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", Ready: true},
			},
		},
	}
	if isSidecarReady(pod) {
		t.Error("expected false when no istio-proxy container present")
	}
}

func TestPodFinding_NoSidecar_NoAmbient(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName:   "node-1",
			Containers: []corev1.Container{{Name: "app"}},
		},
	}
	ac := ambientContext{
		ztunnelNodes: map[string]bool{},
		waypointNS:   map[string]bool{},
	}
	f := podFinding(pod, false, false, false, ac)

	if f.Mode != ModeNone {
		t.Errorf("expected NONE, got %s", f.Mode)
	}
	if f.ProxyVersion != "N/A" {
		t.Errorf("expected N/A proxy version, got %q", f.ProxyVersion)
	}
}

func TestPodFinding_WithSidecar(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{Name: "app"},
				{Name: "istio-proxy", Image: "istio/proxyv2:1.17.2"},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "istio-proxy", Ready: true},
			},
		},
	}
	ac := ambientContext{
		ztunnelNodes: map[string]bool{},
		waypointNS:   map[string]bool{},
	}
	f := podFinding(pod, true, false, false, ac)

	if f.Mode != ModeSidecar {
		t.Errorf("expected SIDECAR, got %s", f.Mode)
	}
	if !f.NSInject {
		t.Error("expected NSInject true")
	}
	if !f.SidecarReady {
		t.Error("expected SidecarReady true")
	}
	if f.ProxyVersion != "1.17.2" {
		t.Errorf("expected version 1.17.2, got %q", f.ProxyVersion)
	}
}

func TestPodFinding_AmbientMode(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName:   "node-1",
			Containers: []corev1.Container{{Name: "app"}},
		},
	}
	ac := ambientContext{
		ztunnelNodes: map[string]bool{"node-1": true},
		waypointNS:   map[string]bool{},
	}
	// ambientNS=true, ztunnel on node-1
	f := podFinding(pod, false, true, false, ac)

	if f.Mode != ModeAmbient {
		t.Errorf("expected AMBIENT, got %s", f.Mode)
	}
	if !f.ZtunnelNode {
		t.Error("expected ZtunnelNode true")
	}
}

func TestPodFinding_BothMode(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{Name: "app"},
				{Name: "istio-proxy", Image: "istio/proxyv2:1.17.2"},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "istio-proxy", Ready: true},
			},
		},
	}
	ac := ambientContext{
		ztunnelNodes: map[string]bool{"node-1": true},
		waypointNS:   map[string]bool{},
	}
	f := podFinding(pod, false, true, false, ac)

	if f.Mode != ModeBoth {
		t.Errorf("expected BOTH, got %s", f.Mode)
	}
}

func TestPodFinding_OptedOut(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "web",
			Namespace:   "default",
			Annotations: map[string]string{"sidecar.istio.io/inject": "false"},
		},
		Spec: corev1.PodSpec{
			NodeName:   "node-1",
			Containers: []corev1.Container{{Name: "app"}},
		},
	}
	ac := ambientContext{ztunnelNodes: map[string]bool{}, waypointNS: map[string]bool{}}
	f := podFinding(pod, true, false, false, ac)

	if !f.PodOptedOut {
		t.Error("expected PodOptedOut true")
	}
	if f.Mode != ModeNone {
		t.Errorf("expected NONE when opted out without sidecar, got %s", f.Mode)
	}
}

func TestPodFinding_AmbientNS_NoZtunnel(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName:   "node-1",
			Containers: []corev1.Container{{Name: "app"}},
		},
	}
	// namespace is ambient but ztunnel is not on this node
	ac := ambientContext{
		ztunnelNodes: map[string]bool{"node-2": true},
		waypointNS:   map[string]bool{},
	}
	f := podFinding(pod, false, true, false, ac)

	if f.Mode != ModeNone {
		t.Errorf("expected NONE when ztunnel not on pod's node, got %s", f.Mode)
	}
	if f.ZtunnelNode {
		t.Error("expected ZtunnelNode false")
	}
}
