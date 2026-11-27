//go:build linux

package folder

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

// moveToTrash moves the file at path to the freedesktop.org trash on its file
// system: the home trash on the home file system, or the file system's
// $topdir/.Trash/$uid or $topdir/.Trash-$uid. It writes the entry's
// .trashinfo first, created exclusively to claim the name, then renames the
// file into files/ without replacing one. It returns the trashed path, and a
// function that removes the .trashinfo once the file left the trash again.
func moveToTrash(path string) (string, func() error, error) {
	// Find the trash on the file's file system.
	trash, topdir, err := trashDir(path)
	if err != nil {
		return "", nil, err
	}
	for _, sub := range []string{"files", "info"} {
		if err := os.MkdirAll(filepath.Join(trash, sub), 0o700); err != nil {
			return "", nil, err
		}
	}

	// The home trash records the absolute path, a top directory trash the
	// path relative to the top directory.
	recorded := path
	if topdir != "" {
		if rel, err := filepath.Rel(topdir, path); err == nil {
			recorded = rel
		}
	}
	parts := strings.Split(recorded, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	content := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n", strings.Join(parts, "/"), time.Now().Format("2006-01-02T15:04:05"))

	// Claim a name with its .trashinfo, then move the file under it.
	base := filepath.Base(path)
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name = base + "." + strconv.Itoa(n)
		}
		info := filepath.Join(trash, "info", name+".trashinfo")
		f, err := os.OpenFile(info, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		_, werr := f.WriteString(content)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		dst := filepath.Join(trash, "files", name)
		if werr == nil {
			werr = renameNoReplace(path, dst)
		}
		if errors.Is(werr, fs.ErrExist) {
			_ = os.Remove(info)
			continue
		}
		if werr != nil {
			_ = os.Remove(info)
			return "", nil, werr
		}
		return dst, func() error { return os.Remove(info) }, nil
	}
}

// trashDir returns the trash of this user on the file system holding path,
// and its top directory, empty for the home trash.
func trashDir(path string) (string, string, error) {
	// Find the file's file system.
	dev, err := deviceOf(filepath.Dir(path))
	if err != nil {
		return "", "", err
	}

	// Prefer the home trash on its own file system.
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		data = filepath.Join(home, ".local", "share")
	}
	if d, err := deviceOf(data); err == nil && d == dev {
		return filepath.Join(data, "Trash"), "", nil
	}

	// Use the top directory's trash otherwise: a shared sticky .Trash that is
	// not a symlink, or this user's .Trash-$uid.
	topdir, err := mountRoot(path, dev)
	if err != nil {
		return "", "", err
	}
	uid := strconv.Itoa(os.Getuid())
	shared := filepath.Join(topdir, ".Trash")
	if info, err := os.Lstat(shared); err == nil && info.IsDir() && info.Mode()&os.ModeSticky != 0 {
		return filepath.Join(shared, uid), topdir, nil
	}

	// Create this user's own trash when there is no shared one.
	own := filepath.Join(topdir, ".Trash-"+uid)
	if err := os.Mkdir(own, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", "", err
	}
	if info, err := os.Lstat(own); err != nil || !info.IsDir() {
		return "", "", errors.Errorf("no trash on the file system of %s", path)
	}
	return own, topdir, nil
}

// deviceOf returns the device of the file at path.
func deviceOf(path string) (uint64, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return 0, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	return st.Dev, nil
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
