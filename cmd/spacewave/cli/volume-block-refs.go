//go:build !js

package spacewave_cli

import (
	"bufio"
	"context"
	stderrors "errors"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

const (
	// maxVolumeBlockRefLineBytes bounds each physical refs line, excluding its newline.
	maxVolumeBlockRefLineBytes = 64 * 1024
	// volumeBlockMissingReason reports a ref whose source lacks bytes.
	volumeBlockMissingReason = "source lacks block"
	// volumeBlockMismatchReason reports bytes that fail the ref's hash.
	volumeBlockMismatchReason = "hash mismatch"
)

// streamVolumeBlockRefs parses refs in bounded groups and stops when visit fails.
// Each nonblank line holds a B58 ref, optionally followed by a tab and notes.
// The callback must consume the group before returning; the next group reuses it.
func streamVolumeBlockRefs(ctx context.Context, input io.Reader, batchSize int, visit func([]*block.BlockRef) error) error {
	// Bound the refs awaiting a consumer without collecting the input file.
	reader := bufio.NewReaderSize(input, maxVolumeBlockRefLineBytes+1)
	refs := make([]*block.BlockRef, 0, batchSize)
	for lineNo := 1; ; lineNo++ {
		// Consume a line, including a final line without a newline.
		if err := ctx.Err(); err != nil {
			return err
		}
		lineBytes, readErr := reader.ReadSlice('\n')
		lineSize := len(lineBytes)
		if lineSize != 0 && lineBytes[lineSize-1] == '\n' {
			lineSize--
		}
		if readErr == bufio.ErrBufferFull || lineSize > maxVolumeBlockRefLineBytes {
			return errors.Errorf("refs line %d is longer than %d bytes", lineNo, maxVolumeBlockRefLineBytes)
		}
		if readErr != nil && readErr != io.EOF {
			return errors.Wrapf(readErr, "read refs line %d", lineNo)
		}

		// Ignore blank lines and parse only the ref column of the others.
		line := string(lineBytes)
		if strings.TrimSpace(line) != "" {
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			value, _, _ := strings.Cut(line, "\t")
			ref, err := block.UnmarshalBlockRefB58(value)
			if err != nil || ref == nil {
				return errors.Errorf("refs line %d has an invalid B58 block ref", lineNo)
			}
			refs = append(refs, ref)
		}

		// Pass each full group and the final short group to the consumer in order.
		if len(refs) < cap(refs) && readErr == nil {
			continue
		}
		if len(refs) != 0 {
			if err := visit(refs); err != nil {
				return err
			}
			clear(refs)
			refs = refs[:0]
		}
		if readErr == io.EOF {
			return nil
		}
	}
}

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
