package provider

// RepoProvider is the interface all repository backends must implement.
type RepoProvider interface {
	// Name returns the configured name of this provider (from config).
	Name() string

	// LatestVersion returns the latest available version of the given chart.
	// Returns ("", ErrNotFound) if the chart does not exist in this repo.
	LatestVersion(chart string) (string, error)

	// AllVersions returns all known versions of the given chart, sorted
	// descending (newest first). Used to compute how many versions behind
	// a deployed release is.
	AllVersions(chart string) ([]string, error)

	// AppVersionFor returns the app version associated with a specific chart
	// version. Returns an empty string if not found.
	AppVersionFor(chart, chartVersion string) string
}

// ErrNotFound is returned when a chart is not found in a repository.
type ErrNotFound struct {
	Chart string
	Repo  string
}

func (e *ErrNotFound) Error() string {
	return e.Chart + " not found in repo " + e.Repo
}
