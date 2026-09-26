package block

// Transformer encodes and decodes blocks for storage.
// Its functions must be concurrency-safe.
type Transformer interface {
	// EncodeBlock encodes the block according to the config.
	// May reuse the same byte slice if possible.
	EncodeBlock([]byte) ([]byte, error)
	// DecodeBlock decodes the block according to the config.
	// May reuse the same byte slice if possible.
	DecodeBlock([]byte) ([]byte, error)
}
