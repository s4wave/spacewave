//go:build goscript

package p2ptls

import (
	"crypto/tls"
	"testing"
)

func TestBrowserCurvePreferencesAvoidUnsupportedSHA3(t *testing.T) {
	identity, err := NewIdentity(generateHostKey(t))
	if err != nil {
		t.Fatal(err)
	}
	got := identity.config.CurvePreferences
	if len(got) != 1 || got[0] != tls.X25519 {
		t.Fatalf("GoScript curve preferences = %v, want [X25519]", got)
	}
}
