//go:build !js && !wasip1

package world_recovery

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// canonicalScratchPath resolves the actual filesystem target before any writable opener.
func canonicalScratchPath(path string) (string, error) {
	// Native SQLite DSNs append their own query, so ambiguous path syntax is unsupported.
	if strings.ContainsAny(path, "?#%\x00") {
		return "", errors.New("scratch path contains unsupported SQLite URI syntax")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if info, linkErr := os.Lstat(absolute); linkErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("scratch target is a dangling symlink")
	}

	// New scratch files require an existing canonical parent directory.
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}

// rejectSourceAliases checks every filesystem target SQLite or the scratch lease may open.
func rejectSourceAliases(source os.FileInfo, scratch, lease string) error {
	// Existing symlinks and hardlinks are compared by inode, including writable sidecars.
	for _, path := range []string{scratch, scratch + "-wal", scratch + "-shm", scratch + "-journal", lease} {
		info, err := os.Stat(path)
		if err == nil && os.SameFile(source, info) {
			return errors.New("scratch or its writable sidecar/lease aliases the immutable source")
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
