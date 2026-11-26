package block_gc

// FrontRoot is a named root a walk reached a block from.
type FrontRoot struct {
	// BucketID is the bucket of the root.
	BucketID string
	// Name is the name of the root.
	Name string
}
