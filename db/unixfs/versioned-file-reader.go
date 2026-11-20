package unixfs

import "context"

// VersionedFileReader reads the version from the same open file as its bytes.
type VersionedFileReader interface {
	// ReadAtVersion reads the range and returns its file version, if supported.
	ReadAtVersion(ctx context.Context, offset int64, data []byte) (int64, []byte, error)
}
