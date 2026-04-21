package npm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/s4wave/spacewave/bldr/util/exec"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/sirupsen/logrus"
)

// installHashFile is the filename used to cache the install hash.
const installHashFile = ".bldr-install-hash"

// EnsureBunInstall copies srcPackageJson to targetDir/package.json and runs
// bun install, skipping the install if the package.json contents have not
// changed since the last successful install.
func EnsureBunInstall(ctx context.Context, le *logrus.Entry, stateDir, srcPackageJson, targetDir string) error {
	data, err := os.ReadFile(srcPackageJson)
	if err != nil {
		return err
	}

	hash := sha256Hex(data)
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

	cmd, err := BunInstall(ctx, le, stateDir, "--cwd", targetDir)
	if err != nil {
		return err
	}
	if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		return err
	}

	return writeInstallHash(targetDir, hash)
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
