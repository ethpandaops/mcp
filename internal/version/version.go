// Package version provides build version information.
package version

// These variables are set at build time via ldflags.
var (
	// Version is the semantic version of the build.
	Version = "dev"

	// GitCommit is the git commit hash of the build.
	GitCommit = "unknown"

	// BuildTime is the time the build was created.
	BuildTime = "unknown"
)
