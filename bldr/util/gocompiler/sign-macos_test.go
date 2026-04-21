package gocompiler

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestCodesignMacOSNoOpWhenUnset verifies that CodesignMacOS returns nil
// without invoking codesign when BLDR_MACOS_SIGN_IDENTITY is unset.
func TestCodesignMacOSNoOpWhenUnset(t *testing.T) {
	t.Setenv(MacOSSignIdentityEnv, "")
	le := logrus.NewEntry(logrus.New())
	if err := CodesignMacOS(context.Background(), le, "/nonexistent/path"); err != nil {
		t.Fatalf("expected no-op with unset env, got error: %v", err)
	}
}
