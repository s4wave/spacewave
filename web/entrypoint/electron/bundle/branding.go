//go:build !js

package entrypoint_electron_bundle

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"

	bldr_platform "github.com/aperturerobotics/bldr/platform"
	"github.com/aperturerobotics/bldr/util/exec"
	"github.com/aperturerobotics/bldr/util/npm"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// brandingMarkerFile is the marker file name for idempotent branding.
const brandingMarkerFile = ".bldr-branding"

// ApplyDevBranding applies NativeAppConfig branding to an extracted Electron.
//
// electronDistPath is the directory containing the extracted Electron.
// stateDir is used for downloading tools (rcedit on Windows).
// appName is the display name from NativeAppConfig.AppName.
//
// Returns the new electron binary path relative to electronDistPath.
// Idempotent: skips if marker file matches appName.
func ApplyDevBranding(
	ctx context.Context,
	le *logrus.Entry,
	electronDistPath string,
	stateDir string,
	plat bldr_platform.Platform,
	appName string,
) (string, error) {
	if appName == "" {
		return GetElectronBinName(plat), nil
	}

	// Check marker for idempotency.
	markerPath := filepath.Join(electronDistPath, brandingMarkerFile)
	if data, err := os.ReadFile(markerPath); err == nil && string(data) == appName {
		le.Debug("electron branding already applied, skipping")
		return getBrandedBinName(plat, appName), nil
	}

	np, ok := plat.(*bldr_platform.NativePlatform)
	if !ok {
		return GetElectronBinName(plat), nil
	}

	le.WithField("app-name", appName).Info("applying dev-mode branding to Electron")

	var binName string
	var err error
	switch np.GetGOOS() {
	case "darwin":
		binName, err = applyDarwinBranding(le, electronDistPath, appName)
	case "windows":
		binName, err = applyWindowsBranding(ctx, le, electronDistPath, stateDir, appName)
	default:
		binName, err = applyLinuxBranding(electronDistPath, appName)
	}
	if err != nil {
		return "", err
	}

	// Write marker file.
	if wErr := os.WriteFile(markerPath, []byte(appName), 0o644); wErr != nil {
		le.WithError(wErr).Warn("failed to write branding marker")
	}

	return binName, nil
}

// getBrandedBinName returns the expected binary path after branding.
func getBrandedBinName(plat bldr_platform.Platform, appName string) string {
	np, ok := plat.(*bldr_platform.NativePlatform)
	if !ok {
		return "electron"
	}
	switch np.GetGOOS() {
	case "darwin":
		return appName + ".app/Contents/MacOS/" + appName
	case "windows":
		return appName + ".exe"
	default:
		return appName
	}
}

// applyDarwinBranding edits Info.plist, renames the .app and binary, strips quarantine.
func applyDarwinBranding(le *logrus.Entry, electronDistPath, appName string) (string, error) {
	appDir := filepath.Join(electronDistPath, "Electron.app")
	contentsDir := filepath.Join(appDir, "Contents")
	plistPath := filepath.Join(contentsDir, "Info.plist")

	// Read and update Info.plist.
	plistData, err := os.ReadFile(plistPath)
	if err != nil {
		return "", errors.Wrap(err, "read Info.plist")
	}

	plist := string(plistData)
	plist = updatePlistStringValue(plist, "CFBundleName", appName)
	plist = updatePlistStringValue(plist, "CFBundleDisplayName", appName)
	plist = updatePlistStringValue(plist, "CFBundleExecutable", appName)

	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return "", errors.Wrap(err, "write Info.plist")
	}

	// Rename binary: Contents/MacOS/Electron -> Contents/MacOS/{appName}
	oldBin := filepath.Join(contentsDir, "MacOS", "Electron")
	newBin := filepath.Join(contentsDir, "MacOS", appName)
	if err := os.Rename(oldBin, newBin); err != nil {
		return "", errors.Wrap(err, "rename electron binary")
	}

	// Rename .app directory: Electron.app -> {appName}.app
	newAppDir := filepath.Join(electronDistPath, appName+".app")
	if err := os.Rename(appDir, newAppDir); err != nil {
		return "", errors.Wrap(err, "rename Electron.app")
	}

	// Strip quarantine xattr (ignore errors).
	xattrCmd := osexec.Command("xattr", "-dr", "com.apple.quarantine", newAppDir)
	if out, xErr := xattrCmd.CombinedOutput(); xErr != nil {
		le.WithError(xErr).WithField("output", string(out)).Debug("xattr strip (non-fatal)")
	}

	le.Debug("macOS branding applied")
	return appName + ".app/Contents/MacOS/" + appName, nil
}

// applyWindowsBranding runs rcedit and renames the exe.
func applyWindowsBranding(ctx context.Context, le *logrus.Entry, electronDistPath, stateDir, appName string) (string, error) {
	exePath := filepath.Join(electronDistPath, "electron.exe")

	// Run rcedit via bunx to set exe metadata.
	cmd, err := npm.BunX(ctx, le, stateDir, "@electron/rcedit",
		exePath,
		"--set-product-name", appName,
		"--set-file-description", appName,
	)
	if err != nil {
		le.WithError(err).Warn("rcedit setup failed, skipping metadata edit")
	} else if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		le.WithError(err).Warn("rcedit failed, skipping metadata edit")
	}

	// Rename electron.exe -> {appName}.exe
	newExePath := filepath.Join(electronDistPath, appName+".exe")
	if err := os.Rename(exePath, newExePath); err != nil {
		return "", errors.Wrap(err, "rename electron.exe")
	}

	le.Debug("Windows branding applied")
	return appName + ".exe", nil
}

// applyLinuxBranding renames the electron binary.
func applyLinuxBranding(electronDistPath, appName string) (string, error) {
	oldPath := filepath.Join(electronDistPath, "electron")
	newPath := filepath.Join(electronDistPath, appName)
	if err := os.Rename(oldPath, newPath); err != nil {
		return "", errors.Wrap(err, "rename electron binary")
	}
	if err := os.Chmod(newPath, 0o755); err != nil {
		return "", errors.Wrap(err, "chmod renamed binary")
	}
	return appName, nil
}

// updatePlistStringValue replaces the string value for a key in an XML plist.
func updatePlistStringValue(content, key, value string) string {
	re := regexp.MustCompile(`(<key>` + regexp.QuoteMeta(key) + `</key>\s*<string>)[^<]*(</string>)`)
	return re.ReplaceAllString(content, "${1}"+value+"${2}")
}
