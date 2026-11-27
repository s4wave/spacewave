//go:build darwin

package folder

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/pkg/errors"
)

// moveToTrash moves the file at path to the user's Trash on its volume: the
// home Trash on the home volume, or the volume's .Trashes directory for this
// user, never replacing a trashed file. It returns the trashed path, and a
// function that forgets the trash entry once the file left it again.
func moveToTrash(path string) (string, func() error, error) {
	// Find the Trash on the file's volume.
	trash, err := trashDir(path)
	if err != nil {
		return "", nil, err
	}

	// Move it under its name, or its name and a counter when taken, as the
	// Finder names them.
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name = stem + " " + strconv.Itoa(n) + ext
		}
		dst := filepath.Join(trash, name)
		err := renameNoReplace(path, dst)
		if err == nil {
			return dst, func() error { return nil }, nil
		}
		if !os.IsExist(err) {
			return "", nil, err
		}
	}
}

// trashDir returns the Trash of this user on the volume holding path.
func trashDir(path string) (string, error) {
	// Find the file's volume.
	dev, err := deviceOf(filepath.Dir(path))
	if err != nil {
		return "", err
	}

	// Prefer the home Trash on its own volume.
	if home, err := os.UserHomeDir(); err == nil {
		trash := filepath.Join(home, ".Trash")
		if d, err := deviceOf(trash); err == nil && d == dev {
			return trash, nil
		}
	}

	// Use the volume's Trash for this user otherwise.
	root, err := mountRoot(path, dev)
	if err != nil {
		return "", err
	}
	trash := filepath.Join(root, ".Trashes", strconv.Itoa(os.Getuid()))
	if d, err := deviceOf(trash); err == nil && d == dev {
		return trash, nil
	}
	return "", errors.Errorf("no trash on the volume of %s", path)
}

// deviceOf returns the device of the file at path.
func deviceOf(path string) (uint64, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return 0, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	return uint64(st.Dev), nil //nolint:gosec // a device number is not negative.
}

// mountRoot returns the topmost ancestor of path on the device dev.
func mountRoot(path string, dev uint64) (string, error) {
	dir := filepath.Dir(path)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir, nil
		}
		d, err := deviceOf(parent)
		if err != nil {
			return "", err
		}
		if d != dev {
			return dir, nil
		}
		dir = parent
	}
}
