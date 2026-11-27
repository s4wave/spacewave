package devtool_store

// EngineID identifies the devtool World's head root. The bucket and object
// store are shared by every opener, whether the devtool's engine controller,
// the bldr testbed or OpenWorld.
const (
	// EngineID identifies the devtool World: its head is the root
	// world_block.WorldRootName(EngineID) of its bucket.
	EngineID = "bldr"
	// BucketID is the bucket that holds the World's blocks.
	BucketID = "bldr/devtool"
	// ObjectStoreID is the object store an older build kept the World head in.
	ObjectStoreID = "bldr/devtool"
)
