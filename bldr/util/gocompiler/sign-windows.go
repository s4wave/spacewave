package gocompiler

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	uexec "github.com/aperturerobotics/util/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// WindowsSignProfileEnv is the env var holding the Trusted Signing
// certificate profile name (Invoke-TrustedSigning -CertificateProfileName).
// When neither this nor WindowsSignCommandEnv is set, SignWindows is a no-op.
const WindowsSignProfileEnv = "BLDR_WINDOWS_SIGN_PROFILE"

// WindowsSignCommandEnv selects an executable that signs one absolute file path
// in place before Bldr commits its manifest. The command is invoked without a
// shell and must return only after signing and verifying the file.
const WindowsSignCommandEnv = "BLDR_WINDOWS_SIGN_COMMAND"

// WindowsSignIdentityEnv identifies the external command's product and publisher
// policy for build caching. Change it when the selected signing identity changes.
const WindowsSignIdentityEnv = "BLDR_WINDOWS_SIGN_IDENTITY"

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

// signWindowsMu serializes access to the module's shared metadata file.
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
	`-TimestampDigest SHA256 ` +
	`-ExcludeManagedIdentityCredential ` +
	`-ExcludeSharedTokenCacheCredential ` +
	`-ExcludeVisualStudioCredential ` +
	`-ExcludeVisualStudioCodeCredential ` +
	`-ExcludeAzurePowerShellCredential ` +
	`-ExcludeAzureDeveloperCliCredential ` +
	`-ExcludeInteractiveBrowserCredential`

// WindowsSignStartupCacheEnvKeys returns the configuration that changes signed
// executable identity, including the explicit unsigned configuration.
func WindowsSignStartupCacheEnvKeys() []string {
	return []string{
		WindowsSignCommandEnv,
		WindowsSignIdentityEnv,
		WindowsSignProfileEnv,
		WindowsSignAccountEnv,
		WindowsSignEndpointEnv,
		WindowsSignDescriptionEnv,
	}
}

// SignWindows signs a PE binary through an external command or Azure Artifact
// Signing. External commands support signing a cross-build through a remote job.
//
// For direct Azure signing, the TrustedSigning module must be installed on the host
// (Install-Module -Name TrustedSigning). Authentication uses
// environment, workload identity, or a prior az login (azure/login@v3 in CI).
// Developer-tool and interactive credentials are excluded for unattended builds.
//
// No-op when both signing options are unset. The caller gates on GOOS=windows.
func SignWindows(ctx context.Context, le *logrus.Entry, binPath string) error {
	// A remote signer retains its own credentials and verifies the returned bytes.
	command := os.Getenv(WindowsSignCommandEnv)
	if command != "" {
		if os.Getenv(WindowsSignProfileEnv) != "" {
			return errors.New("select either a Windows signing command or an Azure profile")
		}
		if os.Getenv(WindowsSignIdentityEnv) == "" {
			return errors.Errorf("%s requires %s for build caching", WindowsSignCommandEnv, WindowsSignIdentityEnv)
		}
		absolute, err := filepath.Abs(binPath)
		if err != nil {
			return err
		}
		return errors.Wrap(uexec.ExecCmd(le, uexec.NewCmd(ctx, command, absolute)), "sign windows executable")
	}

	// Resolve the optional signing configuration before starting PowerShell.
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

	// Keep values out of command syntax and disable interactive credentials.
	cmd := uexec.NewCmd(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-Command", signWindowsScript)
	cmd.Env = append(os.Environ(),
		"BLDR_SIGN_ENDPOINT="+endpoint,
		"BLDR_SIGN_ACCOUNT="+account,
		"BLDR_SIGN_PROFILE="+profile,
		"BLDR_SIGN_FILE="+binPath,
		"BLDR_SIGN_DESCRIPTION="+description,
	)

	// The PowerShell module writes one shared metadata file for every binary.
	signWindowsMu.Lock()
	defer signWindowsMu.Unlock()
	if err := uexec.ExecCmd(le, cmd); err != nil {
		return errors.Wrapf(err, "Invoke-TrustedSigning (profile=%q, account=%q)", profile, account)
	}
	le.WithField("profile", profile).WithField("bin", binPath).Info("signed windows binary")
	return nil
}
