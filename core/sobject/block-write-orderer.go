package sobject

// BlockWriteOrderer is a SharedObject whose state writes commit to the same
// write-ordered store as its block store, so a state write that is durable
// makes every block store write before it durable too, including one
// committed with write ordering only (kvtx.WithOrderedCommitContext). A
// crash can then lose a block store write only together with every state
// write after it.
type BlockWriteOrderer interface {
	// OrdersBlockWrites reports whether the state writes order the block
	// store's writes, as BlockWriteOrderer describes.
	OrdersBlockWrites() bool
}

// OrdersBlockWrites reports whether so's state writes order its block store's
// writes. A SharedObject that does not implement BlockWriteOrderer does not.
func OrdersBlockWrites(so SharedObject) bool {
	orderer, ok := so.(BlockWriteOrderer)
	return ok && orderer.OrdersBlockWrites()
}
