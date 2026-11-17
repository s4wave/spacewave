//go:build !tinygo

package block_store_s3

// PackLocation names the pack the local index places a block in, with the
// size the index recorded for it. An empty ID means the index places the
// block in no pack it holds an entry for.
type PackLocation struct {
	// ID is the pack's id.
	ID string
	// Size is the pack's recorded size in bytes.
	Size int64
}
