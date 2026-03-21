package version

import (
	"sort"

	"github.com/Masterminds/semver/v3"
)

// Latest returns the highest semver from a list of version strings.
// Non-semver strings are ignored. Returns "" if none are valid.
func Latest(versions []string) string {
	var parsed []*semver.Version
	for _, v := range versions {
		sv, err := semver.NewVersion(v)
		if err == nil {
			parsed = append(parsed, sv)
		}
	}
	if len(parsed) == 0 {
		return ""
	}
	sort.Sort(sort.Reverse(semver.Collection(parsed)))
	return parsed[0].Original()
}

// Behind returns the number of versions in available that are greater than deployed.
// Both must be valid semver; returns 0 on any parse error.
func Behind(deployed string, available []string) int {
	dv, err := semver.NewVersion(deployed)
	if err != nil {
		return 0
	}
	count := 0
	for _, v := range available {
		av, err := semver.NewVersion(v)
		if err != nil {
			continue
		}
		if av.GreaterThan(dv) {
			count++
		}
	}
	return count
}

// IsNewer returns true if candidate is a higher version than current.
func IsNewer(current, candidate string) bool {
	cv, err := semver.NewVersion(current)
	if err != nil {
		return false
	}
	av, err := semver.NewVersion(candidate)
	if err != nil {
		return false
	}
	return av.GreaterThan(cv)
}
