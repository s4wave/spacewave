package npm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/s4wave/spacewave/bldr/util/exec"
	"github.com/sirupsen/logrus"
)

// installHashFile is the filename used to cache the install hash.
const installHashFile = ".bldr-install-hash"

// withInstallLock runs fn while holding the install lock for targetDir,
// creating the target's parent directory first. The unlock error joins any
// error returned by fn.
func withInstallLock(ctx context.Context, targetDir string, fn func() error) (retErr error) {
	// Lock beside the target so rebuilding its contents cannot replace the lock.
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return err
	}
	installLock := newInstallLock(targetDir + ".lock")
	if err := installLock.Lock(ctx); err != nil {
		return err
	}
	defer func() {
		retErr = errors.Join(retErr, installLock.Unlock())
	}()

	// Retain exclusive access through the complete install and hash write.
	return fn()
}

// EnsureSharedBunInstall copies srcPackageJson and its sibling bun.lock, when
// present, into the shared install cache and runs bun install there, skipping
// the install if the package manifest contents have not changed since the
// last successful install. The install directory is keyed by the install
// hash, so identical dependency sets across projects and state directories
// share one node_modules. It returns the install root actually used. When
// the shared cache root is unavailable, it falls back to fallbackDir.
func EnsureSharedBunInstall(ctx context.Context, le *logrus.Entry, stateDir, srcPackageJson, fallbackDir string) (string, error) {
	// Read the dependency inputs before selecting their content-addressed cache.
	data, err := os.ReadFile(srcPackageJson)
	if err != nil {
		return "", err
	}
	lockData, lockFound, err := readSiblingBunLock(srcPackageJson)
	if err != nil {
		return "", err
	}

	// Hash the manifest contents to key the install cache.
	hash := bunInstallHash(data, lockData)

	// Prefer the shared cache directory; fall back to the caller's own
	// state directory when it is unavailable.
	targetDir := fallbackDir
	if sharedDir, ok := sharedInstallDir(hash); ok {
		targetDir = sharedDir
	}
	return targetDir, ensureBunInstallAt(ctx, le, stateDir, targetDir, hash, data, lockData, lockFound)
}

// sharedInstallCacheEnv overrides the root of the shared install cache.
const sharedInstallCacheEnv = "BLDR_SHARED_INSTALL_CACHE"

// sharedInstallDir returns the shared cache directory for the install hash.
// It reports false when the shared cache root cannot be created, and the
// caller should install into its own state directory instead.
func sharedInstallDir(hash string) (string, bool) {
	// Resolve the configured shared root or the host's application cache.
	root := os.Getenv(sharedInstallCacheEnv)
	if root == "" {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return "", false
		}
		root = filepath.Join(userCacheDir, "bldr", "web-pkgs")
	}

	// Allow callers to fall back when the shared root cannot be created.
	// #nosec G703 -- root is an operator-selected dependency cache directory.
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", false
	}
	return filepath.Join(root, hash), true
}

// ensureBunInstallAt runs the cached bun install for the hashed package
// manifest into targetDir.
func ensureBunInstallAt(ctx context.Context, le *logrus.Entry, stateDir, targetDir, hash string, packageJSON, bunLock []byte, lockFound bool) error {
	return withInstallLock(ctx, targetDir, func() error {
		// Skip the install when the hash sentinel already matches.
		if installCurrent(targetDir, hash) {
			le.Debug("bun install cached, skipping")
			return nil
		}

		// Recreate the install directory and seed it with the package
		// manifest and lockfile.
		if err := seedInstallDir(targetDir, packageJSON, bunLock, lockFound); err != nil {
			return err
		}

		// Freeze the install to the seeded lockfile when one exists.
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

		// Record the hash so a later install can skip this directory.
		return writeInstallHash(targetDir, hash)
	})
}

// seedInstallDir recreates targetDir with the package manifest and, when
// lockFound, the lockfile. The manifest omits root scripts: they expect the
// project checkout, which the cache does not contain.
func seedInstallDir(targetDir string, packageJSON, bunLock []byte, lockFound bool) error {
	// Recreate the managed cache directory.
	if err := fsutil.CleanCreateDir(targetDir); err != nil {
		return err
	}

	// Write the manifest without the project's lifecycle scripts.
	seedJSON, err := withoutRootScripts(packageJSON)
	if err != nil {
		return err
	}
	if err := writeSeedFile(targetDir, "package.json", seedJSON); err != nil {
		return err
	}

	// Write the lockfile the frozen install verifies against.
	if !lockFound {
		return nil
	}
	return writeSeedFile(targetDir, "bun.lock", bunLock)
}

// writeSeedFile writes one file of the install directory seed.
func writeSeedFile(targetDir, name string, data []byte) error {
	// #nosec G703 -- targetDir is a managed cache directory created by CleanCreateDir.
	return os.WriteFile(filepath.Join(targetDir, name), data, 0o644)
}

// withoutRootScripts removes the scripts field from a package manifest so a
// bun install cannot run the project's root lifecycle scripts.
func withoutRootScripts(packageJSON []byte) ([]byte, error) {
	// Parse the manifest as an object.
	var parser fastjson.Parser
	manifest, err := parser.ParseBytes(packageJSON)
	if err != nil {
		return nil, err
	}

	// Leave a manifest without scripts byte for byte unchanged.
	if manifest.Get("scripts") == nil {
		return packageJSON, nil
	}

	// Re-encode the manifest without the scripts.
	manifest.Del("scripts")
	return manifest.MarshalTo(nil), nil
}

// readSiblingBunLock reads the bun.lock file beside srcPackageJson. It
// reports found=false when the sibling lockfile does not exist.
func readSiblingBunLock(srcPackageJson string) ([]byte, bool, error) {
	// Read the lockfile beside the manifest, preserving absence as an option.
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

// bunInstallHash returns the install hash for a package manifest and its
// optional bun.lock contents. The hash changes when either file changes.
func bunInstallHash(packageJSON, bunLock []byte) string {
	// Preserve the package-only key when there is no lockfile.
	if bunLock == nil {
		return sha256Hex(packageJSON)
	}

	// Delimit both inputs so distinct manifest and lockfile pairs stay distinct.
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
// The package download cache is local to targetDir and shares its install lock.
func EnsureBunAdd(ctx context.Context, le *logrus.Entry, stateDir, targetDir, pkg string, extraEnv ...string) error {
	// Hash the package string and extra environment so a change to either
	// triggers a fresh install.
	hash := sha256Hex([]byte(pkg + "\x00" + strings.Join(extraEnv, "\x00")))
	return withInstallLock(ctx, targetDir, func() error {
		// Skip the install when the hash sentinel already matches.
		if installCurrent(targetDir, hash) {
			le.Debug("bun add cached, skipping")
			return nil
		}

		// Recreate the install directory and seed it with an empty
		// package manifest.
		if err := fsutil.CleanCreateDir(targetDir); err != nil {
			return err
		}

		// #nosec G703 -- targetDir is a managed cache directory created by the caller.
		if err := os.WriteFile(filepath.Join(targetDir, "package.json"), []byte("{}"), 0o644); err != nil {
			return err
		}

		// Keep downloads under the same target lock as node_modules. Concurrent
		// target installs can race while Bun populates its global cache on Windows.
		cmd, err := BunAdd(ctx, le, stateDir, "--cwd", targetDir, pkg)
		if err != nil {
			return err
		}
		if len(extraEnv) > 0 {
			cmd.Env = append(cmd.Env, extraEnv...)
		}
		cmd.Env = append(cmd.Env, "BUN_INSTALL_CACHE_DIR="+filepath.Join(targetDir, ".bun-cache"))
		if err := exec.StartAndWait(ctx, le, cmd); err != nil {
			return err
		}

		// Record the hash so a later install can skip this directory.
		return writeInstallHash(targetDir, hash)
	})
}

// installCurrent returns true if targetDir has a matching install hash and node_modules exists.
func installCurrent(targetDir, hash string) bool {
	// Require both a successful install marker and its materialized packages.
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
	// Encode the complete digest for a stable filesystem cache key.
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
