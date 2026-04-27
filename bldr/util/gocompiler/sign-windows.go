package gocompiler

import (
	"context"
	"os"
	"sync"

	uexec "github.com/aperturerobotics/util/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// WindowsSignProfileEnv is the env var holding the Trusted Signing
// certificate profile name (Invoke-TrustedSigning -CertificateProfileName).
// When unset, SignWindows is a no-op.
const WindowsSignProfileEnv = "BLDR_WINDOWS_SIGN_PROFILE"

// WindowsSignAccountEnv is the env var holding the Trusted Signing signing
// account name (Invoke-TrustedSigning -CodeSigningAccountName).
const WindowsSignAccountEnv = "BLDR_WINDOWS_SIGN_ACCOUNT"

// WindowsSignEndpointEnv is the env var holding the regional Trusted
// Signing endpoint URL, e.g. https://wus.codesigning.azure.net/
// (Invoke-TrustedSigning -Endpoint). Defaults to defaultWindowsSignEndpoint
// when unset.
const WindowsSignEndpointEnv = "BLDR_WINDOWS_SIGN_ENDPOINT"

// defaultWindowsSignEndpoint is the default Trusted Signing endpoint when
// WindowsSignEndpointEnv is unset. West US 2 is where our signing account
// lives; override via env for accounts in other regions.
const defaultWindowsSignEndpoint = "https://wus.codesigning.azure.net/"

// WindowsSignDescriptionEnv is the env var holding the Authenticode
// signature description (Invoke-TrustedSigning -Description). Defaults
// to "Spacewave" when unset.
const WindowsSignDescriptionEnv = "BLDR_WINDOWS_SIGN_DESCRIPTION"

// defaultWindowsSignDescription is the default Authenticode description
// when WindowsSignDescriptionEnv is unset.
const defaultWindowsSignDescription = "Spacewave"

var signWindowsMu sync.Mutex

// signWindowsScript is the PowerShell script driving the signing call.
// Values flow in via env vars to avoid PowerShell quoting hazards.
const signWindowsScript = `$ErrorActionPreference = 'Stop'
Invoke-TrustedSigning ` +
	`-Endpoint $env:BLDR_SIGN_ENDPOINT ` +
	`-CodeSigningAccountName $env:BLDR_SIGN_ACCOUNT ` +
	`-CertificateProfileName $env:BLDR_SIGN_PROFILE ` +
	`-Files $env:BLDR_SIGN_FILE ` +
	`-Description $env:BLDR_SIGN_DESCRIPTION ` +
	`-FileDigest SHA256 ` +
	`-TimestampRfc3161 'http://timestamp.acs.microsoft.com' ` +
	`-TimestampDigest SHA256`

// SignWindows signs a PE binary via Azure Trusted Signing using the
// Invoke-TrustedSigning cmdlet from the TrustedSigning PowerShell module.
//
// The TrustedSigning module must be installed on the host
// (Install-Module -Name TrustedSigning). Authentication uses
// DefaultAzureCredential, so a prior `az login` (or azure/login@v3 in CI)
// is required.
//
// No-op when BLDR_WINDOWS_SIGN_PROFILE is unset. Caller is responsible
// for gating on GOOS=windows.
func SignWindows(ctx context.Context, le *logrus.Entry, binPath string) error {
	profile := os.Getenv(WindowsSignProfileEnv)
	if profile == "" {
		return nil
	}
	account := os.Getenv(WindowsSignAccountEnv)
	if account == "" {
		return errors.Errorf("%s is set but %s is not", WindowsSignProfileEnv, WindowsSignAccountEnv)
	}
	endpoint := os.Getenv(WindowsSignEndpointEnv)
	if endpoint == "" {
		endpoint = defaultWindowsSignEndpoint
	}
	description := os.Getenv(WindowsSignDescriptionEnv)
	if description == "" {
		description = defaultWindowsSignDescription
	}
	cmd := uexec.NewCmd(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-Command", signWindowsScript)
	cmd.Env = append(os.Environ(),
		"BLDR_SIGN_ENDPOINT="+endpoint,
		"BLDR_SIGN_ACCOUNT="+account,
		"BLDR_SIGN_PROFILE="+profile,
		"BLDR_SIGN_FILE="+binPath,
		"BLDR_SIGN_DESCRIPTION="+description,
	)
	signWindowsMu.Lock()
	defer signWindowsMu.Unlock()
	if err := uexec.ExecCmd(le, cmd); err != nil {
		return errors.Wrapf(err, "Invoke-TrustedSigning (profile=%q, account=%q)", profile, account)
	}
	le.WithField("profile", profile).WithField("bin", binPath).Info("signed windows binary")
	return nil
}
