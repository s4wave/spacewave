//go:build !tinygo

package block_store_s3

import "time"

// ObjectVersion is one version of a key in a version listing.
type ObjectVersion struct {
	// Key is the object key.
	Key string
	// VersionID names the version.
	VersionID string
	// DeleteMarker is set on a delete marker, which B2 calls a hide marker: a
	// version that hides the key and holds no content.
	DeleteMarker bool
	// Size is the version's bytes, zero for a delete marker.
	Size int64
	// ETag is the version's entity tag as listed, quotes included: the MD5
	// of a single-part object, or a digest with a part count after a dash
	// for a multipart one. Empty for a delete marker.
	ETag string
	// IsLatest is set on the key's current version, marker or content.
	IsLatest bool
	// LastModified is when the version was written, zero when the listing
	// did not give a time the parser reads.
	LastModified time.Time
}
