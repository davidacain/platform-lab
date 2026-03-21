package netpol

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func pod(name, ns string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
	}
}

func policy(name, ns string, podSelector metav1.LabelSelector, types []networkingv1.PolicyType) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: types,
		},
	}
}

func TestInspect_NoPolicies(t *testing.T) {
	p := pod("web", "default", map[string]string{"app": "web"})
	cs := fake.NewSimpleClientset(p)

	findings, err := Inspect(context.Background(), cs, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.IngressCovered || f.EgressCovered {
		t.Error("expected no coverage with no policies")
	}
	if len(f.PolicyNames) != 0 {
		t.Errorf("expected no policy names, got %v", f.PolicyNames)
	}
}

func TestInspect_IngressPolicy(t *testing.T) {
	p := pod("web", "default", map[string]string{"app": "web"})
	pol := policy("allow-ingress", "default",
		metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
	)
	cs := fake.NewSimpleClientset(p, pol)

	findings, err := Inspect(context.Background(), cs, "")
	if err != nil {
		t.Fatal(err)
	}
	f := findings[0]
	if !f.IngressCovered {
		t.Error("expected ingress to be covered")
	}
	if f.EgressCovered {
		t.Error("expected egress to not be covered")
	}
	if len(f.PolicyNames) != 1 || f.PolicyNames[0] != "allow-ingress" {
		t.Errorf("unexpected policy names: %v", f.PolicyNames)
	}
}

func TestInspect_EgressPolicy(t *testing.T) {
	p := pod("web", "default", map[string]string{"app": "web"})
	pol := policy("allow-egress", "default",
		metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
	)
	cs := fake.NewSimpleClientset(p, pol)

	findings, err := Inspect(context.Background(), cs, "")
	if err != nil {
		t.Fatal(err)
	}
	f := findings[0]
	if f.IngressCovered {
		t.Error("expected ingress to not be covered")
	}
	if !f.EgressCovered {
		t.Error("expected egress to be covered")
	}
}

func TestInspect_BothPolicies(t *testing.T) {
	p := pod("web", "default", map[string]string{"app": "web"})
	ingress := policy("allow-ingress", "default",
		metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
	)
	egress := policy("allow-egress", "default",
		metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
	)
	cs := fake.NewSimpleClientset(p, ingress, egress)

	findings, err := Inspect(context.Background(), cs, "")
	if err != nil {
		t.Fatal(err)
	}
	f := findings[0]
	if !f.IngressCovered || !f.EgressCovered {
		t.Error("expected both ingress and egress to be covered")
	}
	if len(f.PolicyNames) != 2 {
		t.Errorf("expected 2 policy names, got %v", f.PolicyNames)
	}
}

func TestInspect_EmptyPodSelector_SelectsAll(t *testing.T) {
	p := pod("web", "default", map[string]string{"app": "web"})
	// Empty podSelector selects all pods in the namespace.
	pol := policy("catch-all", "default",
		metav1.LabelSelector{},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
	)
	cs := fake.NewSimpleClientset(p, pol)

	findings, err := Inspect(context.Background(), cs, "")
	if err != nil {
		t.Fatal(err)
	}
	if !findings[0].IngressCovered {
		t.Error("expected empty podSelector to select all pods")
	}
}

func TestInspect_PolicyInDifferentNamespace(t *testing.T) {
	p := pod("web", "ns-a", map[string]string{"app": "web"})
	pol := policy("allow-ingress", "ns-b",
		metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
	)
	cs := fake.NewSimpleClientset(p, pol)

	findings, err := Inspect(context.Background(), cs, "ns-a")
	if err != nil {
		t.Fatal(err)
	}
	if findings[0].IngressCovered {
		t.Error("policy in different namespace should not apply")
	}
}

func TestInspect_LabelMismatch(t *testing.T) {
	p := pod("web", "default", map[string]string{"app": "web"})
	pol := policy("other", "default",
		metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
	)
	cs := fake.NewSimpleClientset(p, pol)

	findings, err := Inspect(context.Background(), cs, "")
	if err != nil {
		t.Fatal(err)
	}
	if findings[0].IngressCovered {
		t.Error("policy with non-matching selector should not apply")
	}
}

func TestPolicyNamesDisplay_None(t *testing.T) {
	f := Finding{}
	if got := f.PolicyNamesDisplay(); got != "none" {
		t.Errorf("got %q, want %q", got, "none")
	}
}

func TestPolicyNamesDisplay_Multiple(t *testing.T) {
	f := Finding{PolicyNames: []string{"a", "b"}}
	if got := f.PolicyNamesDisplay(); got != "a, b" {
		t.Errorf("got %q, want %q", got, "a, b")
	}
}
