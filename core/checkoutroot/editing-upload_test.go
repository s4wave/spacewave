//go:build !js

package checkoutroot

import (
	"io"
	"os"
	"strings"
)

// editingUpload makes a host edit as the publisher starts consuming the stream.
type editingUpload struct {
	// root holds the destination file to edit on the first read.
	root *os.Root
	// data supplies the uploaded contents.
	data *strings.Reader
}

// Read edits the destination once and streams the proposed replacement.
func (r *editingUpload) Read(p []byte) (int, error) {
	// Change the destination before sending the first bytes.
	if r.root != nil {
		if err := r.root.WriteFile("file", []byte("edited"), 0o644); err != nil {
			return 0, err
		}
		r.root = nil
	}

	// Supply the upload bytes after the host edit.
	return r.data.Read(p)
}

// _ is a type assertion.
var _ io.Reader = (*editingUpload)(nil)
