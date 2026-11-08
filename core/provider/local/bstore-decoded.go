package provider_local

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ErrNoDecodedStore is returned by StoreDecodedRefs on a store that does not
// keep peer reads for upload, which never stores a peer's block itself.
var ErrNoDecodedStore = errors.New("the block store does not keep peer reads for upload")

// StoreDecodedRefs stores a block a read served without its refs, with refs
// the caller decoded from its bytes by the block's type, as a peer fill stores
// a peer's block that came with them: with its upload marker, below the
// ownership layer as a cache fill. The read path serves such a block without
// storing it, since the pack would keep it as a leaf; refs decoded by type
// are the refs a write of the block records. It writes only on the World's
// new owner, where peer fills upload.
func (b *BlockStore) StoreDecodedRefs(ctx context.Context, ref *block.BlockRef, data []byte, refs []*block.BlockRef) error {
	// Write only where peer fills upload.
	p := b.placement
	if p == nil || p.serveOnly || !p.peerUpload {
		return ErrNoDecodedStore
	}

	// Store only bytes that hash to ref.
	if ref.GetEmpty() {
		return errors.New("store decoded refs: empty ref")
	}
	if _, err := ref.GetHash().VerifyData(data); err != nil {
		return errors.Wrap(err, "verify decoded block")
	}
	return p.fillFromFallback(ctx, ref, &block.StoredBlock{Data: data, Refs: refs, RefsKnown: true})
}
