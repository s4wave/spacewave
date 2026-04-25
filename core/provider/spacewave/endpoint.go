//go:build !build_type_dev

package provider_spacewave

import "os"

// initEndpoint returns the cloud API endpoint from the controller config.
func initEndpoint(configured string) string {
	if env := os.Getenv("SPACEWAVE_CLOUD_BASE_URL"); env != "" {
		return env
	}
	return configured
}

// initAccountBaseURL returns the browser-facing account base URL from config.
func initAccountBaseURL(configured string) string {
	if env := os.Getenv("SPACEWAVE_CLOUD_ACCOUNT_BASE_URL"); env != "" {
		return env
	}
	return configured
}

// initPublicBaseURL returns the browser-facing public app base URL from config.
func initPublicBaseURL(configured string) string {
	if env := os.Getenv("SPACEWAVE_CLOUD_PUBLIC_BASE_URL"); env != "" {
		return env
	}
	return configured
}

// initSigningEnvPrefix returns the request-signing environment prefix from config.
func initSigningEnvPrefix(configured string) string {
	if env := os.Getenv("SPACEWAVE_CLOUD_SIGNING_ENV_PREFIX"); env != "" {
		return env
	}
	return configured
}
