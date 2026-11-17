//go:build !tinygo

package block_store_s3

import "time"

// ObjectStat describes a stored object.
type ObjectStat struct {
	// Size is the content length in bytes.
	Size int64
	// ChecksumSHA256 is the SHA-256 the service verified when the object was
	// uploaded with one, or nil.
	ChecksumSHA256 []byte
	// LastModified is when the service stored the object, from its
	// Last-Modified header, or zero when it sent none that parses.
	LastModified time.Time
}
