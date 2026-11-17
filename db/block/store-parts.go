package block

import "context"

// StorePart is one labeled child of a block store built from other stores.
type StorePart struct {
	// Label names the child's role in its parent, such as "lower" or "upper".
	Label string
	// Store is the child store.
	Store StoreOps
}

// StoreParts is implemented by a block store built from other stores, so an
// inspector can open it into its children.
type StoreParts interface {
	// GetStoreParts returns the child stores, which are fixed when the store
	// is built.
	GetStoreParts() []StorePart
}

// StoreHolder is implemented by a controller that holds a block store it built
// itself, outside the bus. The controller may build, replace or drop the store
// without a controller change, so a watcher waits on WaitHeldStore.
type StoreHolder interface {
	// GetHeldStore returns the held store, or nil before it is built.
	GetHeldStore() StoreOps
	// WaitHeldStore waits until the held store differs from prev, and returns
	// it, or returns ctx's error.
	WaitHeldStore(ctx context.Context, prev StoreOps) (StoreOps, error)
}
