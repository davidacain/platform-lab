package version

import (
	"testing"
)

func TestLatest(t *testing.T) {
	cases := []struct {
		versions []string
		want     string
	}{
		{[]string{"1.0.0", "2.0.0", "1.5.0"}, "2.0.0"},
		{[]string{"1.0.0"}, "1.0.0"},
		{[]string{}, ""},
		{[]string{"not-semver", "1.0.0"}, "1.0.0"},
		{[]string{"v1.2.0", "v1.10.0", "v1.3.0"}, "v1.10.0"},
	}
	for _, c := range cases {
		got := Latest(c.versions)
		if got != c.want {
			t.Errorf("Latest(%v) = %q, want %q", c.versions, got, c.want)
		}
	}
}

func TestBehind(t *testing.T) {
	available := []string{"1.0.0", "1.1.0", "1.2.0", "2.0.0"}
	cases := []struct {
		deployed string
		want     int
	}{
		{"1.0.0", 3},
		{"1.2.0", 1},
		{"2.0.0", 0},
		{"not-semver", 0},
	}
	for _, c := range cases {
		got := Behind(c.deployed, available)
		if got != c.want {
			t.Errorf("Behind(%q, ...) = %d, want %d", c.deployed, got, c.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current   string
		candidate string
		want      bool
	}{
		{"1.0.0", "1.1.0", true},
		{"1.1.0", "1.0.0", false},
		{"1.0.0", "1.0.0", false},
		{"not-semver", "1.0.0", false},
		{"1.0.0", "not-semver", false},
	}
	for _, c := range cases {
		got := IsNewer(c.current, c.candidate)
		if got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.candidate, got, c.want)
		}
	}
}
