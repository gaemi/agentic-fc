// Package buildinfo exposes release metadata injected by release builds.
package buildinfo

import "fmt"

var (
	Version = "dev"
	Commit  = "unknown"
)

func String(name string) string {
	return fmt.Sprintf("%s %s (%s)", name, Version, Commit)
}
