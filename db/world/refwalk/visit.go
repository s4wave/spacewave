package refwalk

import "github.com/s4wave/spacewave/db/block"

// Visit is what reading one block found.
type Visit struct {
	// Node is the block read.
	Node Node
	// Found is false when the store lacks the block.
	Found bool
	// Leaf is true when the block's type declares no refs. A schema-encoded
	// leaf still needs a successful exact decode before its empty list is
	// certain; check Unsure and DecodeErr before trusting it.
	Leaf bool
	// Unsure reports a guessed type the walker could not confirm, or a
	// schema-encoded leaf read with LeafPresenceOnly: its list is not certain
	// and the walker does not descend.
	Unsure bool
	// DecodeErr is why the block did not decode as its type, which must
	// explain every byte as DecodeExact checks.
	DecodeErr error
	// Refs are the refs a write records for the decoded block, nil for a leaf.
	// For an untyped block they are the refs a scan of its bytes found.
	Refs []*block.BlockRef
	// ScanMissed are the Refs of a block decoded by type that a scan of the
	// same bytes did not find, on a walk that scans.
	ScanMissed []*block.BlockRef
	// ScanExtra are the hashes that scan found outside Refs.
	ScanExtra []ScanHit
	// ScanShape outlines the bytes of a block with a ScanMissed ref, as
	// refwalk.ScanShape does.
	ScanShape string
	// ScanHits are an untyped block's Refs with the field each was found in.
	ScanHits []ScanHit
	// Opaque reports an untyped block whose bytes are not a message, so the
	// scan read no refs from it.
	Opaque bool
	// Data is the block as stored, before any transform decodes it, when
	// the walker keeps bytes and the block was found, or nil otherwise.
	Data []byte
	// blk is the decoded block, nil when decoding failed or was skipped.
	blk any
}
