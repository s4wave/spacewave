package provider_local

import (
	"net/http"
	"net/url"
)

// googleTestTransport routes only Google's test requests to offline listeners.
// It preserves every other transport target, including the provider testbed.
type googleTestTransport struct {
	// base handles each rewritten HTTP request.
	base http.RoundTripper
	// oauth replaces the OAuth token endpoint's origin.
	oauth *url.URL
	// drive replaces the Drive endpoint's origin.
	drive *url.URL
}

// RoundTrip redirects external API origins while preserving paths and queries.
func (t *googleTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	// Clone instead of mutating the OAuth transport's request or headers.
	var origin *url.URL
	switch request.URL.Hostname() {
	case "oauth2.googleapis.com":
		origin = t.oauth
	case "www.googleapis.com":
		origin = t.drive
	default:
		return t.base.RoundTrip(request)
	}
	clone := request.Clone(request.Context())
	address := *request.URL
	address.Scheme, address.Host = origin.Scheme, origin.Host
	clone.URL, clone.Host = &address, origin.Host
	return t.base.RoundTrip(clone)
}

// _ is a type assertion.
var _ http.RoundTripper = (*googleTestTransport)(nil)
