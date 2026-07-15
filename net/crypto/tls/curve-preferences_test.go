//go:build !goscript

package p2ptls

import "testing"

func TestBrowserCurvePreferencesKeepNativeDefaults(t *testing.T) {
	identity, err := NewIdentity(generateHostKey(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := identity.config.CurvePreferences; got != nil {
		t.Fatalf("native curve preferences = %v, want nil", got)
	}
}
