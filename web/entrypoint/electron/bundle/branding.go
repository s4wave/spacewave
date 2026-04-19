//go:build !js

package entrypoint_electron_bundle

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strconv"

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
// stateDir is used for downloading tools (resedit-cli on Windows).
// appName is the display name from NativeAppConfig.AppName.
// iconPath is the absolute path to the source icon PNG (empty to skip icon).
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
	iconPath string,
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
		binName, err = applyDarwinBranding(ctx, le, electronDistPath, appName, iconPath)
	case "windows":
		binName, err = applyWindowsBranding(ctx, le, electronDistPath, stateDir, appName, iconPath)
	default:
		binName, err = applyLinuxBranding(electronDistPath, appName, iconPath)
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
// If iconPath points to a PNG, converts to .icns and copies to Resources/.
func applyDarwinBranding(ctx context.Context, le *logrus.Entry, electronDistPath, appName, iconPath string) (string, error) {
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

	// Copy app icon if provided.
	if iconPath != "" {
		if err := convertAndCopyDarwinIcon(ctx, le, iconPath, contentsDir); err != nil {
			le.WithError(err).Warn("icon copy failed (non-fatal)")
		} else {
			plist = updatePlistStringValue(plist, "CFBundleIconFile", "app")
		}
	}

	// #nosec G703 -- plistPath points inside the app bundle being assembled locally.
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

// convertAndCopyDarwinIcon converts a PNG to .icns and copies to Resources/.
func convertAndCopyDarwinIcon(ctx context.Context, le *logrus.Entry, srcPng, contentsDir string) error {
	resourcesDir := filepath.Join(contentsDir, "Resources")

	// Create a temporary iconset directory.
	iconsetDir := filepath.Join(contentsDir, "app.iconset")
	if err := os.MkdirAll(iconsetDir, 0o755); err != nil {
		return errors.Wrap(err, "create iconset dir")
	}
	defer os.RemoveAll(iconsetDir)

	// Generate icon sizes using sips.
	sizes := []int{16, 32, 64, 128, 256, 512}
	for _, sz := range sizes {
		outFile := filepath.Join(iconsetDir, "icon_"+strconv.Itoa(sz)+"x"+strconv.Itoa(sz)+".png")
		// #nosec G204 -- sips is invoked with local bundle asset paths selected by the builder.
		cmd := osexec.CommandContext(ctx, "sips", "-z", strconv.Itoa(sz), strconv.Itoa(sz), srcPng, "--out", outFile)
		if out, err := cmd.CombinedOutput(); err != nil {
			return errors.Wrapf(err, "sips %dx%d: %s", sz, sz, string(out))
		}
		// Generate @2x variant.
		sz2 := sz * 2
		if sz2 <= 1024 {
			out2x := filepath.Join(iconsetDir, "icon_"+strconv.Itoa(sz)+"x"+strconv.Itoa(sz)+"@2x.png")
			// #nosec G204 -- sips is invoked with local bundle asset paths selected by the builder.
			cmd2 := osexec.CommandContext(ctx, "sips", "-z", strconv.Itoa(sz2), strconv.Itoa(sz2), srcPng, "--out", out2x)
			if out, err := cmd2.CombinedOutput(); err != nil {
				return errors.Wrapf(err, "sips %dx%d@2x: %s", sz, sz, string(out))
			}
		}
	}

	// Convert iconset to .icns.
	icnsPath := filepath.Join(resourcesDir, "app.icns")
	cmd := osexec.CommandContext(ctx, "iconutil", "-c", "icns", iconsetDir, "-o", icnsPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "iconutil: %s", string(out))
	}

	le.Debug("macOS icon converted and copied")
	return nil
}

// applyWindowsBranding runs resedit and renames the exe.
// If iconPath points to a PNG, converts to .ico and sets via resedit --icon.
//
// resedit-cli (jet2jet/resedit-js-cli) replaces the deprecated rcedit
// wrapper. Unlike rcedit, resedit does not operate in-place: it reads
// <in> and writes <out>. We emit directly to {appName}.exe and drop the
// original electron.exe when resedit succeeds; on failure we fall back
// to a plain rename so the build still produces an (unbranded) binary.
func applyWindowsBranding(ctx context.Context, le *logrus.Entry, electronDistPath, stateDir, appName, iconPath string) (string, error) {
	exePath := filepath.Join(electronDistPath, "electron.exe")
	newExePath := filepath.Join(electronDistPath, appName+".exe")

	// Build resedit args: input output --product-name X --file-description X [--icon ico].
	reseditArgs := []string{
		exePath,
		newExePath,
		"--product-name", appName,
		"--file-description", appName,
	}

	// Convert PNG to .ico if provided.
	if iconPath != "" {
		icoPath := filepath.Join(electronDistPath, "app.ico")
		if err := convertPngToIco(ctx, le, stateDir, iconPath, icoPath); err != nil {
			le.WithError(err).Warn("icon conversion failed, skipping icon")
		} else {
			reseditArgs = append(reseditArgs, "--icon", icoPath)
		}
	}

	// Run resedit-cli via bunx to produce the rebranded exe.
	edited := false
	if cmd, err := npm.BunX(ctx, le, stateDir, "resedit-cli", reseditArgs...); err != nil {
		le.WithError(err).Warn("resedit setup failed, skipping metadata edit")
	} else if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		le.WithError(err).Warn("resedit failed, skipping metadata edit")
	} else {
		edited = true
	}

	if edited {
		// resedit wrote newExePath from exePath; drop the original.
		if err := os.Remove(exePath); err != nil && !os.IsNotExist(err) {
			return "", errors.Wrap(err, "remove original electron.exe")
		}
	} else {
		// Fall back to a plain rename without metadata so the build still produces a binary.
		if err := os.Rename(exePath, newExePath); err != nil {
			return "", errors.Wrap(err, "rename electron.exe")
		}
	}

	le.Debug("Windows branding applied")
	return appName + ".exe", nil
}

// convertPngToIco converts a PNG to ICO using png-to-ico via bunx.
// png-to-ico outputs .ico to stdout, so we capture and write to file.
func convertPngToIco(ctx context.Context, le *logrus.Entry, stateDir, srcPng, destIco string) error {
	cmd, err := npm.BunX(ctx, le, stateDir, "png-to-ico", srcPng)
	if err != nil {
		return errors.Wrap(err, "setup png-to-ico")
	}
	// NewCmd presets Stdout to os.Stdout; clear it so Output() can bind its
	// own buffer (Cmd.Output refuses to run when Stdout is already set).
	cmd.Stdout = nil
	outData, err := cmd.Output()
	if err != nil {
		return errors.Wrap(err, "run png-to-ico")
	}
	return os.WriteFile(destIco, outData, 0o644)
}

// applyLinuxBranding renames the electron binary and copies the icon.
func applyLinuxBranding(electronDistPath, appName, iconPath string) (string, error) {
	oldPath := filepath.Join(electronDistPath, "electron")
	newPath := filepath.Join(electronDistPath, appName)
	if err := os.Rename(oldPath, newPath); err != nil {
		return "", errors.Wrap(err, "rename electron binary")
	}
	if err := os.Chmod(newPath, 0o755); err != nil {
		return "", errors.Wrap(err, "chmod renamed binary")
	}
	// Copy icon to resources directory if provided.
	if iconPath != "" {
		resourcesDir := filepath.Join(electronDistPath, "resources")
		if err := os.MkdirAll(resourcesDir, 0o755); err == nil {
			destIcon := filepath.Join(resourcesDir, "app.png")
			if data, err := os.ReadFile(iconPath); err == nil {
				// #nosec G703 -- destIcon points inside the local Electron dist resources directory.
				_ = os.WriteFile(destIcon, data, 0o644)
			}
		}
	}
	return appName, nil
}

// updatePlistStringValue replaces the string value for a key in an XML plist.
func updatePlistStringValue(content, key, value string) string {
	re := regexp.MustCompile(`(<key>` + regexp.QuoteMeta(key) + `</key>\s*<string>)[^<]*(</string>)`)
	return re.ReplaceAllString(content, "${1}"+value+"${2}")
}
