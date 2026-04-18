//go:build darwin

package gocompiler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestCodesignMacOSAdHocIntegration builds a minimal Go binary and signs it
// with an ad-hoc identity ("-") via CodesignMacOS, then verifies the signature
// using codesign --verify. Exercises the full sign + verify path without
// requiring a Developer ID cert.
func TestCodesignMacOSAdHocIntegration(t *testing.T) {
	if _, err := exec.LookPath("codesign"); err != nil {
		t.Skip("codesign not available")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	tempDir := t.TempDir()
	mainGo := filepath.Join(tempDir, "main.go")
	src := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(mainGo, []byte(src), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	binPath := filepath.Join(tempDir, "testbin")
	build := exec.Command("go", "build", "-o", binPath, mainGo)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	t.Setenv(MacOSSignIdentityEnv, "-")
	le := logrus.NewEntry(logrus.New())
	if err := CodesignMacOS(context.Background(), le, binPath); err != nil {
		t.Fatalf("CodesignMacOS: %v", err)
	}

	verify := exec.Command("codesign", "--verify", "--strict", binPath)
	if out, err := verify.CombinedOutput(); err != nil {
		t.Fatalf("codesign --verify: %v\n%s", err, out)
	}
}
