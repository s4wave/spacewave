//go:build linux

package boot

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
)

// toolApplets is the fixed BusyBox interface expected by the serial-shell image.
// The input build must implement these applets; Linux execution is checked later.
const toolApplets = "sh cat ls echo pwd mkdir rmdir rm cp mv touch sync dd wc head tail sha256sum sleep ps kill true false uname id stat mount umount"

// buildTools adds ordinary relative applet links and empty future mountpoints.
func buildTools(ctx context.Context, root *unixfs.FSHandle, busybox io.Reader) error {
	// Create fixed directories without copying a host tree or following host links.
	ts := time.Now()
	for _, name := range []string{"bin", "data", "dev", "proc", "tmp"} {
		if err := root.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_Dir(), 0o755, ts); err != nil {
			return err
		}
	}

	// Stream exactly one tool executable; applets share its immutable file content.
	bin, err := root.Lookup(ctx, "bin")
	if err != nil {
		return err
	}
	defer bin.Release()
	if err := writeImageFile(ctx, bin, "busybox", busybox); err != nil {
		return err
	}
	for applet := range strings.FieldsSeq(toolApplets) {
		if err := bin.Symlink(ctx, true, applet, []string{"busybox"}, false, ts); err != nil {
			return err
		}
	}

	// Refuse missing content or layout before publishing this constructed image.
	return verifyTools(ctx, root)
}

// verifyTools checks the fixed layout and streams the entire required tool file.
// Hash-addressed content comes from the retained root, not bootstrap tool pages.
func verifyTools(ctx context.Context, root *unixfs.FSHandle) error {
	// Require real empty mountpoints; the later scratch mount sets its own mode.
	for _, name := range []string{"bin", "data", "dev", "proc", "tmp"} {
		dir, err := root.Lookup(ctx, name)
		if err != nil {
			return err
		}
		err = verifyImageDirectory(ctx, dir, 0o755)
		if err == nil && name != "bin" {
			err = dir.ReaddirAll(ctx, 0, func(unixfs.FSCursorDirent) error {
				return errors.New("image mountpoint is not empty: " + name)
			})
		}
		dir.Release()
		if err != nil {
			return err
		}
	}

	// Require one executable and each ordinary relative applet link to that file.
	bin, err := root.Lookup(ctx, "bin")
	if err != nil {
		return err
	}
	defer bin.Release()
	for applet := range strings.FieldsSeq(toolApplets) {
		entry, err := bin.Lookup(ctx, applet)
		if err != nil {
			return err
		}
		link, absolute, err := entry.Readlink(ctx, "")
		entry.Release()
		if err != nil {
			return err
		}
		if absolute || !slices.Equal(link, []string{"busybox"}) {
			return errors.New("invalid image applet link: " + applet)
		}
	}
	return verifyImageFile(ctx, root, "bin/busybox", nil)
}

// verifyImageDirectory checks the directory kind and exact permission bits.
func verifyImageDirectory(ctx context.Context, node *unixfs.FSHandle, mode fs.FileMode) error {
	// Check the kind before treating a node as a file, directory or symbolic link.
	actualKind, err := node.GetNodeType(ctx)
	if err != nil {
		return err
	}
	if !actualKind.GetIsDirectory() {
		return errors.New("unexpected image node type")
	}

	// Require the exact stored permission bits of each fixed directory.
	actualMode, err := node.GetPermissions(ctx)
	if err != nil {
		return err
	}
	if actualMode != mode {
		return errors.New("unexpected image node permissions")
	}
	return nil
}
