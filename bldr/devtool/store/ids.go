package devtool_store

// The devtool World's identity: the engine ID that names its head root, and
// the bucket and object store every opener uses, whether the devtool's engine
// controller, the bldr testbed or OpenWorld.
const (
	// EngineID names the devtool World: its head is the root
	// world_block.WorldRootName(EngineID) of its bucket.
	EngineID = "bldr"
	// BucketID is the bucket that holds the World's blocks.
	BucketID = "bldr/devtool"
	// ObjectStoreID is the object store an older build kept the World head in.
	ObjectStoreID = "bldr/devtool"
)
