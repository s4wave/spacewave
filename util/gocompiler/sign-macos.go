package gocompiler

import (
	"context"
	"os"

	uexec "github.com/aperturerobotics/util/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// MacOSSignIdentityEnv is the env var holding the macOS code-signing identity.
// Example value: "Developer ID Application: Aperture Robotics LLC (6YCJAUQGQ6)".
// When unset, CodesignMacOS is a no-op.
const MacOSSignIdentityEnv = "BLDR_MACOS_SIGN_IDENTITY"

// MacOSSignEntitlementsEnv is the env var holding the path to an entitlements
// plist passed via codesign --entitlements. Optional.
const MacOSSignEntitlementsEnv = "BLDR_MACOS_SIGN_ENTITLEMENTS"

// MacOSSignOptionsEnv is the env var holding comma-separated codesign
// --options values. Defaults to "runtime" (hardened runtime).
const MacOSSignOptionsEnv = "BLDR_MACOS_SIGN_OPTIONS"

// defaultMacOSSignOptions is the default codesign --options value when
// MacOSSignOptionsEnv is unset. "runtime" enables hardened runtime.
const defaultMacOSSignOptions = "runtime"

// CodesignMacOS signs a Mach-O binary using codesign(1).
//
// No-op when BLDR_MACOS_SIGN_IDENTITY is unset. Fails the caller when
// codesign exits non-zero. Caller is responsible for gating on GOOS=darwin.
func CodesignMacOS(ctx context.Context, le *logrus.Entry, binPath string) error {
	identity := os.Getenv(MacOSSignIdentityEnv)
	if identity == "" {
		return nil
	}
	options := os.Getenv(MacOSSignOptionsEnv)
	if options == "" {
		options = defaultMacOSSignOptions
	}
	args := []string{
		"--force",
		"--sign", identity,
		"--options", options,
	}
	if entitlements := os.Getenv(MacOSSignEntitlementsEnv); entitlements != "" {
		args = append(args, "--entitlements", entitlements)
	}
	args = append(args, binPath)
	cmd := uexec.NewCmd(ctx, "codesign", args...)
	cmd.Env = os.Environ()
	if err := uexec.ExecCmd(le, cmd); err != nil {
		return errors.Wrapf(err, "codesign sign (identity=%q)", identity)
	}
	verifyCmd := uexec.NewCmd(ctx, "codesign", "--verify", "--strict", binPath)
	verifyCmd.Env = os.Environ()
	if err := uexec.ExecCmd(le, verifyCmd); err != nil {
		return errors.Wrapf(err, "codesign verify (identity=%q, %s=%q)", identity, MacOSSignEntitlementsEnv, os.Getenv(MacOSSignEntitlementsEnv))
	}
	le.WithField("identity", identity).WithField("bin", binPath).Info("codesigned macOS binary")
	return nil
}
