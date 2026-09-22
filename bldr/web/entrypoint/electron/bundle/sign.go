//go:build !js

package entrypoint_electron_bundle

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/util/exec"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	"github.com/s4wave/spacewave/bldr/util/npm"
	"github.com/sirupsen/logrus"
)

// electronSignScript uses the complete signing API, including identity and
// entitlement controls unavailable in the package's CLI.
const electronSignScript = `import { sign } from '@electron/osx-sign'
await sign({
  app: process.argv[1],
  identity: process.env.BLDR_MACOS_SIGN_IDENTITY,
  identityValidation: process.env.BLDR_MACOS_SIGN_IDENTITY !== '-',
  platform: 'darwin',
  preAutoEntitlements: false,
  preEmbedProvisioningProfile: false,
})`

// SignElectron signs the branded renderer before its bytes enter the plugin
// manifest. The Electron macOS signer handles nested helpers and JIT
// entitlements; Windows uses the same signing service as native Go plugins.
// Signing is disabled when the platform's signing identity is unset.
func SignElectron(
	ctx context.Context,
	le *logrus.Entry,
	stateDir string,
	platform bldr_platform.Platform,
	electronDistPath string,
	binaryName string,
) error {
	native, ok := platform.(*bldr_platform.NativePlatform)
	if !ok {
		return nil
	}

	// Apply the configured platform signer to the final branded executable.
	binaryPath := filepath.Join(electronDistPath, binaryName)
	switch native.GetGOOS() {
	case "windows":
		return gocompiler.SignWindows(ctx, le, binaryPath)
	case "darwin":
		identity := os.Getenv(gocompiler.MacOSSignIdentityEnv)
		if identity == "" {
			return nil
		}
		toolDir := filepath.Join(stateDir, "electron-sign")
		if err := npm.EnsureBunAdd(ctx, le, stateDir, toolDir, "@electron/osx-sign@2.7.0"); err != nil {
			return err
		}
		bunPath, err := npm.ResolveBunPath(ctx, le, stateDir)
		if err != nil {
			return err
		}
		appPath := filepath.Dir(filepath.Dir(filepath.Dir(binaryPath)))
		cmd := exec.NewCmd(ctx, bunPath, "-e", electronSignScript, appPath)
		cmd.Dir = toolDir
		return exec.ExecCmd(le, cmd)
	default:
		return nil
	}
}
