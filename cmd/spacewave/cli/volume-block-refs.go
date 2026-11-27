//go:build !js

package spacewave_cli

import (
	"bufio"
	stderrors "errors"
	"io"
	"os"

	"github.com/pkg/errors"
)

// openVolumeBlockLacks opens a buffered private report and refuses to truncate an input file.
// The returned close function flushes the rows and closes the file.
func openVolumeBlockLacks(path string, inputs ...string) (_ io.Writer, closeLacks func() error, err error) {
	// Leave the stream unreported when no output was requested.
	if path == "" {
		return io.Discard, func() error { return nil }, nil
	}

	// Resolve the existing inputs before the output open can create a file.
	files := make([]os.FileInfo, len(inputs))
	for i, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, nil, err
		}
		files[i] = info
	}

	// Inspect the opened output before changing any existing file.
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return nil, nil, errors.Wrap(err, "open lacks output")
	}
	defer func() {
		if err != nil {
			err = stderrors.Join(err, errors.Wrap(out.Close(), "close lacks output"))
		}
	}()
	info, err := out.Stat()
	if err != nil {
		return nil, nil, errors.Wrap(err, "stat lacks output")
	}

	// Check each input by file identity, including links to it.
	for i, input := range files {
		if os.SameFile(info, input) {
			return nil, nil, errors.Errorf("lacks output names an input file %s", inputs[i])
		}
	}

	// Require a private file even when reusing an earlier report.
	if err := out.Chmod(0o600); err != nil {
		return nil, nil, errors.Wrap(err, "set lacks output permissions")
	}
	if err := out.Truncate(0); err != nil {
		return nil, nil, errors.Wrap(err, "clear lacks output")
	}

	// Flush buffered rows before closing the report, retaining both errors.
	writer := bufio.NewWriter(out)
	return writer, func() error {
		return stderrors.Join(errors.Wrap(writer.Flush(), "flush lacks output"), errors.Wrap(out.Close(), "close lacks output"))
	}, nil
}
