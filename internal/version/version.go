package version

import "fmt"

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func String() string {
	return fmt.Sprintf("clime %s (commit: %s, built: %s)", Version, GitCommit, BuildDate)
}
