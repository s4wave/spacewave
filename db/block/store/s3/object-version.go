//go:build !tinygo

package block_store_s3

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
}
