package gocompiler

import (
	"context"
	"os"

	uexec "github.com/aperturerobotics/util/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// WindowsSignProfileEnv is the env var holding the Azure Trusted Signing
// certificate profile name (az sign --trusted-signing-cert-profile). When
// unset, SignWindows is a no-op.
const WindowsSignProfileEnv = "BLDR_WINDOWS_SIGN_PROFILE"

// WindowsSignAccountEnv is the env var holding the Azure Trusted Signing
// account name (az sign --trusted-signing-account).
const WindowsSignAccountEnv = "BLDR_WINDOWS_SIGN_ACCOUNT"

// WindowsSignPublisherEnv is the env var holding the publisher name used
// for --publisher-name. Defaults to "Aperture Robotics LLC".
const WindowsSignPublisherEnv = "BLDR_WINDOWS_SIGN_PUBLISHER"

// defaultWindowsSignPublisher is the default publisher string when
// WindowsSignPublisherEnv is unset.
const defaultWindowsSignPublisher = "Aperture Robotics LLC"

// SignWindows signs a PE binary via Azure Trusted Signing (az sign).
//
// No-op when BLDR_WINDOWS_SIGN_PROFILE is unset. Fails when az sign exits
// non-zero. Caller is responsible for gating on GOOS=windows.
func SignWindows(ctx context.Context, le *logrus.Entry, binPath string) error {
	profile := os.Getenv(WindowsSignProfileEnv)
	if profile == "" {
		return nil
	}
	account := os.Getenv(WindowsSignAccountEnv)
	if account == "" {
		return errors.Errorf("%s is set but %s is not", WindowsSignProfileEnv, WindowsSignAccountEnv)
	}
	publisher := os.Getenv(WindowsSignPublisherEnv)
	if publisher == "" {
		publisher = defaultWindowsSignPublisher
	}
	args := []string{
		"sign",
		"--file", binPath,
		"--publisher-name", publisher,
		"--description", "Spacewave",
		"--trusted-signing-account", account,
		"--trusted-signing-cert-profile", profile,
	}
	cmd := uexec.NewCmd(ctx, "az", args...)
	cmd.Env = os.Environ()
	if err := uexec.ExecCmd(le, cmd); err != nil {
		return errors.Wrapf(err, "az sign (profile=%q, account=%q)", profile, account)
	}
	le.WithField("profile", profile).WithField("bin", binPath).Info("signed windows binary")
	return nil
}
