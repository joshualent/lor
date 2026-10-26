// Package version holds the build version (set with -ldflags).
package version

// Version is overridden at build time: -X github.com/joshualent/lor/internal/version.Version=...
var Version = "dev"
