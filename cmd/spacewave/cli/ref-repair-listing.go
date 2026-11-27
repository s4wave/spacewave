//go:build !js

package spacewave_cli

import (
	"bufio"
	stderrors "errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// refRepairListing publishes a private listing only after its walk succeeds.
type refRepairListing struct {
	// path is the destination replaced by the completed listing.
	path string
	// file is the private temporary file, nil when no listing was requested.
	file *os.File
	// writer buffers the streamed rows, nil when no listing was requested.
	writer *bufio.Writer
	// closed reports that finish already flushed and closed the temporary file.
	closed bool
}

// openRefRepairListing opens a temporary listing beside outputPath.
// An empty outputPath disables the listing.
// An output that names the Volume or another protected path, including an alias, is refused.
func openRefRepairListing(volumePath, outputPath string, protectedPaths ...string) (*refRepairListing, error) {
	// Leave the optional listing disabled until an output is requested.
	listing := &refRepairListing{path: outputPath}
	if outputPath == "" {
		return listing, nil
	}

	// Compare both normalized names and file identities before creating anything.
	for _, path := range append([]string{volumePath}, protectedPaths...) {
		if path == "" {
			continue
		}
		if err := refuseRefRepairAlias(outputPath, path); err != nil {
			if path == volumePath {
				return nil, errors.Wrap(err, "listing names the volume file")
			}
			return nil, err
		}
	}

	// Keep the temporary file private and on the destination's filesystem.
	file, err := os.CreateTemp(filepath.Dir(outputPath), filepath.Base(outputPath)+".*.tmp")
	if err != nil {
		return nil, err
	}
	listing.file = file
	listing.writer = bufio.NewWriter(file)
	return listing, nil
}

// finish flushes and syncs the completed listing before publishing it.
func (l *refRepairListing) finish() error {
	// Skip publication when no listing was requested.
	if l.file == nil {
		return nil
	}

	// Close the file even when a buffered write or sync fails.
	err := stderrors.Join(l.writer.Flush(), l.file.Sync(), l.file.Close())
	l.closed = true
	if err != nil {
		return errors.Wrap(err, "finish listing")
	}
	return os.Rename(l.file.Name(), l.path)
}

// discard closes and removes a temporary listing on every exit path.
// After finish renames the file, its temporary name no longer exists.
func (l *refRepairListing) discard() error {
	// Flush and close even a failed walk, without publishing its partial listing.
	if l.file == nil {
		return nil
	}
	var err error
	if !l.closed {
		err = stderrors.Join(l.writer.Flush(), l.file.Close())
		l.closed = true
	}

	// A published listing has already moved away from its temporary name.
	removeErr := os.Remove(l.file.Name())
	if errors.Is(removeErr, fs.ErrNotExist) {
		removeErr = nil
	}
	return stderrors.Join(err, removeErr)
}

// refuseRefRepairAlias protects inputs and other outputs even before they exist.
func refuseRefRepairAlias(outputPath, protectedPath string) error {
	// Resolve parent directories too, so two new outputs cannot alias through a symlink.
	outputDir, err := filepath.EvalSymlinks(filepath.Dir(outputPath))
	if err != nil {
		return err
	}
	protectedDir, err := filepath.EvalSymlinks(filepath.Dir(protectedPath))
	if err != nil {
		return err
	}

	// Normalize the destination names after resolving their parent directories.
	outputName, err := filepath.Abs(filepath.Join(outputDir, filepath.Base(outputPath)))
	if err != nil {
		return err
	}
	protectedName, err := filepath.Abs(filepath.Join(protectedDir, filepath.Base(protectedPath)))
	if err != nil {
		return err
	}
	if outputName == protectedName {
		return errors.Errorf("listing %q names a protected input or output file %q", outputPath, protectedPath)
	}

	// Compare existing files by identity to catch hard links and final symlinks.
	output, err := os.Stat(outputPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	protected, err := os.Stat(protectedPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if os.SameFile(output, protected) {
		return errors.Errorf("listing %q names a protected input or output file %q", outputPath, protectedPath)
	}
	return nil
}
