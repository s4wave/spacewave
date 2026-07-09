//go:build !build_type_dev

package cdn

// BaseURL returns the CDN origin used for anonymous read artifacts. Prod
// builds always return =DefaultBaseURL=; the env-var override ships only
// with dev builds (see =base-url-dev.go=).
func BaseURL() string {
	return DefaultBaseURL
}
