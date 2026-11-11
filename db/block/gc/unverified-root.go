package block_gc

// UnverifiedRoot is a named root a list rebuild's walk reached unverified
// lists from.
type UnverifiedRoot struct {
	// BucketID is the root's bucket; reader pins and stages are in the
	// bucket with the empty ID.
	BucketID string
	// Name is the root's name in its bucket.
	Name string
	// Unverified counts the unverified lists the walk reached first from
	// this root.
	Unverified uint64
}
