// Package device defines the storage device a browser volume engine writes
// through, with memory and file implementations.
//
// A device holds named files addressed by byte offset. It orders every call
// and makes them durable only at a flush: a crash keeps every call before the
// last completed flush and may keep, drop, or tear any later write, truncate,
// or remove.
package device

import (
	"context"
	"strings"

	"github.com/pkg/errors"
)

// ErrShortRead reports a read range that extends past the end of its file.
var ErrShortRead = errors.New("read past end of file")

// Write is one positional write.
type Write struct {
	// Name is the file to write, created when absent.
	Name string
	// Offset is the byte offset of the first written byte. Writing past the
	// end extends the file, zero filling any gap.
	Offset int64
	// Data holds the bytes to write. The device does not retain it.
	Data []byte
}

// Read is one positional read.
type Read struct {
	// Name is the file to read.
	Name string
	// Offset is the byte offset of the first byte to read.
	Offset int64
	// Data receives len(Data) bytes.
	Data []byte
}

// File describes one file on a device.
type File struct {
	// Name is the file name.
	Name string
	// Size is the file length in bytes.
	Size int64
}

// Device is a flat namespace of files with batched positional I/O. Every
// method is one device round trip, and calls apply in the order they return.
type Device interface {
	// Write applies writes in order. With flush set it returns once every
	// earlier call on the device, including these writes, is durable.
	Write(ctx context.Context, writes []Write, flush bool) error
	// Read fills every read. A range past the end of its file, or of a
	// missing file, fails with ErrShortRead.
	Read(ctx context.Context, reads []Read) error
	// Truncate sets a file's length, zero filling when it grows.
	Truncate(ctx context.Context, name string, size int64) error
	// Remove deletes files; a missing file is not an error.
	Remove(ctx context.Context, names []string) error
	// List returns every file.
	List(ctx context.Context) ([]File, error)
}

// ValidName checks that name is a non-empty flat file name.
func ValidName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return errors.Errorf("invalid device file name %q", name)
	}
	return nil
}
