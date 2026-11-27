package block_gc

// LeasedRoot identifies a root a lease holds.
type LeasedRoot struct {
	// BucketID is the root's bucket.
	BucketID string
	// Name is the root's name.
	Name string
}
