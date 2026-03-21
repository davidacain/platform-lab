package security

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func ptr[T any](v T) *T { return &v }

// makeContainer builds a container with the given security context for testing.
func makeContainer(name string, sc *corev1.SecurityContext) corev1.Container {
	return corev1.Container{Name: name, SecurityContext: sc}
}

func TestIssues_NilSecurityContext(t *testing.T) {
	f := containerFinding("default", "pod", makeContainer("app", nil), false, false, false, false)
	issues := f.Issues()

	mustContain(t, issues, "ROOT")
	mustContain(t, issues, "SECCOMP:MISSING")
}

func TestIssues_FullyHardened(t *testing.T) {
	sc := &corev1.SecurityContext{
		RunAsNonRoot:             ptr(true),
		RunAsUser:                ptr(int64(1000)),
		RunAsGroup:               ptr(int64(1000)),
		Privileged:               ptr(false),
		AllowPrivilegeEscalation: ptr(false),
		ReadOnlyRootFilesystem:   ptr(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	issues := f.Issues()

	if len(issues) != 0 {
		t.Errorf("expected no issues for hardened container, got: %v", issues)
	}
}

func TestIssues_RunAsRoot_ExplicitUID0(t *testing.T) {
	sc := &corev1.SecurityContext{
		RunAsNonRoot: ptr(true), // set but uid overrides
		RunAsUser:    ptr(int64(0)),
	}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	mustContain(t, f.Issues(), "ROOT")
}

func TestIssues_RunAsGroup_Root(t *testing.T) {
	sc := &corev1.SecurityContext{
		RunAsNonRoot: ptr(true),
		RunAsUser:    ptr(int64(1000)),
		RunAsGroup:   ptr(int64(0)),
	}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	mustContain(t, f.Issues(), "ROOT_GID")
}

func TestIssues_Privileged(t *testing.T) {
	sc := &corev1.SecurityContext{
		Privileged: ptr(true),
	}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	mustContain(t, f.Issues(), "PRIV")
}

func TestIssues_AllowEscalation_Default(t *testing.T) {
	// AllowPrivilegeEscalation defaults to true in Kubernetes when not set.
	sc := &corev1.SecurityContext{}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	mustContain(t, f.Issues(), "ESC")
}

func TestIssues_AllowEscalation_ExplicitFalse(t *testing.T) {
	sc := &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr(false),
	}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	mustNotContain(t, f.Issues(), "ESC")
}

func TestIssues_ReadOnlyRootFS_Missing(t *testing.T) {
	sc := &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr(false),
	}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	mustContain(t, f.Issues(), "NO_READONLY")
}

func TestIssues_ReadOnlyRootFS_False(t *testing.T) {
	sc := &corev1.SecurityContext{
		ReadOnlyRootFilesystem: ptr(false),
	}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	mustContain(t, f.Issues(), "NO_READONLY")
}

func TestIssues_Seccomp_Unconfined(t *testing.T) {
	sc := &corev1.SecurityContext{
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeUnconfined,
		},
	}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	mustContain(t, f.Issues(), "SECCOMP:UNCONFINED")
	mustNotContain(t, f.Issues(), "SECCOMP:MISSING")
}

func TestIssues_Seccomp_RuntimeDefault(t *testing.T) {
	sc := &corev1.SecurityContext{
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	mustNotContain(t, f.Issues(), "SECCOMP:MISSING")
	mustNotContain(t, f.Issues(), "SECCOMP:UNCONFINED")
}

func TestIssues_AddedCaps(t *testing.T) {
	sc := &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{"NET_ADMIN", "SYS_PTRACE"},
		},
	}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	mustContain(t, f.Issues(), "CAP:NET_ADMIN")
	mustContain(t, f.Issues(), "CAP:SYS_PTRACE")
}

func TestIssues_DroppedAll(t *testing.T) {
	sc := &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	mustNotContain(t, f.Issues(), "NO_DROP_ALL")
}

func TestIssues_NoDropAll(t *testing.T) {
	sc := &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"NET_RAW"},
		},
	}
	f := containerFinding("default", "pod", makeContainer("app", sc), false, false, false, false)
	mustContain(t, f.Issues(), "NO_DROP_ALL")
}

func TestIssues_HostNamespaces(t *testing.T) {
	f := containerFinding("default", "pod", makeContainer("app", nil), false, true, true, true)
	issues := f.Issues()
	mustContain(t, issues, "HOST_NET")
	mustContain(t, issues, "HOST_PID")
	mustContain(t, issues, "HOST_IPC")
}

func TestContainerDisplay_Init(t *testing.T) {
	f := Finding{Container: "setup", IsInit: true}
	if got := f.ContainerDisplay(); got != "setup(init)" {
		t.Errorf("got %q, want %q", got, "setup(init)")
	}
}

func TestContainerDisplay_Normal(t *testing.T) {
	f := Finding{Container: "app", IsInit: false}
	if got := f.ContainerDisplay(); got != "app" {
		t.Errorf("got %q, want %q", got, "app")
	}
}

// helpers

func mustContain(t *testing.T, issues []string, token string) {
	t.Helper()
	for _, i := range issues {
		if i == token {
			return
		}
	}
	t.Errorf("expected issue %q in %v", token, issues)
}

func mustNotContain(t *testing.T, issues []string, token string) {
	t.Helper()
	for _, i := range issues {
		if i == token {
			t.Errorf("unexpected issue %q in %v", token, issues)
			return
		}
	}
}
