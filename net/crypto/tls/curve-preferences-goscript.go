//go:build goscript

package p2ptls

import (
	"crypto/tls"
	"sync"

	"github.com/sirupsen/logrus"
)

var browserCurveWarningOnce sync.Once

// browserCurvePreferences limits GoScript browser TLS to plain X25519. Go's
// default includes X25519MLKEM768, whose ML-KEM/SHAKE implementation reaches
// SHA3 operations unsupported by GoScript's numeric runtime.
func browserCurvePreferences() []tls.CurveID {
	return []tls.CurveID{tls.X25519}
}

// LogBrowserCurvePreferenceWarning logs the GoScript browser TLS limitation once.
func LogBrowserCurvePreferenceWarning(le *logrus.Entry) {
	browserCurveWarningOnce.Do(func() {
		le.Warn("GoScript browser p2p TLS uses experimental X25519-only curves; post-quantum X25519MLKEM768 is unavailable because SHA3 is unsupported")
	})
}
