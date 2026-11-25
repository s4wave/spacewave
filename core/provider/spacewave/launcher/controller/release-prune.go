//go:build !js && !goscript

package spacewave_launcher_controller

import (
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// KeepStagedReleases turns off the removal of older release copies, so every
// version folder the launcher staged stays in its staging directory. Call it
// before the controller executes.
func (c *Controller) KeepStagedReleases() {
	c.keepStagedReleases = true
}

// pruneStagedReleases removes the version folders in stagingDir other than
// the one just staged, so the staging directory holds one release copy and
// not every release since install.
//
// Invariants:
//
//  1. It keeps the version just staged, the versions the app and daemon
//     update states name, which covers a daemon handoff already accepted
//     from an older release, and the folder holding the running daemon.
//  2. When it cannot tell where the running daemon lives it removes nothing,
//     since a daemon that runs from a staged copy would lose its own files.
//  3. It removes only directories, not links, whose names are valid release
//     versions, at the top of stagingDir. The removal goes through os.Root,
//     which cannot leave stagingDir, so neither a link inside a folder nor a
//     crafted name reaches a file outside it.
//  4. A failure keeps the folder for the next staging; the staged update
//     stands either way.
func (c *Controller) pruneStagedReleases(stagingDir, staged string) error {
	// Collect the versions that stay.
	keep := map[string]bool{staged: true}
	if info := c.launcherInfoCtr.GetValue(); info != nil {
		keep[info.GetUpdateState().GetVersion()] = true
		keep[info.GetDaemonUpdateState().GetVersion()] = true
	}

	// Keep the folder of a daemon that runs from a staged copy.
	executable, _, _, err := c.currentExecutableBundle()
	if err != nil {
		return errors.Wrap(err, "locate the running daemon")
	}
	if version, ok := stagedVersionOf(stagingDir, executable); ok {
		keep[version] = true
	}

	// Remove every other version folder through a root confined to stagingDir.
	root, err := os.OpenRoot(stagingDir)
	if err != nil {
		return err
	}
	defer root.Close()
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return err
	}
	var failures []error
	for _, entry := range entries {
		name := entry.Name()
		if keep[name] || !entry.Type().IsDir() {
			continue
		}
		if _, err := releaseVersionStagingRoot(stagingDir, name); err != nil {
			continue
		}
		if err := root.RemoveAll(name); err != nil {
			failures = append(failures, err)
		}
	}
	return stderrors.Join(failures...)
}

// stagedVersionOf returns the version folder of stagingDir that holds path,
// comparing the paths as given and then with their links resolved, so a
// linked profile directory still matches.
func stagedVersionOf(stagingDir, path string) (string, bool) {
	// Compare as given, then with both paths resolved.
	if version, ok := firstSegmentBelow(stagingDir, path); ok {
		return version, true
	}
	resolvedDir, err := filepath.EvalSymlinks(stagingDir)
	if err != nil {
		return "", false
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	return firstSegmentBelow(resolvedDir, resolvedPath)
}

// firstSegmentBelow returns the first segment of path below dir.
func firstSegmentBelow(dir, path string) (string, bool) {
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return strings.SplitN(rel, string(filepath.Separator), 2)[0], true
}
