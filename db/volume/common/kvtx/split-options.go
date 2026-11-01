package kvtx

// SplitOptions adjusts a CopyStoreSplitting.
type SplitOptions struct {
	// Drop reports the blocks the copy leaves out, as CopyStoreDropping
	// describes; nil keeps every block.
	Drop DropBlocks
	// Kept returns keys the copy writes beside each block it keeps while Drop
	// is set, such as the keys that queue the block for the place the dropped
	// ones went; nil writes none.
	Kept KeptBlockKeys
	// Omit lists key prefixes the copy leaves out. The copy seeks past each
	// one, reading none of its keys, so an omitted key counts nowhere.
	Omit [][]byte
}
