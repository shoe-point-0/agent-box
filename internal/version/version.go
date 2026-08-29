package version

import "fmt"

// Values are replaced with -ldflags by production builds.
var (
	Version   = "devel"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info is the build identity reported by an Agent Box binary.
type Info struct {
	Version   string
	Commit    string
	BuildDate string
}

// Current returns the build identity linked into the current binary.
func Current() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}
}

// String formats a one-line build identity for a named binary.
func (i Info) String(name string) string {
	return fmt.Sprintf("%s %s (commit %s, built %s)", name, i.Version, i.Commit, i.BuildDate)
}
