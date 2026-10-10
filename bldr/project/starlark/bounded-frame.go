//go:build !js

package bldr_project_starlark

import (
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

// Kinds of frame exchanged between the process that evaluates a project and
// the child that does the evaluation.
const (
	// frameStart carries the memory budget and root file name to the child.
	frameStart byte = 'S'
	// frameRead carries a path from the child to the parent.
	frameRead byte = 'R'
	// frameData carries the file bytes the parent read for the child.
	frameData byte = 'D'
	// frameLoaded carries the path of a file the evaluation loaded.
	frameLoaded byte = 'L'
	// frameConfig carries the marshaled project config, ending the evaluation.
	frameConfig byte = 'C'
	// frameError carries the text of an error from the other side.
	frameError byte = 'E'
)

// frameHeaderSize is the size of a frame header: the kind and the body length.
const frameHeaderSize = 5

// writeFrame writes a frame of the given kind to w.
func writeFrame(w io.Writer, kind byte, body []byte) error {
	// Refuse a body the reader would refuse.
	if len(body) > MaxFileSize {
		return errors.Wrapf(ErrFileTooLarge, "write frame %c", kind)
	}

	// Write the header, then the body.
	header := [frameHeaderSize]byte{kind}
	binary.BigEndian.PutUint32(header[1:], uint32(len(body)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

// readFrame reads one frame from r and refuses one larger than MaxFileSize, so
// the peer cannot make the reader allocate without bound.
func readFrame(r io.Reader) (byte, []byte, error) {
	// Read the header and check the size before allocating the body.
	var header [frameHeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	size := binary.BigEndian.Uint32(header[1:])
	if size > MaxFileSize {
		return 0, nil, errors.Wrapf(ErrFileTooLarge, "read frame %c", header[0])
	}

	// Read the body.
	body := make([]byte, size)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	return header[0], body, nil
}
