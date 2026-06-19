//go:build goscript

package transform_all

import (
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_chksum "github.com/s4wave/spacewave/db/block/transform/chksum"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
)

// BuildStepFactories returns the hydra block transforms for the goscript
// browser build. lz4 is excluded: pierrec/lz4 pulls reflect into the browser
// closure and the browser persists gzip-compressed blocks, so the lz4 decoder
// is not needed here. Native builds keep lz4 for cross-peer block decoding.
func BuildStepFactories() []block_transform.StepFactory {
	return []block_transform.StepFactory{
		transform_gzip.NewStepFactory(),
		transform_s2.NewStepFactory(),
		transform_chksum.NewStepFactory(),
		transform_blockenc.NewStepFactory(),
	}
}
