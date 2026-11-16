//go:build !tinygo

package block_store_s3

// ObjectStat describes a stored object.
type ObjectStat struct {
	// Size is the content length in bytes.
	Size int64
	// ChecksumSHA256 is the SHA-256 the service verified when the object was
	// uploaded with one, or nil.
	ChecksumSHA256 []byte
}
