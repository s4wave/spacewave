package appversion

import (
	_ "embed"
	"strings"
)

//go:embed version.txt
var versionText string

// GetVersion returns the shipped runtime version string for this entrypoint.
func GetVersion() string {
	version := strings.TrimSpace(versionText)
	if version == "" {
		return "0.0.0"
	}
	return version
}
