//go:build build_type_dev

package provider_spacewave

import "os"

// initEndpoint returns the cloud API endpoint, honoring the
// SPACEWAVE_CLOUD_BASE_URL environment variable in dev builds.
func initEndpoint(configured string) string {
	if env := os.Getenv("SPACEWAVE_CLOUD_BASE_URL"); env != "" {
		return env
	}
	return configured
}

// initAccountBaseURL returns the browser-facing account base URL,
// honoring SPACEWAVE_CLOUD_ACCOUNT_BASE_URL in dev builds.
func initAccountBaseURL(configured string) string {
	if env := os.Getenv("SPACEWAVE_CLOUD_ACCOUNT_BASE_URL"); env != "" {
		return env
	}
	return configured
}

// initPublicBaseURL returns the browser-facing public app base URL,
// honoring SPACEWAVE_CLOUD_PUBLIC_BASE_URL in dev builds.
func initPublicBaseURL(configured string) string {
	if env := os.Getenv("SPACEWAVE_CLOUD_PUBLIC_BASE_URL"); env != "" {
		return env
	}
	return configured
}

// initSigningEnvPrefix returns the request-signing environment prefix,
// honoring SPACEWAVE_CLOUD_SIGNING_ENV_PREFIX in dev builds.
func initSigningEnvPrefix(configured string) string {
	if env := os.Getenv("SPACEWAVE_CLOUD_SIGNING_ENV_PREFIX"); env != "" {
		return env
	}
	return configured
}
