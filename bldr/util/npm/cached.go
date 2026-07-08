package npm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/util/fsutil"
	"github.com/s4wave/spacewave/bldr/util/exec"
	"github.com/sirupsen/logrus"
)

// installHashFile is the filename used to cache the install hash.
const installHashFile = ".bldr-install-hash"

// EnsureBunInstall copies srcPackageJson and its sibling bun.lock, when
// present, to targetDir and runs bun install, skipping the install if the
// package manifest contents have not changed since the last successful install.
func EnsureBunInstall(ctx context.Context, le *logrus.Entry, stateDir, srcPackageJson, targetDir string) error {
	data, err := os.ReadFile(srcPackageJson)
	if err != nil {
		return err
	}
	lockData, lockFound, err := readSiblingBunLock(srcPackageJson)
	if err != nil {
		return err
	}

	hash := bunInstallHash(data, lockData)
	if installCurrent(targetDir, hash) {
		le.Debug("bun install cached, skipping")
		return nil
	}

	if err := fsutil.CleanCreateDir(targetDir); err != nil {
		return err
	}
	// #nosec G703 -- targetDir is a managed cache directory created by CleanCreateDir above.
	if err := os.WriteFile(filepath.Join(targetDir, "package.json"), data, 0o644); err != nil {
		return err
	}
	if lockFound {
		// #nosec G703 -- targetDir is a managed cache directory created by CleanCreateDir above.
		if err := os.WriteFile(filepath.Join(targetDir, "bun.lock"), lockData, 0o644); err != nil {
			return err
		}
	}

	installArgs := []string{"--cwd", targetDir}
	if lockFound {
		installArgs = append(installArgs, "--frozen-lockfile")
	}
	cmd, err := BunInstall(ctx, le, stateDir, installArgs...)
	if err != nil {
		return err
	}
	if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		return err
	}

	return writeInstallHash(targetDir, hash)
}

// EnsureNodeModulesLink links parentDir/node_modules to installDir/node_modules.
func EnsureNodeModulesLink(parentDir, installDir string) error {
	target, err := filepath.Abs(filepath.Join(installDir, "node_modules"))
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err != nil {
		return err
	}
	linkPath := filepath.Join(parentDir, "node_modules")
	info, err := os.Lstat(linkPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		existing, err := os.Readlink(linkPath)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(existing) {
			existing = filepath.Join(parentDir, existing)
		}
		existing, err = filepath.Abs(existing)
		if err != nil {
			return err
		}
		if _, err := os.Stat(existing); err == nil {
			return nil
		}
		if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(target, linkPath); err != nil {
		if !os.IsExist(err) {
			return err
		}
		if _, err := os.Lstat(linkPath); err != nil {
			return err
		}
	}
	return nil
}

func readSiblingBunLock(srcPackageJson string) ([]byte, bool, error) {
	lockPath := filepath.Join(filepath.Dir(srcPackageJson), "bun.lock")
	data, err := os.ReadFile(lockPath)
	if err == nil {
		return data, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, err
}

func bunInstallHash(packageJSON, bunLock []byte) string {
	if bunLock == nil {
		return sha256Hex(packageJSON)
	}
	data := make([]byte, 0, len(packageJSON)+len(bunLock)+len("\x00bun.lock\x00"))
	data = append(data, packageJSON...)
	data = append(data, "\x00bun.lock\x00"...)
	data = append(data, bunLock...)
	return sha256Hex(data)
}

// EnsureBunAdd runs bun add for pkg in targetDir, skipping the install if the
// package string has not changed since the last successful install.
//
// extraEnv is appended to the bun subprocess environment as "KEY=value"
// strings. Typical use is to pass npm install-time overrides such as
// npm_config_platform / npm_config_arch so postinstall scripts (e.g.
// electron's @electron/get) download artifacts for a non-host target
// instead of the host platform. The env is folded into the install cache
// hash so switching targets between runs triggers a fresh install.
func EnsureBunAdd(ctx context.Context, le *logrus.Entry, stateDir, targetDir, pkg string, extraEnv ...string) error {
	hash := sha256Hex([]byte(pkg + "\x00" + strings.Join(extraEnv, "\x00")))
	if installCurrent(targetDir, hash) {
		le.Debug("bun add cached, skipping")
		return nil
	}

	if err := fsutil.CleanCreateDir(targetDir); err != nil {
		return err
	}
	// #nosec G703 -- targetDir is a managed cache directory created by CleanCreateDir above.
	if err := os.WriteFile(filepath.Join(targetDir, "package.json"), []byte("{}"), 0o644); err != nil {
		return err
	}

	cmd, err := BunAdd(ctx, le, stateDir, "--cwd", targetDir, pkg)
	if err != nil {
		return err
	}
	if len(extraEnv) > 0 {
		cmd.Env = append(cmd.Env, extraEnv...)
	}
	if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		return err
	}

	return writeInstallHash(targetDir, hash)
}

// installCurrent returns true if targetDir has a matching install hash and node_modules exists.
func installCurrent(targetDir, hash string) bool {
	existing, err := os.ReadFile(filepath.Join(targetDir, installHashFile))
	if err != nil {
		return false
	}
	if string(existing) != hash {
		return false
	}
	info, err := os.Stat(filepath.Join(targetDir, "node_modules"))
	return err == nil && info.IsDir()
}

// writeInstallHash writes the install hash sentinel file.
func writeInstallHash(targetDir, hash string) error {
	// #nosec G703 -- targetDir is a managed cache directory created by the caller.
	return os.WriteFile(filepath.Join(targetDir, installHashFile), []byte(hash), 0o644)
}

// sha256Hex returns the hex-encoded SHA-256 of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
