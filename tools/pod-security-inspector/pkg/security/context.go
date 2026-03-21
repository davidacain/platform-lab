package security

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Finding represents a single container's security context evaluation.
type Finding struct {
	Namespace   string
	Pod         string
	Container   string
	IsInit      bool
	RunAsRoot   bool   // runAsNonRoot == false or missing
	RunAsUser   *int64 // nil if not set
	RunAsGroup  *int64 // nil if not set
	Privileged  bool
	AllowEscalation bool
	ReadOnlyRootFS  *bool  // nil if not set
	SeccompProfile  string // "runtime", "localhost", "unconfined", "MISSING"
	AddedCaps       []string
	DroppedAll      bool
	HostNetwork     bool
	HostPID         bool
	HostIPC         bool
}

// RootFlag returns a display string for the running-as-root finding.
func (f *Finding) RootFlag() string {
	if f.RunAsRoot {
		return "YES"
	}
	if f.RunAsUser != nil && *f.RunAsUser == 0 {
		return "YES(uid=0)"
	}
	if f.RunAsGroup != nil && *f.RunAsGroup == 0 {
		return "YES(gid=0)"
	}
	return "NO"
}

// CapsDisplay returns a display string for added capabilities.
func (f *Finding) CapsDisplay() string {
	if len(f.AddedCaps) == 0 {
		return "none"
	}
	return strings.Join(f.AddedCaps, ",")
}

// ReadOnlyDisplay returns a display string for readOnlyRootFilesystem.
func (f *Finding) ReadOnlyDisplay() string {
	if f.ReadOnlyRootFS == nil {
		return "MISSING"
	}
	if *f.ReadOnlyRootFS {
		return "YES"
	}
	return "NO"
}

// ContainerDisplay returns the container name with an (init) suffix if applicable.
func (f *Finding) ContainerDisplay() string {
	if f.IsInit {
		return f.Container + "(init)"
	}
	return f.Container
}

// boolStr converts a bool to YES/NO.
func boolStr(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

// Inspect lists all pods in the given namespace (or all namespaces if empty)
// and returns a Finding per container (including initContainers).
func Inspect(ctx context.Context, cs kubernetes.Interface, namespace string) ([]Finding, error) {
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	var findings []Finding
	for _, pod := range pods.Items {
		hostNet := pod.Spec.HostNetwork
		hostPID := pod.Spec.HostPID
		hostIPC := pod.Spec.HostIPC

		for _, c := range pod.Spec.InitContainers {
			findings = append(findings, containerFinding(pod.Namespace, pod.Name, c, true, hostNet, hostPID, hostIPC))
		}
		for _, c := range pod.Spec.Containers {
			findings = append(findings, containerFinding(pod.Namespace, pod.Name, c, false, hostNet, hostPID, hostIPC))
		}
	}
	return findings, nil
}

func containerFinding(ns, podName string, c corev1.Container, isInit, hostNet, hostPID, hostIPC bool) Finding {
	f := Finding{
		Namespace:   ns,
		Pod:         podName,
		Container:   c.Name,
		IsInit:      isInit,
		HostNetwork: hostNet,
		HostPID:     hostPID,
		HostIPC:     hostIPC,
	}

	sc := c.SecurityContext
	if sc == nil {
		// Everything is missing — flag root, no readonly, no seccomp
		f.RunAsRoot = true
		f.SeccompProfile = "MISSING"
		return f
	}

	// runAsNonRoot
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		f.RunAsRoot = true
	}

	f.RunAsUser = sc.RunAsUser
	f.RunAsGroup = sc.RunAsGroup

	if sc.Privileged != nil {
		f.Privileged = *sc.Privileged
	}

	if sc.AllowPrivilegeEscalation != nil {
		f.AllowEscalation = *sc.AllowPrivilegeEscalation
	} else {
		// default is true in Kubernetes
		f.AllowEscalation = true
	}

	f.ReadOnlyRootFS = sc.ReadOnlyRootFilesystem

	// Seccomp profile
	if sc.SeccompProfile == nil {
		f.SeccompProfile = "MISSING"
	} else {
		switch sc.SeccompProfile.Type {
		case corev1.SeccompProfileTypeRuntimeDefault:
			f.SeccompProfile = "runtime"
		case corev1.SeccompProfileTypeLocalhost:
			f.SeccompProfile = "localhost"
		case corev1.SeccompProfileTypeUnconfined:
			f.SeccompProfile = "UNCONFINED"
		default:
			f.SeccompProfile = string(sc.SeccompProfile.Type)
		}
	}

	// Capabilities
	if sc.Capabilities != nil {
		for _, cap := range sc.Capabilities.Add {
			f.AddedCaps = append(f.AddedCaps, string(cap))
		}
		for _, cap := range sc.Capabilities.Drop {
			if strings.EqualFold(string(cap), "ALL") {
				f.DroppedAll = true
				break
			}
		}
	}

	return f
}

// Issues returns a list of short tokens describing each problem found.
// Empty slice means the container is clean.
func (f *Finding) Issues() []string {
	var issues []string
	if f.RunAsRoot || (f.RunAsUser != nil && *f.RunAsUser == 0) {
		issues = append(issues, "ROOT")
	}
	if f.RunAsGroup != nil && *f.RunAsGroup == 0 {
		issues = append(issues, "ROOT_GID")
	}
	if f.Privileged {
		issues = append(issues, "PRIV")
	}
	if f.AllowEscalation {
		issues = append(issues, "ESC")
	}
	if f.ReadOnlyRootFS == nil {
		issues = append(issues, "NO_READONLY")
	} else if !*f.ReadOnlyRootFS {
		issues = append(issues, "NO_READONLY")
	}
	switch f.SeccompProfile {
	case "MISSING":
		issues = append(issues, "SECCOMP:MISSING")
	case "UNCONFINED":
		issues = append(issues, "SECCOMP:UNCONFINED")
	}
	for _, cap := range f.AddedCaps {
		issues = append(issues, "CAP:"+cap)
	}
	if !f.DroppedAll {
		issues = append(issues, "NO_DROP_ALL")
	}
	if f.HostNetwork {
		issues = append(issues, "HOST_NET")
	}
	if f.HostPID {
		issues = append(issues, "HOST_PID")
	}
	if f.HostIPC {
		issues = append(issues, "HOST_IPC")
	}
	return issues
}

// HasIssues returns true if the finding has any red-flag condition.
func (f *Finding) HasIssues() bool {
	return f.RunAsRoot ||
		(f.RunAsUser != nil && *f.RunAsUser == 0) ||
		(f.RunAsGroup != nil && *f.RunAsGroup == 0) ||
		f.Privileged ||
		f.AllowEscalation ||
		f.ReadOnlyRootFS == nil || !*f.ReadOnlyRootFS ||
		f.SeccompProfile == "MISSING" || f.SeccompProfile == "UNCONFINED" ||
		len(f.AddedCaps) > 0 ||
		!f.DroppedAll ||
		f.HostNetwork || f.HostPID || f.HostIPC
}
