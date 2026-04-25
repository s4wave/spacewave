//go:build !prod_signing

package resource_root

import (
	"encoding/base64"
	"os"
)

func webAppEndpoint() string {
	if endpoint := os.Getenv("SPACEWAVE_WEB_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	return "https://staging.spacewave.app"
}

func webAppAuthorization() string {
	if auth := os.Getenv("SPACEWAVE_WEB_AUTHORIZATION"); auth != "" {
		return auth
	}
	if auth := os.Getenv("SPACEWAVE_WEB_BASIC_AUTH"); auth != "" {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	}
	return ""
}
