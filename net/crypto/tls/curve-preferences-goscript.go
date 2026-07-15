//go:build goscript

package p2ptls

import "crypto/tls"

// browserCurvePreferences limits GoScript browser TLS to plain X25519. Go's
// default includes X25519MLKEM768, whose ML-KEM/SHAKE implementation reaches
// SHA3 operations unsupported by GoScript's numeric runtime.
func browserCurvePreferences() []tls.CurveID {
	return []tls.CurveID{tls.X25519}
}
