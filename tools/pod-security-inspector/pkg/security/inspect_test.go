package security

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestInspect_AllNamespaces(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", SecurityContext: nil},
			},
		},
	}

	cs := fake.NewSimpleClientset(pod)
	findings, err := Inspect(context.Background(), cs, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Pod != "web" {
		t.Errorf("unexpected pod name %q", findings[0].Pod)
	}
}

func TestInspect_NamespaceFilter(t *testing.T) {
	podA := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "ns-a"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	podB := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: "ns-b"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}

	cs := fake.NewSimpleClientset(podA, podB)
	findings, err := Inspect(context.Background(), cs, "ns-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Namespace != "ns-a" {
		t.Errorf("unexpected namespace %q", findings[0].Namespace)
	}
}

func TestInspect_InitContainers(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: "init"}},
			Containers:     []corev1.Container{{Name: "app"}},
		},
	}

	cs := fake.NewSimpleClientset(pod)
	findings, err := Inspect(context.Background(), cs, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings (1 init + 1 container), got %d", len(findings))
	}
	if !findings[0].IsInit {
		t.Error("expected first finding to be an init container")
	}
	if findings[1].IsInit {
		t.Error("expected second finding to not be an init container")
	}
}

func TestInspect_HostNamespacesPropagate(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.PodSpec{
			HostNetwork: true,
			HostPID:     true,
			Containers:  []corev1.Container{{Name: "app"}},
		},
	}

	cs := fake.NewSimpleClientset(pod)
	findings, err := Inspect(context.Background(), cs, "")
	if err != nil {
		t.Fatal(err)
	}
	if !findings[0].HostNetwork {
		t.Error("expected HostNetwork to be true")
	}
	if !findings[0].HostPID {
		t.Error("expected HostPID to be true")
	}
}
