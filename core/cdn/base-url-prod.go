//go:build !build_type_dev

package cdn

import "os"

// BaseURL returns the CDN origin used for anonymous read artifacts.
func BaseURL() string {
	if env := os.Getenv("SPACEWAVE_CDN_BASE_URL"); env != "" {
		return env
	}
	return DefaultBaseURL
}
