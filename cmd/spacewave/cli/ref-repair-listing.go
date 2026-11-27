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
}

// openRefRepairListing opens a temporary listing beside outputPath.
// An empty outputPath disables the listing.
// An output that names the Volume, including an alias, is refused.
func openRefRepairListing(volumePath, outputPath string) (*refRepairListing, error) {
	// Leave the optional listing disabled until an output is requested.
	listing := &refRepairListing{path: outputPath}
	if outputPath == "" {
		return listing, nil
	}

	// Compare file identities before creating anything.
	output, err := os.Stat(outputPath)
	switch {
	case err == nil:
		vol, err := os.Stat(volumePath)
		if err != nil {
			return nil, err
		}
		if os.SameFile(vol, output) {
			return nil, errors.Errorf("listing %q names the volume file", outputPath)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}

	// Keep the temporary file private and on the destination's filesystem.
	listing.file, err = os.CreateTemp(filepath.Dir(outputPath), filepath.Base(outputPath)+".*.tmp")
	if err != nil {
		return nil, err
	}
	listing.writer = bufio.NewWriter(listing.file)
	return listing, nil
}

// finish flushes and syncs the completed listing before publishing it.
func (l *refRepairListing) finish() error {
	// Skip publication when no listing was requested.
	if l.file == nil {
		return nil
	}

	// Close the file even when a buffered write or sync fails.
	if err := stderrors.Join(l.writer.Flush(), l.file.Sync(), l.file.Close()); err != nil {
		return errors.Wrap(err, "finish listing")
	}
	return os.Rename(l.file.Name(), l.path)
}

// discard closes and removes a temporary listing on every exit path.
// After finish renames the file, its temporary name no longer exists.
func (l *refRepairListing) discard() {
	if l.file != nil {
		// Close errors cannot change the failed run; finish checks them on success.
		_ = l.file.Close()
		// Removal is best effort after failure or a successful rename.
		_ = os.Remove(l.file.Name())
	}
}
