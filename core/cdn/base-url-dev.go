//go:build build_type_dev

package cdn

import "os"

// BaseURL returns the CDN origin used for anonymous read artifacts. Dev
// builds honor =SPACEWAVE_CDN_BASE_URL= so the client can auto-mount against
// a staging / localhost mirror without rebuilding.
func BaseURL() string {
	if env := os.Getenv("SPACEWAVE_CDN_BASE_URL"); env != "" {
		return env
	}
	return DefaultBaseURL
}
