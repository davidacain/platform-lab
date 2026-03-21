package netpol

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// Finding represents network policy coverage for a single pod.
type Finding struct {
	Namespace     string
	Pod           string
	IngressCovered bool
	EgressCovered  bool
	PolicyNames   []string
}

// PolicyNamesDisplay returns a comma-separated list of matching policy names, or "none".
func (f *Finding) PolicyNamesDisplay() string {
	if len(f.PolicyNames) == 0 {
		return "none"
	}
	return strings.Join(f.PolicyNames, ", ")
}

func boolStr(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

// IngressDisplay returns YES/NO for ingress coverage.
func (f *Finding) IngressDisplay() string { return boolStr(f.IngressCovered) }

// EgressDisplay returns YES/NO for egress coverage.
func (f *Finding) EgressDisplay() string { return boolStr(f.EgressCovered) }

// Inspect returns one Finding per pod describing which NetworkPolicies select it
// and whether ingress/egress is covered.
func Inspect(ctx context.Context, cs kubernetes.Interface, namespace string) ([]Finding, error) {
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	policies, err := cs.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list networkpolicies: %w", err)
	}

	var findings []Finding
	for _, pod := range pods.Items {
		f := podFinding(pod, policies.Items)
		findings = append(findings, f)
	}
	return findings, nil
}

func podFinding(pod corev1.Pod, policies []networkingv1.NetworkPolicy) Finding {
	f := Finding{
		Namespace: pod.Namespace,
		Pod:       pod.Name,
	}

	podLabels := labels.Set(pod.Labels)

	for _, pol := range policies {
		if pol.Namespace != pod.Namespace {
			continue
		}

		sel, err := metav1.LabelSelectorAsSelector(&pol.Spec.PodSelector)
		if err != nil {
			continue
		}

		// An empty podSelector selects all pods in the namespace.
		if !sel.Matches(podLabels) {
			continue
		}

		f.PolicyNames = append(f.PolicyNames, pol.Name)

		for _, pType := range pol.Spec.PolicyTypes {
			switch pType {
			case networkingv1.PolicyTypeIngress:
				f.IngressCovered = true
			case networkingv1.PolicyTypeEgress:
				f.EgressCovered = true
			}
		}

		// If PolicyTypes is not set, Ingress is implied when ingress rules exist.
		if len(pol.Spec.PolicyTypes) == 0 && len(pol.Spec.Ingress) > 0 {
			f.IngressCovered = true
		}
	}

	return f
}
