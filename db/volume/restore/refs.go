//go:build !js

package restore

import (
	"bufio"
	"context"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

const (
	// MaxRefLineBytes bounds each physical refs line, excluding its newline.
	MaxRefLineBytes = 64 * 1024
	// MissingReason reports a ref whose source lacks bytes.
	MissingReason = "source lacks block"
	// MismatchReason reports bytes that fail the ref's hash.
	MismatchReason = "hash mismatch"
)

// StreamRefs parses refs in bounded groups and stops when visit fails.
// Each nonblank line holds a B58 ref, optionally followed by a tab and notes.
// The callback must consume the group before returning; the next group reuses it.
func StreamRefs(ctx context.Context, input io.Reader, batchSize int, visit func([]*block.BlockRef) error) error {
	// Bound the refs awaiting a consumer without collecting the input file.
	reader := bufio.NewReaderSize(input, MaxRefLineBytes+1)
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
		if readErr == bufio.ErrBufferFull || lineSize > MaxRefLineBytes {
			return errors.Errorf("refs line %d is longer than %d bytes", lineNo, MaxRefLineBytes)
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
