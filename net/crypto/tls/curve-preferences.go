//go:build !goscript

package p2ptls

import "crypto/tls"

// browserCurvePreferences returns nil so native and TinyGo TLS use Go's
// default curve preferences, including the post-quantum hybrid curves.
func browserCurvePreferences() []tls.CurveID {
	return nil
}
